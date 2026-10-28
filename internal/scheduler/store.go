// Package scheduler runs jobs at their time: reminders, repeating channel
// autoposts, and any other kind with a handler. Jobs live in lys.scheduled_jobs,
// so they survive restarts.
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
)

// Job is one scheduled job and the channel it belongs to.
type Job struct {
	ID          int64
	Kind        string
	Payload     json.RawMessage
	RunAt       time.Time
	RepeatEvery time.Duration // 0 runs once
	Attempts    int
	CreatedBy   int64 // platform user; 0 if unknown

	ConnectionID int64
	Platform     string
	Channel      core.Channel
	Active       bool // false while the channel is blocked or revoked
}

// NewJob is what Schedule needs.
type NewJob struct {
	Kind         string
	ConnectionID int64
	CreatedBy    int64
	Payload      any // stored as JSON
	RunAt        time.Time
	RepeatEvery  time.Duration
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Schedule adds a job and returns its ID.
func (s *Store) Schedule(ctx context.Context, j NewJob) (int64, error) {
	payload, err := json.Marshal(j.Payload)
	if err != nil {
		return 0, fmt.Errorf("scheduler: encode payload: %w", err)
	}
	var repeat *time.Duration
	if j.RepeatEvery > 0 {
		repeat = &j.RepeatEvery
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO lys.scheduled_jobs (kind, connection_id, created_by, payload, run_at, repeat_every)
		VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6) RETURNING id`,
		j.Kind, j.ConnectionID, j.CreatedBy, payload, j.RunAt, repeat).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("database: schedule %s job: %w", j.Kind, err)
	}
	return id, nil
}

const selectJob = `
	SELECT j.id, j.kind, j.payload, j.run_at, COALESCE(j.repeat_every, interval '0'), j.attempts, COALESCE(j.created_by, 0),
	       pc.id, pc.platform, pc.external_id, pc.name, (pc.authorized AND pc.enabled AND NOT pc.admin_blocked)
	FROM lys.scheduled_jobs j
	JOIN lys.platform_connections pc ON pc.id = j.connection_id`

func scanJobs(rows pgx.Rows) ([]Job, error) {
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.Payload, &j.RunAt, &j.RepeatEvery, &j.Attempts, &j.CreatedBy,
			&j.ConnectionID, &j.Platform, &j.Channel.ID, &j.Channel.Name, &j.Active); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Claim takes up to limit due jobs for this instance, leasing them for lease.
// A job whose lease runs out (its instance died) can be claimed again.
func (s *Store) Claim(ctx context.Context, limit int, lease time.Duration) ([]Job, error) {
	var jobs []Job
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, selectJob+`
			WHERE j.done_at IS NULL AND j.run_at <= now()
			  AND (j.claimed_until IS NULL OR j.claimed_until < now())
			ORDER BY j.run_at
			LIMIT $1
			FOR UPDATE OF j SKIP LOCKED`, limit)
		if err != nil {
			return err
		}
		if jobs, err = scanJobs(rows); err != nil {
			return err
		}
		for _, j := range jobs {
			if _, err := tx.Exec(ctx, `UPDATE lys.scheduled_jobs SET claimed_until = now() + $2::interval WHERE id = $1`, j.ID, lease); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("database: claim due jobs: %w", err)
	}
	return jobs, nil
}

// nextRun moves a repeating job to its next time after now.
const nextRun = `run_at + repeat_every * (floor(extract(epoch FROM now() - run_at) / extract(epoch FROM repeat_every)) + 1)`

// Done records a finished run: a one-off job is done, a repeating one moves
// to its next time. failure, if set, is kept as the job's last error.
func (s *Store) Done(ctx context.Context, id int64, failure string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE lys.scheduled_jobs SET
			done_at = CASE WHEN repeat_every IS NULL THEN now() END,
			run_at = CASE WHEN repeat_every IS NULL THEN run_at ELSE `+nextRun+` END,
			attempts = 0, claimed_until = NULL, last_error = NULLIF($2, '')
		WHERE id = $1`, id, failure)
	if err != nil {
		return fmt.Errorf("database: finish job %d: %w", id, err)
	}
	return nil
}

// Retry records a failed attempt and when to try again.
func (s *Store) Retry(ctx context.Context, id int64, failure string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE lys.scheduled_jobs SET attempts = attempts + 1, last_error = $2, run_at = $3, claimed_until = NULL
		WHERE id = $1`, id, failure, at)
	if err != nil {
		return fmt.Errorf("database: retry job %d: %w", id, err)
	}
	return nil
}

// Pending lists a channel's jobs of one kind that haven't finished, soonest first.
func (s *Store) Pending(ctx context.Context, kind string, connectionID int64) ([]Job, error) {
	rows, err := s.pool.Query(ctx, selectJob+`
		WHERE j.done_at IS NULL AND j.kind = $1 AND j.connection_id = $2
		ORDER BY j.run_at`, kind, connectionID)
	if err != nil {
		return nil, fmt.Errorf("database: list %s jobs: %w", kind, err)
	}
	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, fmt.Errorf("database: list %s jobs: %w", kind, err)
	}
	return jobs, nil
}

// CountByCreator counts someone's unfinished jobs of one kind.
func (s *Store) CountByCreator(ctx context.Context, kind string, createdBy int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM lys.scheduled_jobs WHERE done_at IS NULL AND kind = $1 AND created_by = $2`,
		kind, createdBy).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("database: count %s jobs: %w", kind, err)
	}
	return n, nil
}

// ErrNotFound means no unfinished job matches.
var ErrNotFound = errors.New("no such job")

// Cancel finishes a job early. It must belong to the channel, so a command
// can't cancel another channel's jobs.
func (s *Store) Cancel(ctx context.Context, id, connectionID int64, kind string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE lys.scheduled_jobs SET done_at = now(), last_error = 'cancelled'
		WHERE id = $1 AND connection_id = $2 AND kind = $3 AND done_at IS NULL`, id, connectionID, kind)
	if err != nil {
		return fmt.Errorf("database: cancel job %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
