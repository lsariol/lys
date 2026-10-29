package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/lsariol/lys/internal/features/sleeper"
	"github.com/lsariol/lys/internal/sources"
)

// Poller is the source that checks each league's trades, waiver claims and
// free-agent moves, and queues the new ones for the sender (sender.go).
type Poller struct {
	client *api.Client
	db     *pgxpool.Pool
	every  time.Duration
}

var _ sources.Source = (*Poller)(nil)

func New(client *api.Client, db *pgxpool.Pool) *Poller {
	return &Poller{client: client, db: db, every: 10 * time.Minute}
}

func (p *Poller) Name() string { return "sleeper" }

func (p *Poller) Run(ctx context.Context, _ sources.Publisher) error {
	t := time.NewTicker(p.every)
	defer t.Stop()
	for {
		if err := p.poll(ctx); err != nil {
			log.Printf("[Sleeper] %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// catchUp is how far before a league's last check a new move may have
// happened and still be announced: Sleeper can list a move a little after it
// happened. Anything older is recorded quietly, such as the moves already
// there when a kind of move is first tracked.
const catchUp = time.Hour

// moveKinds are the transactions that are announced, with their alert's name.
var moveKinds = map[string]string{"trade": "trades", "waiver": "waivers", "free_agent": "freeagents"}

// polledLeague is a league to check, with its alert settings.
type polledLeague struct {
	id         uuid.UUID
	sleeperID  string
	lastPolled *time.Time // nil before its first check, which only records what exists
	alerts     map[string]bool
}

func (p *Poller) poll(ctx context.Context) error {
	rows, err := p.db.Query(ctx, `
		SELECT id, sleeper_league_id, last_polled_at,
		       COALESCE((notifications->>'trades')::boolean, true), COALESCE((notifications->>'waivers')::boolean, true),
		       COALESCE((notifications->>'freeagents')::boolean, true)
		FROM lys.sleeper_leagues
		WHERE enabled AND community_id IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("database: read Sleeper leagues: %w", err)
	}
	leagues, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (polledLeague, error) {
		var l polledLeague
		var trades, waivers, freeAgents bool
		err := r.Scan(&l.id, &l.sleeperID, &l.lastPolled, &trades, &waivers, &freeAgents)
		l.alerts = map[string]bool{"trades": trades, "waivers": waivers, "freeagents": freeAgents}
		return l, err
	})
	if err != nil {
		return fmt.Errorf("database: read Sleeper leagues: %w", err)
	}
	if len(leagues) == 0 {
		return nil
	}

	state, err := p.client.NFLState(ctx)
	if err != nil {
		return fmt.Errorf("Sleeper: NFL state: %w", err)
	}
	// This week and last, so moves processed around the week change aren't missed.
	week := max(state.Week, 1)
	weeks := []int{week}
	if week > 1 {
		weeks = append(weeks, week-1)
	}

	for _, l := range leagues {
		if err := p.pollLeague(ctx, l, weeks, week); err != nil {
			log.Printf("[Sleeper] league %s: %v", l.sleeperID, err)
		}
	}
	return nil
}

func (p *Poller) pollLeague(ctx context.Context, l polledLeague, weeks []int, week int) error {
	var txs []api.Transaction
	for _, w := range weeks {
		got, err := p.client.Transactions(ctx, l.sleeperID, w)
		if err != nil {
			return err
		}
		txs = append(txs, got...)
	}
	sort.Slice(txs, func(i, j int) bool { return txs[i].StatusUpdated < txs[j].StatusUpdated })

	var latest int64
	for _, tx := range txs {
		alert, ok := moveKinds[tx.Type]
		if tx.Status != "complete" || !ok {
			continue
		}
		latest = max(latest, tx.StatusUpdated)

		data, err := json.Marshal(tx)
		if err != nil {
			return err
		}
		at := time.UnixMilli(tx.StatusUpdated)
		queue := l.lastPolled != nil && l.alerts[alert] && at.After(l.lastPolled.Add(-catchUp))
		ev := event{league: l.sleeperID, kind: tx.Type, id: tx.TransactionID}
		if _, err := record(ctx, p.db, ev, data, at, queue); err != nil {
			return err
		}
	}

	_, err := p.db.Exec(ctx, `
		UPDATE lys.sleeper_leagues
		SET last_polled_at = now(), current_week = $2,
		    last_transaction_at = GREATEST(last_transaction_at, CASE WHEN $3::bigint > 0 THEN to_timestamp($3::bigint / 1000.0) END),
		    updated_at = now()
		WHERE id = $1`, l.id, week, latest)
	if err != nil {
		return fmt.Errorf("database: record the Sleeper poll: %w", err)
	}
	return nil
}
