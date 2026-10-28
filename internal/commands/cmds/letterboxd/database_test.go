package letterboxd

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/database"
)

// TestPGStore needs a Postgres set up by scripts/test-db.sh.
func TestPGStore(t *testing.T) {
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
		`DELETE FROM lys.letterboxd_feed_subscriptions WHERE user_id = 'lbte-u1'`,
		`DELETE FROM lys.communities WHERE name = 'LetterboxdTest'`,
		`DELETE FROM lys.platform_users WHERE external_id = 'lbte-u1'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var community, conn, user int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('LetterboxdTest') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_connections (community_id, platform, external_id, name) VALUES ($1, 'twitch', 'lbte-ch', 'LetterboxdTest') RETURNING id`, community).Scan(&conn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_users (platform, external_id, display_name) VALUES ('twitch', 'lbte-u1', 'Watcher') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}

	s := NewStore(pool)
	sub := Subscription{PlatformUserID: user, ConnectionID: conn, FeedURL: "https://letterboxd.com/lbte_fan/rss/", LetterboxdName: "lbte_fan",
		Username: "Watcher", UserID: "lbte-u1", AlertChannel: "twitch:lbte-ch"}
	if err := s.AddSubscription(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if watching, err := s.AlreadyWatching(ctx, user); err != nil || !watching {
		t.Fatalf("watching: %v, %v", watching, err)
	}

	// The old table gets the same subscription, which v0.0.0's startup sync reads (until v0.1.1).
	var oldChannels []string
	if err := pool.QueryRow(ctx, `SELECT alert_channels FROM lys.letterboxd_feed_subscriptions WHERE user_id = 'lbte-u1'`).Scan(&oldChannels); err != nil || len(oldChannels) != 1 || oldChannels[0] != "twitch:lbte-ch" {
		t.Fatalf("old row: %v, %v", oldChannels, err)
	}

	name, found, err := s.RemoveSubscription(ctx, user, "lbte-u1")
	if err != nil || !found || name != "lbte_fan" {
		t.Fatalf("remove: %q %v %v", name, found, err)
	}
	if watching, err := s.AlreadyWatching(ctx, user); err != nil || watching {
		t.Fatalf("still watching: %v, %v", watching, err)
	}
	if _, found, err := s.RemoveSubscription(ctx, user, "lbte-u1"); err != nil || found {
		t.Fatalf("second remove: %v, %v", found, err)
	}
}
