package sleeper

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// recapFakes answers as week 9 of 2026: ESPN's saved week 9 schedule, and for
// every league two matchups a week (the same each week), with four teams.
func recapFakes(t *testing.T) *http.Client {
	fixture, err := os.ReadFile("../../features/sleeper/testdata/espn-week9.json")
	if err != nil {
		t.Fatal(err)
	}
	const matchups = `[
		{"roster_id": 1, "matchup_id": 1, "points": 132.4, "starters": ["11"], "starters_points": [30.5], "players": ["11", "22"], "players_points": {"11": 30.5, "22": 12}},
		{"roster_id": 2, "matchup_id": 1, "points": 98.1, "starters": ["33"], "starters_points": [10], "players": ["33"], "players_points": {"33": 10}},
		{"roster_id": 3, "matchup_id": 2, "points": 120.6},
		{"roster_id": 4, "matchup_id": 2, "points": 119.9}]`
	return &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		var body string
		p := req.URL.Path
		switch {
		case p == "/v1/state/nfl":
			body = `{"week": 9, "season": "2026", "season_type": "regular"}`
		case strings.Contains(p, "/matchups/"):
			body = matchups
		case strings.HasSuffix(p, "/rosters"):
			body = `[{"roster_id": 1, "owner_id": "u1"}, {"roster_id": 2, "owner_id": "u2"}, {"roster_id": 3, "owner_id": "u3"}, {"roster_id": 4, "owner_id": "u4"}]`
		case strings.HasSuffix(p, "/users"):
			body = `[{"user_id": "u1", "display_name": "alice"}, {"user_id": "u2", "display_name": "bob"}, {"user_id": "u3", "display_name": "cy"}, {"user_id": "u4", "display_name": "dee"}]`
		case strings.HasPrefix(p, "/v1/league/"):
			body = `{"league_id": "x", "season": "2026", "settings": {"playoff_week_start": 15}}`
		case p == "/v1/players/nfl":
			body = `{"11": {"full_name": "Jamie Runner"}}`
		case req.URL.Query().Get("week") == "9":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(fixture)), Header: make(http.Header)}, nil
		default:
			body = `{"week": {"number": 8}, "events": []}` // no schedule: that week is skipped
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

// TestRecaps needs a Postgres set up by scripts/test-db.sh.
func TestRecaps(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	leagues := []string{"rc-on", "rc-off", "rc-late"}
	exec(`DELETE FROM lys.sleeper_events WHERE sleeper_league_id = ANY($1)`, leagues)
	exec(`DELETE FROM lys.sleeper_leagues WHERE sleeper_league_id = ANY($1)`, leagues)
	exec(`DELETE FROM lys.communities WHERE name LIKE 'RecapTest%'`)
	league := func(id string, recap bool) {
		t.Helper()
		var community int64
		if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ($1) RETURNING id`, "RecapTest "+id).Scan(&community); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO lys.sleeper_leagues (sleeper_league_id, community_id, notifications) VALUES ($1, $2, jsonb_build_object('recap', $3::boolean))`, id, community, recap)
	}
	league("rc-on", true)
	league("rc-off", false)

	client := api.New(recapFakes(t))
	games, err := client.Games(ctx, "2026", 9)
	if err != nil {
		t.Fatal(err)
	}
	due := api.RecapDue(games)
	if due.In(api.Eastern).Format("Mon 15:04") != "Tue 10:00" {
		t.Fatalf("due %s", due.In(api.Eastern))
	}

	r := NewRecaps(client, pool)
	recaps := func(id string, at time.Time) []string {
		t.Helper()
		r.now = func() time.Time { return at }
		if err := r.tick(ctx); err != nil {
			t.Fatal(err)
		}
		rows, err := pool.Query(ctx, `SELECT COALESCE(event_data->>'text', event_data->>'skipped') FROM lys.sleeper_events
			WHERE sleeper_league_id = $1 AND event_type = 'recap' ORDER BY external_event_id`, id)
		if err != nil {
			t.Fatal(err)
		}
		texts, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return texts
	}

	if got := recaps("rc-on", due.Add(-time.Minute)); len(got) != 0 {
		t.Fatalf("before it's due: %q", got)
	}
	got := recaps("rc-on", due.Add(time.Minute))
	if len(got) != 3 || got[0] != "🏈 Week 9 results: alice 132.40 def. bob 98.10 · cy 120.60 def. dee 119.90" ||
		!strings.HasPrefix(got[1], "🏈 Week 9 highlights: top score alice (132.40)") ||
		!strings.HasPrefix(got[2], "🏈 Standings after week 9: 1. alice 9-0 · 2. cy 9-0") {
		t.Fatalf("due: %q", got)
	}
	if got := recaps("rc-on", due.Add(2*time.Minute)); len(got) != 3 {
		t.Fatalf("made twice: %q", got)
	}
	if got := recaps("rc-off", due.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("recaps off: %q", got)
	}

	// A league first seen more than a day late is skipped, not posted.
	league("rc-late", true)
	if got := recaps("rc-late", due.Add(25*time.Hour)); len(got) != 1 || got[0] != "late" {
		t.Fatalf("late: %q", got)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lys.sleeper_events WHERE sleeper_league_id = 'rc-late' AND send_after IS NOT NULL`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("late recap queued: %d, %v", queued, err)
	}
}
