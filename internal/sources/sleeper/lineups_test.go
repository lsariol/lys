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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/database"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

var (
	thursday = time.Date(2026, 11, 6, 1, 15, 0, 0, time.UTC)  // JAX @ BAL, Thu 8:15 PM ET
	sunday   = time.Date(2026, 11, 8, 14, 30, 0, 0, time.UTC) // CIN @ ATL, Sun 9:30 AM ET (London)
)

func TestReminders(t *testing.T) {
	week := []api.Game{
		{Kickoff: thursday, Away: "JAX", Home: "BAL"},
		{Kickoff: sunday, Away: "CIN", Home: "ATL"},
		{Kickoff: sunday.Add(3*time.Hour + 30*time.Minute), Away: "DAL", Home: "IND"},
	}
	got := reminders("2026", 9, week)
	want := []reminder{
		{thursday.Add(-24 * time.Hour), "2026-w9-1793927700-1d", "🏈 Week 9 starts in 1 day: JAX @ BAL, Thu 8:15 PM ET. Set your lineups!"},
		{thursday.Add(-time.Hour), "2026-w9-1793927700-1h", "🏈 1 hour until Week 9's first game: JAX @ BAL, 8:15 PM ET. Set your lineups!"},
		{sunday.Add(-time.Hour), "2026-w9-1794148200-1h", "🏈 1 hour until Sunday's first game: CIN @ ATL, 9:30 AM ET. Set your lineups!"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d reminders: %+v", len(got), got)
	}
	for i := range want {
		if !got[i].at.Equal(want[i].at) || got[i].id != want[i].id || got[i].text != want[i].text {
			t.Errorf("reminder %d:\n got  %v %s %q\n want %v %s %q", i, got[i].at, got[i].id, got[i].text, want[i].at, want[i].id, want[i].text)
		}
	}

	// A week that starts on Sunday gets its two reminders once, not a second "1 hour".
	if got := reminders("2026", 1, week[1:]); len(got) != 2 || !strings.Contains(got[1].text, "Week 1's first game: CIN @ ATL") {
		t.Fatalf("Sunday-first week: %+v", got)
	}
	if got := reminders("2026", 1, nil); len(got) != 0 {
		t.Fatalf("no games: %+v", got)
	}
}

// espn answers state/nfl as week 9 and ESPN's schedule from the saved week 9
// (week 10 with one game, a week later).
func espn(t *testing.T, seasonType string) *http.Client {
	fixture, err := os.ReadFile("../../features/sleeper/testdata/espn-week9.json")
	if err != nil {
		t.Fatal(err)
	}
	week10 := `{"week":{"number":10},"events":[{"date":"2026-11-13T01:15Z","competitions":[{"competitors":[{"homeAway":"home","team":{"abbreviation":"KC"}},{"homeAway":"away","team":{"abbreviation":"DEN"}}]}]}]}`
	return &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case req.URL.Path == "/v1/state/nfl":
			body = []byte(`{"week": 9, "season": "2026", "season_type": "` + seasonType + `"}`)
		case req.URL.Query().Get("week") == "9":
			body = fixture
		case req.URL.Query().Get("week") == "10":
			body = []byte(week10)
		default:
			t.Errorf("unexpected request %s", req.URL)
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// TestLineups needs a Postgres set up by scripts/test-db.sh.
func TestLineups(t *testing.T) {
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
	exec(`DELETE FROM lys.sleeper_events WHERE sleeper_league_id IN ('lu-on', 'lu-off')`)
	exec(`DELETE FROM lys.sleeper_leagues WHERE sleeper_league_id IN ('lu-on', 'lu-off')`)
	exec(`DELETE FROM lys.communities WHERE name IN ('LineupsOn', 'LineupsOff')`)
	league := func(name, id, channel string, lineups bool) {
		var community int64
		if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ($1) RETURNING id`, name).Scan(&community); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO lys.platform_connections (community_id, platform, external_id, name) VALUES ($1, 'twitch', $2, $3)`, community, channel, name)
		exec(`INSERT INTO lys.sleeper_leagues (sleeper_league_id, community_id, notifications) VALUES ($1, $2, jsonb_build_object('lineups', $3::boolean))`, id, community, lineups)
	}
	league("LineupsOn", "lu-on", "lu-ch-on", true)
	league("LineupsOff", "lu-off", "lu-ch-off", false)

	l := NewLineups(api.New(espn(t, "regular")), pool)
	// tick runs a check at the time, and returns the reminders it queued.
	seen := 0
	tick := func(at time.Time) ([]string, time.Time) {
		t.Helper()
		l.now = func() time.Time { return at }
		next, err := l.tick(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var off int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM lys.sleeper_events WHERE sleeper_league_id = 'lu-off'`).Scan(&off); err != nil || off != 0 {
			t.Fatalf("a league with lineup reminders off got %d, %v", off, err)
		}
		rows, err := pool.Query(ctx, `SELECT event_data->>'text' FROM lys.sleeper_events
			WHERE sleeper_league_id = 'lu-on' AND event_type = 'lineup_reminder' AND send_after IS NOT NULL ORDER BY created_at, occurred_at`)
		if err != nil {
			t.Fatal(err)
		}
		texts, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		texts, seen = texts[seen:], len(texts)
		return texts, next
	}

	if sent, next := tick(thursday.Add(-30 * time.Hour)); len(sent) != 0 || !next.Equal(thursday.Add(-24*time.Hour)) {
		t.Fatalf("early: sent %q, next %v", sent, next)
	}
	if sent, _ := tick(thursday.Add(-24*time.Hour + 5*time.Minute)); len(sent) != 1 || !strings.Contains(sent[0], "starts in 1 day") {
		t.Fatalf("1 day before: %q", sent)
	}
	if sent, _ := tick(thursday.Add(-24*time.Hour + 6*time.Minute)); len(sent) != 0 {
		t.Fatalf("sent twice: %q", sent)
	}
	// A restart 40 minutes after the hour reminder is due: too late to send it.
	if sent, next := tick(thursday.Add(-20 * time.Minute)); len(sent) != 0 || !next.Equal(sunday.Add(-time.Hour)) {
		t.Fatalf("late: sent %q, next %v", sent, next)
	}
	if sent, _ := tick(sunday.Add(-time.Hour + time.Minute)); len(sent) != 1 || !strings.Contains(sent[0], "Sunday's first game: CIN @ ATL") {
		t.Fatalf("Sunday: %q", sent)
	}
	// Week 10's "1 day" falls in week 9, and is found too.
	if _, next := tick(sunday); !next.Equal(time.Date(2026, 11, 12, 1, 15, 0, 0, time.UTC)) {
		t.Fatalf("next after Sunday: %v", next)
	}

	if seen != 2 {
		t.Fatalf("queued %d reminders, want 2", seen)
	}

	// Out of season, nothing is asked of ESPN and nothing is sent.
	off := NewLineups(api.New(espn(t, "off")), pool)
	if next, err := off.tick(ctx); err != nil || !next.IsZero() {
		t.Fatalf("off season: next %v, %v", next, err)
	}
}
