package sleeper

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/database"
)

type outbox struct {
	mu   sync.Mutex
	sent []core.Outbound
}

func (o *outbox) Publish(ctx context.Context, out core.Outbound) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, out)
	return nil
}

// fakeSleeper answers the Sleeper API calls the poller makes; transactions
// returns the current list for week 3.
type fakeSleeper struct {
	mu           sync.Mutex
	transactions string
}

func (f *fakeSleeper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := "[]"
	switch p := req.URL.Path; {
	case p == "/v1/state/nfl":
		body = `{"week": 3, "season": "2026", "season_type": "regular"}`
	case p == "/v1/league/pt-league/transactions/3":
		body = f.transactions
	case p == "/v1/league/pt-league/rosters":
		body = `[{"roster_id": 1, "owner_id": "u1"}, {"roster_id": 2, "owner_id": "u2"}]`
	case p == "/v1/league/pt-league/users":
		body = `[{"user_id": "u1", "display_name": "alice", "metadata": {"team_name": "Alice FC"}}, {"user_id": "u2", "display_name": "bob"}]`
	case p == "/v1/players/nfl":
		body = `{"11": {"full_name": "Jamie Runner", "position": "RB", "team": "KC"}, "22": {"full_name": "Sam Catcher", "position": "WR", "team": "BUF"}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

func (f *fakeSleeper) set(body string) {
	f.mu.Lock()
	f.transactions = body
	f.mu.Unlock()
}

const trade1 = `{"transaction_id": "tx1", "type": "trade", "status": "complete", "status_updated": 1790000000000, "roster_ids": [1, 2], "adds": {"11": 2, "22": 1}}`
const trade2 = `{"transaction_id": "tx2", "type": "trade", "status": "complete", "status_updated": 1790000100000, "roster_ids": [1, 2], "adds": {"11": 1, "22": 2}}`

// TestPoller needs a Postgres set up by scripts/test-db.sh.
func TestPoller(t *testing.T) {
	migratorURL, appURL := os.Getenv("LYS_TEST_MIGRATOR_DATABASE_URL"), os.Getenv("LYS_TEST_DATABASE_URL")
	if migratorURL == "" || appURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL and LYS_TEST_DATABASE_URL aren't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()
	if err := database.Migrate(ctx, migratorURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DELETE FROM lys.sleeper_events WHERE sleeper_league_id = 'pt-league'`)
	exec(`DELETE FROM lys.sleeper_leagues WHERE sleeper_league_id = 'pt-league'`)
	exec(`DELETE FROM lys.communities WHERE name = 'SleeperPollTest'`)
	var community int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('SleeperPollTest') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO lys.platform_connections (community_id, platform, external_id, name) VALUES ($1, 'twitch', 'pt-ch', 'SleeperPollTest')`, community)
	exec(`INSERT INTO lys.sleeper_leagues (twitch_channel_id, sleeper_league_id, community_id) VALUES ('pt-ch', 'pt-league', $1)`, community)

	api := &fakeSleeper{}
	api.set("[" + trade1 + "]")
	p := NewPoller(&http.Client{Transport: api}, pool)
	pub := &outbox{}

	// The first check records what's there without announcing it.
	if err := p.poll(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if len(pub.sent) != 0 {
		t.Fatalf("first check announced: %+v", pub.sent)
	}

	api.set("[" + trade1 + "," + trade2 + "]")
	if err := p.poll(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if len(pub.sent) != 1 || pub.sent[0].Channel.ID != "pt-ch" ||
		pub.sent[0].Text != "🏈 Trade: Alice FC gets Jamie Runner | bob gets Sam Catcher" {
		t.Fatalf("second check: %+v", pub.sent)
	}

	if err := p.poll(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if len(pub.sent) != 1 {
		t.Fatalf("announced again: %+v", pub.sent)
	}

	// With trades turned off, a new trade is recorded but not announced.
	exec(`UPDATE lys.sleeper_leagues SET notifications = '{"trades": false}' WHERE sleeper_league_id = 'pt-league'`)
	api.set("[" + trade1 + "," + trade2 + `,{"transaction_id": "tx3", "type": "trade", "status": "complete", "status_updated": 1790000200000, "roster_ids": [1, 2], "adds": {"11": 2}}]`)
	if err := p.poll(ctx, pub); err != nil {
		t.Fatal(err)
	}
	var recorded, published int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(published_at) FROM lys.sleeper_events WHERE sleeper_league_id = 'pt-league'`).Scan(&recorded, &published); err != nil {
		t.Fatal(err)
	}
	if len(pub.sent) != 1 || recorded != 3 || published != 1 {
		t.Fatalf("trades off: sent %d, recorded %d, published %d", len(pub.sent), recorded, published)
	}
}
