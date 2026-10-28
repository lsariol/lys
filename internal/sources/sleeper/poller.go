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

// Poller is the source that announces each league's trades and waiver claims
// in its community's channels.
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
	rows, err := p.db.Query(ctx, `
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
		if err := p.pollLeague(ctx, l, weeks, week, pub); err != nil {
			log.Printf("[Sleeper] league %s: %v", l.sleeperID, err)
		}
	}
	return nil
}

func (p *Poller) pollLeague(ctx context.Context, l polledLeague, weeks []int, week int, pub sources.Publisher) error {
	var txs []api.Transaction
	for _, w := range weeks {
		got, err := p.client.Transactions(ctx, l.sleeperID, w)
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

		data, err := json.Marshal(tx)
		if err != nil {
			return err
		}
		ev := event{league: l.sleeperID, kind: tx.Type, id: tx.TransactionID}
		recorded, err := record(ctx, p.db, ev, data, time.UnixMilli(tx.StatusUpdated))
		if err != nil {
			return err
		}
		wanted := recorded && !l.firstPoll && ((tx.Type == "trade" && l.trades) || (tx.Type == "waiver" && l.waivers))
		if !wanted {
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
		if err := announce(ctx, p.db, ev, l.communityID, text, pub); err != nil {
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

// names loads the league's teams and the player catalog for messages; what
// can't be loaded falls back to IDs.
func (p *Poller) names(ctx context.Context, sleeperID string) transactionNames {
	rosters, err := p.client.Rosters(ctx, sleeperID)
	if err != nil {
		log.Printf("[Sleeper] league %s rosters: %v", sleeperID, err)
	}
	users, err := p.client.LeagueUsers(ctx, sleeperID)
	if err != nil {
		log.Printf("[Sleeper] league %s users: %v", sleeperID, err)
	}
	catalog, err := p.client.Players(ctx)
	if err != nil {
		log.Printf("[Sleeper] players: %v", err)
	}

	return transactionNames{
		team: func(rosterID int) string {
			if rosters != nil && users != nil {
				for _, r := range *rosters {
					if r.RosterID == rosterID {
						return api.TeamName(r, *users)
					}
				}
			}
			return fmt.Sprintf("Team %d", rosterID)
		},
		player: func(playerID string) string { return api.PlayerName(playerID, catalog) },
	}
}
