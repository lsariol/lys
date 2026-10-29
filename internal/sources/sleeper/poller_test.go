package sleeper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/database"
	api "github.com/lsariol/lys/internal/features/sleeper"
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

// take returns what was sent since the last take.
func (o *outbox) take() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var texts []string
	for _, out := range o.sent {
		texts = append(texts, out.Text)
	}
	o.sent = nil
	return texts
}

// fakeSleeper answers the Sleeper API calls of league "pt-league";
// transactions returns the current list for week 3.
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
		body = `{"11": {"full_name": "Jamie Runner"}, "22": {"full_name": "Sam Catcher"}, "33": {"full_name": "Pat Kicker"}, "44": {"full_name": "Lee Lineman"}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

func (f *fakeSleeper) set(txs ...string) {
	f.mu.Lock()
	f.transactions = "[" + strings.Join(txs, ",") + "]"
	f.mu.Unlock()
}

// move is a completed transaction as Sleeper lists it.
func move(id, kind string, at time.Time, roster int, adds, drops string) string {
	return fmt.Sprintf(`{"transaction_id": %q, "type": %q, "status": "complete", "status_updated": %d, "roster_ids": [%d], "adds": %s, "drops": %s}`,
		id, kind, at.UnixMilli(), roster, adds, drops)
}

// testDB is the migrated test database; the test is skipped without one.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
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
	return pool
}

// TestPollerAndSender needs a Postgres set up by scripts/test-db.sh.
func TestPollerAndSender(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	queued := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM lys.sleeper_events WHERE sleeper_league_id = 'pt-league' AND send_after IS NOT NULL AND published_at IS NULL`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec(`DELETE FROM lys.sleeper_events WHERE sleeper_league_id = 'pt-league'`)
	exec(`DELETE FROM lys.sleeper_leagues WHERE sleeper_league_id = 'pt-league'`)
	exec(`DELETE FROM lys.communities WHERE name = 'SleeperPollTest'`)
	exec(`DELETE FROM lys.platform_users WHERE external_id = 'pt-alice'`)
	var community, alice int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('SleeperPollTest') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_users (platform, external_id, display_name) VALUES ('twitch', 'pt-alice', 'AliceTV') RETURNING id`).Scan(&alice); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO lys.platform_connections (community_id, platform, external_id, name) VALUES ($1, 'twitch', 'pt-ch', 'SleeperPollTest')`, community)
	exec(`INSERT INTO lys.sleeper_leagues (sleeper_league_id, community_id) VALUES ('pt-league', $1)`, community)
	exec(`INSERT INTO lys.sleeper_connections (league_id, sleeper_user_id, sleeper_roster_id, platform_user_id)
		SELECT id, 'u1', 1, $1 FROM lys.sleeper_leagues WHERE sleeper_league_id = 'pt-league'`, alice)

	now := time.Now()
	oldTrade := move("tx1", "trade", now.Add(-48*time.Hour), 1, `{"11": 2, "22": 1}`, `{}`)
	trade := strings.Replace(move("tx2", "trade", now.Add(-5*time.Minute), 1, `{"11": 2, "22": 1}`, `{}`), `"roster_ids": [1]`, `"roster_ids": [1, 2]`, 1)
	pickup := move("fa1", "free_agent", now.Add(-4*time.Minute), 1, `{"33": 1}`, `{"22": 1}`)
	second := move("fa2", "free_agent", now.Add(-3*time.Minute), 1, `{"44": 1}`, `{}`)
	stale := move("fa0", "free_agent", now.Add(-3*time.Hour), 2, `{"44": 2}`, `{}`)

	fake := &fakeSleeper{}
	client := api.New(&http.Client{Transport: fake})
	p := New(client, pool)

	// The first check records what's there without queueing it.
	fake.set(oldTrade)
	if err := p.poll(ctx); err != nil {
		t.Fatal(err)
	}
	if n := queued(); n != 0 {
		t.Fatalf("first check queued %d", n)
	}

	// New moves are queued; a move from before the last check (less an hour) isn't.
	fake.set(oldTrade, trade, pickup, second, stale)
	if err := p.poll(ctx); err != nil {
		t.Fatal(err)
	}
	if n := queued(); n != 3 {
		t.Fatalf("queued %d, want the trade and two pickups", n)
	}

	// The sender posts one per league every 90 seconds, oldest first, and both
	// of Alice's pickups as one message.
	// Other tests' leagues share the database; only this league's queue is sent.
	exec(`UPDATE lys.sleeper_events SET send_after = NULL WHERE sleeper_league_id <> 'pt-league' AND published_at IS NULL`)
	s := NewSender(client, pool)
	pub := &outbox{}
	start := time.Now().Add(time.Second) // after the queue's send_after times
	step := func(after time.Duration) []string {
		t.Helper()
		s.now = func() time.Time { return start.Add(after) }
		if err := s.send(ctx, pub); err != nil {
			t.Fatal(err)
		}
		return pub.take()
	}
	if got := step(0); len(got) != 1 || got[0] != "🏈 Trade: @AliceTV gets Sam Catcher | bob gets Jamie Runner" {
		t.Fatalf("first: %q", got)
	}
	if got := step(10 * time.Second); len(got) != 0 {
		t.Fatalf("within the gap: %q", got)
	}
	if got := step(91 * time.Second); len(got) != 1 || got[0] != "🏈 @AliceTV picked up Pat Kicker and Lee Lineman, dropped Sam Catcher" {
		t.Fatalf("second: %q", got)
	}
	if got := step(200 * time.Second); len(got) != 0 || queued() != 0 {
		t.Fatalf("nothing should be left: %q, %d queued", got, queued())
	}

	// An alert turned off after a move was queued: it's dropped, not posted.
	if _, err := record(ctx, pool, event{"pt-league", "free_agent", "fa3"}, []byte(move("fa3", "free_agent", now, 2, `{"11": 2}`, `{}`)), now, true); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE lys.sleeper_leagues SET notifications = '{"freeagents": false}' WHERE sleeper_league_id = 'pt-league'`)
	if got := step(300 * time.Second); len(got) != 0 || queued() != 0 {
		t.Fatalf("alert off: %q, %d queued", got, queued())
	}

	// With it off, new free-agent moves are recorded but not queued.
	fake.set(oldTrade, trade, pickup, second, stale, move("fa4", "free_agent", time.Now(), 2, `{"22": 2}`, `{}`))
	if err := p.poll(ctx); err != nil {
		t.Fatal(err)
	}
	if n := queued(); n != 0 {
		t.Fatalf("alert off: queued %d", n)
	}

	// A move that waited over 12 hours is too stale to post.
	if _, err := record(ctx, pool, event{"pt-league", "waiver", "w1"}, []byte(move("w1", "waiver", now, 2, `{"11": 2}`, `{}`)), now, true); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE lys.sleeper_events SET send_after = now() - interval '13 hours' WHERE sleeper_league_id = 'pt-league' AND external_event_id = 'w1'`)
	if got := step(400 * time.Second); len(got) != 0 || queued() != 0 {
		t.Fatalf("stale: %q, %d queued", got, queued())
	}

	var published int
	if err := pool.QueryRow(ctx, `SELECT count(published_at) FROM lys.sleeper_events WHERE sleeper_league_id = 'pt-league'`).Scan(&published); err != nil || published != 3 {
		t.Fatalf("published %d, %v", published, err)
	}
}
