package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	api "github.com/lsariol/lys/internal/features/sleeper"
	"github.com/lsariol/lys/internal/sources"
)

// Sender is the source that posts the leagues' queued announcements (rows in
// sleeper_events with send_after set and no published_at), oldest first, one
// per league every gap. Free-agent moves and waiver claims waiting for the
// same team go out as one message. The queue is in the database, so a
// restart loses nothing.
type Sender struct {
	client *api.Client
	db     *pgxpool.Pool
	now    func() time.Time
	every  time.Duration
	gap    time.Duration
}

var _ sources.Source = (*Sender)(nil)

func NewSender(client *api.Client, db *pgxpool.Pool) *Sender {
	return &Sender{client: client, db: db, now: time.Now, every: 5 * time.Second, gap: 90 * time.Second}
}

func (s *Sender) Name() string { return "sleeper announcements" }

// queuedText is the stored message of a lineup reminder or a recap part.
type queuedText struct {
	Text string `json:"text"`
}

// alerts is each kind's switch in sleeper_leagues.notifications, and expires
// how long it may wait in the queue before it's too stale to post.
var (
	alerts  = map[string]string{"trade": "trades", "waiver": "waivers", "free_agent": "freeagents", "lineup_reminder": "lineups", "recap": "recap"}
	expires = map[string]time.Duration{"trade": 12 * time.Hour, "waiver": 12 * time.Hour, "free_agent": 12 * time.Hour, "lineup_reminder": lateBy, "recap": recapLate}
)

