package connections

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/botaccounts"
	"github.com/lsariol/lys/internal/database"
)

// open needs a Postgres set up by scripts/test-db.sh; it removes this test's rows first.
func open(t *testing.T) *pgxpool.Pool {
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
	for _, q := range []string{
		`DELETE FROM lys.communities WHERE id IN (SELECT community_id FROM lys.platform_connections WHERE external_id LIKE 'conntest-%')`,
		`DELETE FROM lys.bot_accounts WHERE external_id LIKE 'conntest-%'`,
		`DELETE FROM lys.twitch_channels WHERE user_id LIKE 'conntest-%'`, // left by older releases' tests (until the old tables are dropped)
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func TestEnrollBlockPrefix(t *testing.T) {
	pool := open(t)
	ctx := context.Background()
	s := New(pool)

	c, created, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-1", Name: "ConnTest", Login: "conntest"})
	if err != nil || !created || !c.Active() || c.Prefix != "!" {
		t.Fatalf("enroll: %+v created %v err %v", c, created, err)
	}
	if _, created, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-1", Name: "ConnTestRenamed"}); err != nil || created {
		t.Fatalf("second enroll: created %v err %v", created, err)
	}

	var timesJoined, oldRows int
	if err := pool.QueryRow(ctx, `SELECT times_joined FROM lys.twitch_connection_details WHERE connection_id = $1`, c.ID).Scan(&timesJoined); err != nil || timesJoined != 2 {
		t.Fatalf("times joined: %d, %v", timesJoined, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lys.twitch_channels WHERE user_id = 'conntest-1'`).Scan(&oldRows); err != nil || oldRows != 0 {
		t.Fatalf("the old twitch_channels table was written: %d rows, %v", oldRows, err)
	}

	if roles, err := s.BotRoles(ctx); err != nil || roles["conntest-1"] {
		t.Fatalf("a new channel's bot role: %v, %v", roles["conntest-1"], err)
	}
	if err := s.SetBotRole(ctx, "conntest-1", true); err != nil {
		t.Fatal(err)
	}
	if roles, err := s.BotRoles(ctx); err != nil || !roles["conntest-1"] {
		t.Fatalf("the bot's role wasn't kept: %v, %v", roles["conntest-1"], err)
	}

	if err := s.SetPrefix(ctx, "twitch", "conntest-1", "?"); err != nil {
		t.Fatal(err)
	}
	found, err := s.Find(ctx, "twitch", "conntestrenamed")
	if err != nil || found.ID != c.ID || found.Prefix != "?" {
		t.Fatalf("find by name: %+v, %v", found, err)
	}
	if err := s.SetPrefix(ctx, "twitch", "conntest-missing", "?"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("prefix for a missing channel: %v", err)
	}

	inDirectory := func() bool {
		chans, err := s.Channels(ctx, "twitch")
		if err != nil {
			t.Fatal(err)
		}
		for _, ch := range chans {
			if ch.ID == "conntest-1" {
				return true
			}
		}
		return false
	}
	if !inDirectory() {
		t.Fatal("an enrolled channel isn't in the directory")
	}

	if err := s.SetBlocked(ctx, c.ID, true, "spam"); err != nil {
		t.Fatal(err)
	}
	if inDirectory() {
		t.Fatal("a blocked channel is still in the directory")
	}
	if blocked, _ := s.Find(ctx, "twitch", "conntest-1"); !blocked.AdminBlocked || blocked.BlockReason != "spam" {
		t.Fatalf("blocked: %+v", blocked)
	}

	if err := s.SetBlocked(ctx, c.ID, false, ""); err != nil || !inDirectory() {
		t.Fatalf("unblock: %v", err)
	}

	var events []string
	rows, err := pool.Query(ctx, `SELECT event_type::text FROM lys.connection_events WHERE connection_id = $1 ORDER BY id`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if want := []string{"enrolled", "admin_blocked", "admin_unblocked"}; len(events) != len(want) || events[0] != want[0] || events[1] != want[1] || events[2] != want[2] {
		t.Fatalf("events: %v, want %v", events, want)
	}
}

func TestBotAccounts(t *testing.T) {
	pool := open(t)
	ctx := context.Background()
	b := botaccounts.New(pool)

	if _, found, err := b.Default(ctx, "conntest-platform"); err != nil || found {
		t.Fatalf("empty: found %v err %v", found, err)
	}
	first := botaccounts.Account{Platform: "conntest-platform", ExternalID: "conntest-bot-1", Login: "one", AccessTokenKey: "A", RefreshTokenKey: "R", Scopes: []string{"user:bot"}}
	second := first
	second.ExternalID, second.Login = "conntest-bot-2", "two"
	for _, a := range []botaccounts.Account{first, second} {
		if err := b.SaveDefault(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	got, found, err := b.Default(ctx, "conntest-platform")
	if err != nil || !found || got.ExternalID != "conntest-bot-2" || got.RefreshTokenKey != "R" || len(got.Scopes) != 1 {
		t.Fatalf("default: %+v found %v err %v", got, found, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM lys.bot_accounts WHERE platform = 'conntest-platform'`); err != nil {
		t.Fatal(err)
	}
}
