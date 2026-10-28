package sleeper

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/sources"
)

// event is something announced once in a league's channels: a trade, a waiver
// claim or a lineup reminder. Its row in sleeper_events stops a second
// announcement, across restarts and Lys instances.
type event struct {
	league string // the Sleeper league ID
	kind   string // trade, waiver, lineup_reminder
	id     string // unique within the league and kind
}

func (e event) String() string { return e.kind + " " + e.id }

// record notes the event; false if it was there already.
func record(ctx context.Context, db *pgxpool.Pool, e event, data []byte, at time.Time) (bool, error) {
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		INSERT INTO lys.sleeper_events (sleeper_league_id, event_type, external_event_id, event_data, occurred_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (sleeper_league_id, event_type, external_event_id) DO NOTHING
		RETURNING id`, e.league, e.kind, e.id, data, at).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("database: record Sleeper %s: %w", e, err)
	}
	return true, nil
}

// announce posts text in every active channel of the community, and marks the
// event published if it reached one.
func announce(ctx context.Context, db *pgxpool.Pool, e event, communityID int64, text string, pub sources.Publisher) error {
	rows, err := db.Query(ctx, `
		SELECT platform, external_id, name FROM lys.platform_connections
		WHERE community_id = $1 AND authorized AND enabled AND NOT admin_blocked`, communityID)
	if err != nil {
		return fmt.Errorf("database: read the league's channels: %w", err)
	}
	type channel struct {
		platform string
		core.Channel
	}
	channels, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (channel, error) {
		var c channel
		err := r.Scan(&c.platform, &c.ID, &c.Name)
		return c, err
	})
	if err != nil {
		return fmt.Errorf("database: read the league's channels: %w", err)
	}

	sent := false
	for _, c := range channels {
		if err := pub.Publish(ctx, core.Outbound{Platform: c.platform, Channel: c.Channel, Text: text}); err != nil {
			log.Printf("[Sleeper] announce %s in %q: %v", e, c.Name, err)
			continue
		}
		sent = true
	}
	if sent {
		if _, err := db.Exec(ctx, `UPDATE lys.sleeper_events SET published_at = now()
			WHERE sleeper_league_id = $1 AND event_type = $2 AND external_event_id = $3`, e.league, e.kind, e.id); err != nil {
			return fmt.Errorf("database: mark Sleeper %s announced: %w", e, err)
		}
	}
	return nil
}
