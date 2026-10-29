package sleeper

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/database"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// fakeLeague answers the Sleeper calls of a three-team league, "tt-league".
type fakeLeague struct{}

func (fakeLeague) RoundTrip(req *http.Request) (*http.Response, error) {
	body := "[]"
	switch req.URL.Path {
	case "/v1/league/tt-league/rosters":
		body = `[{"roster_id": 1, "owner_id": "u1"}, {"roster_id": 2, "owner_id": "u2"}, {"roster_id": 3, "owner_id": "u3"}]`
	case "/v1/league/tt-league/users":
		body = `[{"user_id": "u1", "display_name": "alice", "metadata": {"team_name": "Alice FC"}}, {"user_id": "u2", "display_name": "bob"}, {"user_id": "u3", "display_name": "cy"}]`
	case "/v1/players/nfl":
		body = `{"11": {"full_name": "Jamie Runner"}, "22": {"full_name": "Sam Catcher"}, "33": {"full_name": "Pat Kicker"}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

// TestTradesAndLinks needs a Postgres set up by scripts/test-db.sh.
func TestTradesAndLinks(t *testing.T) {
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
	exec(`DELETE FROM lys.sleeper_events WHERE sleeper_league_id = 'tt-league'`)
	exec(`DELETE FROM lys.sleeper_leagues WHERE sleeper_league_id = 'tt-league'`)
	exec(`DELETE FROM lys.communities WHERE name = 'TradesTest'`)
	exec(`DELETE FROM lys.platform_users WHERE external_id IN ('tt-alice', 'tt-bob')`)

	var community, alice, bob int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('TradesTest') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	for id, name := range map[*int64]string{&alice: "AliceTV", &bob: "BobTV"} {
		if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_users (platform, external_id, display_name) VALUES ('twitch', $1, $2) RETURNING id`,
			"tt-"+strings.ToLower(strings.TrimSuffix(name, "TV")), name).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO lys.sleeper_leagues (sleeper_league_id, league_name, season, community_id) VALUES ('tt-league', 'Test League', '2026', $1)`, community)
	exec(`INSERT INTO lys.sleeper_events (sleeper_league_id, event_type, external_event_id, event_data, occurred_at) VALUES
		('tt-league', 'trade', 't1', '{"type": "trade", "roster_ids": [1, 2], "adds": {"11": 2, "22": 1}}', '2026-10-01 12:00-04'),
		('tt-league', 'trade', 't2', '{"type": "trade", "roster_ids": [2, 3], "adds": {"33": 2}}', '2026-10-02 12:00-04'),
		('tt-league', 'waiver', 'w1', '{"type": "waiver", "roster_ids": [1], "adds": {"33": 1}}', '2026-10-03 12:00-04')`)

	sl := New(api.New(&http.Client{Transport: fakeLeague{}}), pool)
	run := func(user string, platformUser int64, args ...string) commands.Reply {
		t.Helper()
		e := commands.Request{Args: args, Prefix: "!", Connection: core.Connection{CommunityID: community}}
		e.Channel.ID, e.User.ID = "tt-channel", "tt-"+user
		e.User.Name, e.Identity.PlatformUserID = user, platformUser
		reply, err := sl.Run(ctx, e)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return reply
	}

	// Linking: one team per person, and one person per team.
	if got := run("AliceTV", alice, "link", "alice").Text; got != "🏈 @AliceTV is now linked to Alice FC (alice on Sleeper)." {
		t.Fatalf("link: %q", got)
	}
	if got := run("AliceTV", alice, "link", "bob").Text; !strings.Contains(got, "you're already linked to alice") {
		t.Fatalf("second link: %q", got)
	}
	if got := run("BobTV", bob, "link", "user", "alice").Text; !strings.Contains(got, "already linked to @AliceTV. A moderator can remove the link with !ff unlink @AliceTV") {
		t.Fatalf("taken team: %q", got)
	}
	if got := run("BobTV", bob, "unlink", "@AliceTV").Text; got != "Only moderators can unlink someone else." {
		t.Fatalf("unlink by a viewer: %q", got)
	}

	// Trades, newest first, with @names for linked teams.
	reply := run("BobTV", bob, "trades")
	if reply.Text != "🏈 Trade, Oct 2: bob gets Pat Kicker | cy gets nothing" ||
		len(reply.More) != 1 || reply.More[0] != "🏈 Trade, Oct 1: @AliceTV gets Sam Catcher | bob gets Jamie Runner" {
		t.Fatalf("trades: %q %q", reply.Text, reply.More)
	}
	if reply := run("BobTV", bob, "trades", "@alicetv"); !strings.HasPrefix(reply.Text, "🏈 Trade, Oct 1:") || len(reply.More) != 0 {
		t.Fatalf("Alice's trades: %q %q", reply.Text, reply.More)
	}

	// A moderator removes Alice's link; then her team shows by name.
	e := commands.Request{Args: []string{"unlink", "@AliceTV"}, Prefix: "!", Connection: core.Connection{CommunityID: community}}
	e.Channel.ID, e.User.ID = "tt-channel", "tt-mod"
	e.User.Name, e.User.Roles, e.Identity.PlatformUserID = "Mod", core.RoleModerator, bob
	if reply, err := sl.Run(ctx, e); err != nil || reply.Text != "🏈 @AliceTV is no longer linked to a team in Test League." {
		t.Fatalf("unlink by a moderator: %q, %v", reply.Text, err)
	}
	if got := run("BobTV", bob, "trades").More[0]; !strings.Contains(got, "Alice FC gets Sam Catcher") {
		t.Fatalf("after unlinking: %q", got)
	}
}
