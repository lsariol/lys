package sleeper

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
)

// event is something posted once in a league's channels: a trade, a waiver
// claim, a free-agent move, a lineup reminder or a part of a recap. Its row in
// sleeper_events stops a second announcement, across restarts and Lys
// instances; a queued row waits there for the sender (sender.go).
type event struct {
	league string // the Sleeper league ID
	kind   string // trade, waiver, free_agent, lineup_reminder, recap
	id     string // unique within the league and kind
}

func (e event) String() string { return e.kind + " " + e.id }

// querier is a pool or a transaction.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// record notes the event, queued for the sender if queue is set; false if it
// was there already.
func record(ctx context.Context, db querier, e event, data []byte, at time.Time, queue bool) (bool, error) {
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		INSERT INTO lys.sleeper_events (sleeper_league_id, event_type, external_event_id, event_data, occurred_at, send_after)
		VALUES ($1, $2, $3, $4, $5, CASE WHEN $6 THEN now() END)
		ON CONFLICT (sleeper_league_id, event_type, external_event_id) DO NOTHING
		RETURNING id`, e.league, e.kind, e.id, data, at, queue).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("database: record Sleeper %s: %w", e, err)
	}
	return true, nil
}

// channel is one of a community's active channels.
type channel struct {
	platform string
	core.Channel
}

// channels are the community's channels that Lys may post in.
func channels(ctx context.Context, db *pgxpool.Pool, communityID int64) ([]channel, error) {
	rows, err := db.Query(ctx, `
		SELECT platform, external_id, name FROM lys.platform_connections
		WHERE community_id = $1 AND authorized AND enabled AND NOT admin_blocked`, communityID)
	if err != nil {
		return nil, fmt.Errorf("database: read the league's channels: %w", err)
	}
	chs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (channel, error) {
		var c channel
		err := r.Scan(&c.platform, &c.ID, &c.Name)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("database: read the league's channels: %w", err)
	}
	return chs, nil
}