func (s *Sender) Run(ctx context.Context, pub sources.Publisher) error {
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		if err := s.send(ctx, pub); err != nil {
			log.Printf("[Sleeper] announcements: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// queued is the next announcement waiting in a league.
type queued struct {
	id        uuid.UUID
	league    string
	kind      string
	data      []byte
	sendAfter time.Time
	community *int64 // nil when the league was unlinked
	enabled   bool
	alerts    map[string]bool // the league's switches; a missing one is on
	lastSent  *time.Time      // the league's last announcement
}

// send posts the next announcement of each league whose gap has passed.
func (s *Sender) send(ctx context.Context, pub sources.Publisher) error {
	now := s.now()
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (e.sleeper_league_id)
		       e.id, e.sleeper_league_id, e.event_type, e.event_data, e.send_after,
		       l.community_id, COALESCE(l.enabled, false), COALESCE(l.notifications, '{}'::jsonb),
		       (SELECT max(p.published_at) FROM lys.sleeper_events p
		        WHERE p.sleeper_league_id = e.sleeper_league_id AND p.send_after IS NOT NULL)
		FROM lys.sleeper_events e
		LEFT JOIN lys.sleeper_leagues l ON l.sleeper_league_id = e.sleeper_league_id
		WHERE e.published_at IS NULL AND e.send_after IS NOT NULL AND e.send_after <= $1
		ORDER BY e.sleeper_league_id, e.send_after, e.occurred_at, e.external_event_id`, now)
	if err != nil {
		return fmt.Errorf("database: read the Sleeper queue: %w", err)
	}
	heads, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (queued, error) {
		var q queued
		var notifications []byte
		err := r.Scan(&q.id, &q.league, &q.kind, &q.data, &q.sendAfter, &q.community, &q.enabled, &notifications, &q.lastSent)
		if err == nil {
			err = json.Unmarshal(notifications, &q.alerts)
		}
		return q, err
	})
	if err != nil {
		return fmt.Errorf("database: read the Sleeper queue: %w", err)
	}

	for _, q := range heads {
		if q.lastSent != nil && now.Sub(*q.lastSent) < s.gap {
			continue
		}
		if err := s.post(ctx, q, now, pub); err != nil {
			log.Printf("[Sleeper] league %s: %v", q.league, err)
		}
	}
	return nil
}

// post sends one announcement, or takes it off the queue when it shouldn't go out.
func (s *Sender) post(ctx context.Context, q queued, now time.Time, pub sources.Publisher) error {
	switch on, set := q.alerts[alerts[q.kind]]; {
	case q.community == nil || !q.enabled:
		return s.drop(ctx, q, "the league is unlinked or disabled", q.id)
	case set && !on:
		return s.drop(ctx, q, "its alert was turned off", q.id)
	case now.Sub(q.sendAfter) > expires[q.kind]:
		return s.drop(ctx, q, fmt.Sprintf("it waited over %s", expires[q.kind]), q.id)
	}

	text, ids, err := s.message(ctx, q)
	if err != nil {
		return err
	}
	if text == "" {
		return s.drop(ctx, q, "there's nothing to say", ids...)
	}
	chs, err := channels(ctx, s.db, *q.community)
	if err != nil {
		return err
	}
	if len(chs) == 0 {
		return s.drop(ctx, q, "the league has no active channel", ids...)
	}

	sent := false
	for _, c := range chs {
		if err := pub.Publish(ctx, core.Outbound{Platform: c.platform, Channel: c.Channel, Text: text}); err != nil {
			log.Printf("[Sleeper] announce %s in %q: %v", q.kind, c.Name, err)
			continue
		}
		sent = true
	}
	if !sent {
		return nil // tried again on the next round
	}
	if _, err := s.db.Exec(ctx, `UPDATE lys.sleeper_events SET published_at = $2 WHERE id = ANY($1)`, ids, now); err != nil {
		return fmt.Errorf("database: mark Sleeper %s announced: %w", q.kind, err)
	}
	return nil
}

// message is the chat message for a queued announcement, and the rows it covers.
func (s *Sender) message(ctx context.Context, q queued) (string, []uuid.UUID, error) {
	switch q.kind {
	case "trade":
		var tx api.Transaction
		if err := json.Unmarshal(q.data, &tx); err != nil {
			return "", nil, fmt.Errorf("read a queued trade: %w", err)
		}
		return api.DescribeTrade(tx, s.client.LoadNames(ctx, s.db, q.league)), []uuid.UUID{q.id}, nil

	case "waiver", "free_agent":
		var head api.Transaction
		if err := json.Unmarshal(q.data, &head); err != nil {
			return "", nil, fmt.Errorf("read a queued %s: %w", q.kind, err)
		}
		if len(head.RosterIDs) == 0 {
			return "", []uuid.UUID{q.id}, nil
		}
		roster := head.RosterIDs[0]
		rows, err := s.db.Query(ctx, `
			SELECT id, event_data FROM lys.sleeper_events
			WHERE sleeper_league_id = $1 AND event_type = $2 AND published_at IS NULL AND send_after IS NOT NULL
			  AND (event_data->'roster_ids'->>0)::int = $3
			ORDER BY occurred_at`, q.league, q.kind, roster)
		if err != nil {
			return "", nil, fmt.Errorf("database: read %s moves: %w", q.kind, err)
		}
		var ids []uuid.UUID
		var txs []api.Transaction
		for rows.Next() {
			var id uuid.UUID
			var data []byte
			if err := rows.Scan(&id, &data); err != nil {
				rows.Close()
				return "", nil, fmt.Errorf("database: read %s moves: %w", q.kind, err)
			}
			var tx api.Transaction
			if err := json.Unmarshal(data, &tx); err != nil {
				rows.Close()
				return "", nil, fmt.Errorf("read a queued %s: %w", q.kind, err)
			}
			ids, txs = append(ids, id), append(txs, tx)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return "", nil, fmt.Errorf("database: read %s moves: %w", q.kind, err)
		}
		return api.DescribeMoves(q.kind, roster, txs, s.client.LoadNames(ctx, s.db, q.league)), ids, nil

	default:
		var t queuedText
		if err := json.Unmarshal(q.data, &t); err != nil {
			return "", nil, fmt.Errorf("read a queued %s: %w", q.kind, err)
		}
		return t.Text, []uuid.UUID{q.id}, nil
	}
}

// drop takes announcements off the queue without posting them, and says why.
func (s *Sender) drop(ctx context.Context, q queued, why string, ids ...uuid.UUID) error {
	if _, err := s.db.Exec(ctx, `UPDATE lys.sleeper_events SET send_after = NULL WHERE id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("database: take Sleeper %s off the queue: %w", q.kind, err)
	}
	log.Printf("! [Sleeper] league %s: didn't post a queued %s: %s", q.league, q.kind, why)
	return nil
}
