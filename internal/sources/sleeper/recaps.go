package sleeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/lsariol/lys/internal/features/sleeper"
	"github.com/lsariol/lys/internal/sources"
)

// Recaps is the source that queues each league's weekly recap: its results,
// highlights and standings, due at 10 AM Eastern on the Tuesday after the
// week's last game (api.RecapDue). It checks every minute, so nothing is
// scheduled ahead; the recap's first row in sleeper_events is what makes it
// go out once.
type Recaps struct {
	client *api.Client
	db     *pgxpool.Pool
	now    func() time.Time
	every  time.Duration
}

var _ sources.Source = (*Recaps)(nil)

func NewRecaps(client *api.Client, db *pgxpool.Pool) *Recaps {
	return &Recaps{client: client, db: db, now: time.Now, every: time.Minute}
}

func (r *Recaps) Name() string { return "sleeper recaps" }

// recapLate is how late a recap may still go out (after Lys was down, say).
const recapLate = 24 * time.Hour

func (r *Recaps) Run(ctx context.Context, _ sources.Publisher) error {
	t := time.NewTicker(r.every)
	defer t.Stop()
	for {
		if err := r.tick(ctx); err != nil {
			log.Printf("[Sleeper] recaps: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// tick queues the recaps that are due and not yet made. Last week and this
// week are checked, since Sleeper may already show the next week on Tuesday.
func (r *Recaps) tick(ctx context.Context) error {
	state, err := r.client.NFLState(ctx)
	if err != nil {
		return err
	}
	if state.SeasonType != "regular" {
		return nil
	}

	now := r.now()
	for _, week := range []int{state.Week - 1, state.Week} {
		if week < 1 {
			continue
		}
		games, err := r.client.Games(ctx, state.Season, week)
		if err != nil {
			log.Printf("[Sleeper] recaps: %v", err)
			continue
		}
		if len(games) == 0 {
			continue
		}
		due := api.RecapDue(games)
		if now.Before(due) {
			continue
		}
		if err := r.queue(ctx, state.Season, week, due, now.After(due.Add(recapLate))); err != nil {
			return err
		}
	}
	return nil
}

// queue makes the week's recap for each league with recaps on that hasn't had
// one, and queues its messages. late records it as skipped instead.
func (r *Recaps) queue(ctx context.Context, season string, week int, due time.Time, late bool) error {
	id := fmt.Sprintf("%s-w%d", season, week)
	rows, err := r.db.Query(ctx, `
		SELECT l.sleeper_league_id FROM lys.sleeper_leagues l
		WHERE l.enabled AND l.community_id IS NOT NULL AND COALESCE((l.notifications->>'recap')::boolean, true)
		  AND NOT EXISTS (SELECT 1 FROM lys.sleeper_events e
		                  WHERE e.sleeper_league_id = l.sleeper_league_id AND e.event_type = 'recap' AND e.external_event_id = $1)`,
		id+"-1")
	if err != nil {
		return fmt.Errorf("database: read Sleeper leagues: %w", err)
	}
	leagues, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("database: read Sleeper leagues: %w", err)
	}

	for _, league := range leagues {
		if late {
			if _, err := record(ctx, r.db, event{league, "recap", id + "-1"}, []byte(`{"skipped": "late"}`), due, false); err != nil {
				return err
			}
			log.Printf("! [Sleeper] league %s: skipped the week %d recap: due %s, over %s ago", league, week, due.Format("2006-01-02 15:04"), recapLate)
			continue
		}

		texts, err := r.client.Recap(ctx, league, week, r.client.LoadNames(ctx, r.db, league))
		switch {
		case errors.Is(err, api.ErrNoScores):
			continue // Sleeper isn't ready yet; the next check tries again
		case errors.Is(err, api.ErrNoGames):
			if _, err := record(ctx, r.db, event{league, "recap", id + "-1"}, []byte(`{"skipped": "no games"}`), due, false); err != nil {
				return err
			}
			continue
		case err != nil:
			log.Printf("[Sleeper] league %s: week %d recap: %v", league, week, err)
			continue
		}
		if err := r.store(ctx, league, id, due, texts); err != nil {
			return err
		}
	}
	return nil
}

// store queues a recap's messages in order, all or none.
func (r *Recaps) store(ctx context.Context, league, id string, due time.Time, texts []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: queue a recap: %w", err)
	}
	defer tx.Rollback(ctx)
	for i, text := range texts {
		data, err := json.Marshal(queuedText{Text: text})
		if err != nil {
			return err
		}
		recorded, err := record(ctx, tx, event{league, "recap", fmt.Sprintf("%s-%d", id, i+1)}, data, due, true)
		if err != nil {
			return err
		}
		if !recorded {
			return nil // another Lys queued it first
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: queue a recap: %w", err)
	}
	return nil
}
