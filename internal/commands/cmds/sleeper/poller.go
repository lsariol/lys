package sleeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/sources"
)

// Poller is the source that announces each league's trades and waiver claims
// in its community's channels.
type Poller struct {
	sl    *Sleeper
	every time.Duration
}

var _ sources.Source = (*Poller)(nil)

func NewPoller(http *http.Client, db *pgxpool.Pool) *Poller {
	return &Poller{sl: New(http, db), every: 10 * time.Minute}
}

func (p *Poller) Name() string { return "sleeper" }

func (p *Poller) Run(ctx context.Context, pub sources.Publisher) error {
	t := time.NewTicker(p.every)
	defer t.Stop()
	for {
		if err := p.poll(ctx, pub); err != nil {
			log.Printf("[Sleeper] %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// polledLeague is a league to check, with its alert settings.
type polledLeague struct {
	id          uuid.UUID
	sleeperID   string
	communityID int64
	firstPoll   bool // nothing recorded yet: record what exists without announcing it
	trades      bool
	waivers     bool
}

func (p *Poller) poll(ctx context.Context, pub sources.Publisher) error {
	rows, err := p.sl.db.Query(ctx, `
		SELECT id, sleeper_league_id, community_id, last_polled_at IS NULL,
		       COALESCE((notifications->>'trades')::boolean, true), COALESCE((notifications->>'waivers')::boolean, true)
		FROM lys.sleeper_leagues
		WHERE enabled AND community_id IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("database: read Sleeper leagues: %w", err)
	}
	leagues, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (polledLeague, error) {
		var l polledLeague
		err := r.Scan(&l.id, &l.sleeperID, &l.communityID, &l.firstPoll, &l.trades, &l.waivers)
		return l, err
	})
	if err != nil {
		return fmt.Errorf("database: read Sleeper leagues: %w", err)
	}
	if len(leagues) == 0 {
		return nil
	}

	state, err := getNFLState(ctx, p.sl)
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
		if err := p.pollLeague(ctx, l, weeks, week, pub); err != nil {
			log.Printf("[Sleeper] league %s: %v", l.sleeperID, err)
		}
	}
	return nil
}

func (p *Poller) pollLeague(ctx context.Context, l polledLeague, weeks []int, week int, pub sources.Publisher) error {
	var txs []SleeperTransaction
	for _, w := range weeks {
		got, err := getTransactions(l.sleeperID, w, ctx, p.sl)
		if err != nil {
			return err
		}
		txs = append(txs, got...)
	}
	sort.Slice(txs, func(i, j int) bool { return txs[i].StatusUpdated < txs[j].StatusUpdated })

	var names *transactionNames
	var latest int64
	for _, tx := range txs {
		if tx.Status != "complete" || (tx.Type != "trade" && tx.Type != "waiver") {
			continue
		}
		latest = max(latest, tx.StatusUpdated)

		recorded, err := p.record(ctx, l, tx)
		if err != nil {
			return err
		}
		announce := recorded && !l.firstPoll && ((tx.Type == "trade" && l.trades) || (tx.Type == "waiver" && l.waivers))
		if !announce {
			continue
		}

		if names == nil {
			n := p.names(ctx, l.sleeperID)
			names = &n
		}
		text, ok := describeTransaction(tx, *names)
		if !ok {
			continue
		}
		if err := p.announce(ctx, l, tx, text, pub); err != nil {
			return err
		}
	}

	_, err := p.sl.db.Exec(ctx, `
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

// record notes a transaction in sleeper_events; false if it was already there.
func (p *Poller) record(ctx context.Context, l polledLeague, tx SleeperTransaction) (bool, error) {
	data, err := json.Marshal(tx)
	if err != nil {
		return false, err
	}
	var id uuid.UUID
	err = p.sl.db.QueryRow(ctx, `
		INSERT INTO lys.sleeper_events (sleeper_league_id, event_type, external_event_id, event_data, occurred_at)
		VALUES ($1, $2, $3, $4, to_timestamp($5::bigint / 1000.0))
		ON CONFLICT (sleeper_league_id, event_type, external_event_id) DO NOTHING
		RETURNING id`, l.sleeperID, tx.Type, tx.TransactionID, data, tx.StatusUpdated).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("database: record Sleeper %s %s: %w", tx.Type, tx.TransactionID, err)
	}
	return true, nil
}

// announce posts text in every active channel of the league's community.
func (p *Poller) announce(ctx context.Context, l polledLeague, tx SleeperTransaction, text string, pub sources.Publisher) error {
	rows, err := p.sl.db.Query(ctx, `
		SELECT platform, external_id, name FROM lys.platform_connections
		WHERE community_id = $1 AND authorized AND enabled AND NOT admin_blocked`, l.communityID)
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
			log.Printf("[Sleeper] announce %s %s in %q: %v", tx.Type, tx.TransactionID, c.Name, err)
			continue
		}
		sent = true
	}
	if sent {
		if _, err := p.sl.db.Exec(ctx, `UPDATE lys.sleeper_events SET published_at = now()
			WHERE sleeper_league_id = $1 AND event_type = $2 AND external_event_id = $3`, l.sleeperID, tx.Type, tx.TransactionID); err != nil {
			return fmt.Errorf("database: mark Sleeper %s %s announced: %w", tx.Type, tx.TransactionID, err)
		}
	}
	return nil
}

// names loads the league's teams and the player catalog for messages; what
// can't be loaded falls back to IDs.
func (p *Poller) names(ctx context.Context, sleeperID string) transactionNames {
	rosters, err := getRosters(sleeperID, ctx, p.sl)
	if err != nil {
		log.Printf("[Sleeper] league %s rosters: %v", sleeperID, err)
	}
	users, err := getPlayersInLeague(sleeperID, ctx, p.sl)
	if err != nil {
		log.Printf("[Sleeper] league %s users: %v", sleeperID, err)
	}
	catalog, err := getPlayerCatalog(ctx, p.sl)
	if err != nil {
		log.Printf("[Sleeper] players: %v", err)
	}

	return transactionNames{
		team: func(rosterID int) string {
			if rosters != nil && users != nil {
				for _, r := range *rosters {
					if r.RosterID == rosterID {
						return getTeamName(r, *users)
					}
				}
			}
			return fmt.Sprintf("Team %d", rosterID)
		},
		player: func(playerID string) string { return getPlayerName(playerID, catalog) },
	}
}
