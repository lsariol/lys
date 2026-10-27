package database

import (
	"context"
	"testing"

	"github.com/lsariol/lys/internal/config"
)

// TestLinkLegacyRows writes rows the way the old release does (no new IDs),
// then checks LinkLegacyRows links them, and that a second run changes nothing.
func TestLinkLegacyRows(t *testing.T) {
	if migratorURL == "" || appURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL and LYS_TEST_DATABASE_URL aren't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()
	if err := Migrate(ctx, migratorURL); err != nil {
		t.Fatal(err)
	}
	db := NewDatabase(&config.DatabaseConfig{ConnectionString: appURL})
	if err := db.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	q := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{
		`DELETE FROM lys.letterboxd_feed_subscriptions WHERE user_id = 'lg-u1'`,
		`DELETE FROM lys.sleeper_leagues WHERE twitch_channel_id = 'lg-ch'`,
		`DELETE FROM lys.game_events WHERE channel_id IN ('lg-ch', 'lg-gone')`,
		`DELETE FROM lys.feature_requests WHERE channel_id = 'lg-ch'`,
		`DELETE FROM lys.communities WHERE id IN (SELECT community_id FROM lys.platform_connections WHERE external_id IN ('lg-ch', 'lg-gone'))`,
		`DELETE FROM lys.platform_users WHERE external_id LIKE 'lg-%'`,
	} {
		q(s)
	}

	var community, connection int64
	if err := db.Pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('Legacy') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `INSERT INTO lys.platform_connections (community_id, platform, external_id, name)
		VALUES ($1, 'twitch', 'lg-ch', 'Legacy') RETURNING id`, community).Scan(&connection); err != nil {
		t.Fatal(err)
	}

	q(`INSERT INTO lys.game_events (user_id, username, channel_id, channel_name, game_code, raw_message) VALUES ('lg-u1', 'Gamer', 'lg-ch', 'Legacy', 'oneuppuzzle', 'x')`)
	q(`INSERT INTO lys.game_events (user_id, username, channel_id, channel_name, game_code, raw_message) VALUES ('lg-u1', 'Gamer', 'lg-gone', 'Gone', 'oneuppuzzle', 'x')`)
	q(`INSERT INTO lys.feature_requests (user_id, username, channel_id, channel_name, platform, body) VALUES ('lg-u2', 'Asker', 'lg-ch', 'Legacy', 'twitch', 'idea')`)
	q(`INSERT INTO lys.sleeper_leagues (twitch_channel_id, sleeper_league_id) VALUES ('lg-ch', 'lg-league')`)
	q(`INSERT INTO lys.sleeper_connections (league_id, twitch_user_id, twitch_username, sleeper_user_id, sleeper_roster_id)
		SELECT id, 'lg-u3', 'Manager', 's1', 1 FROM lys.sleeper_leagues WHERE sleeper_league_id = 'lg-league'`)
	q(`INSERT INTO lys.letterboxd_feed_subscriptions (username, user_id, letterboxd_username, feed_url, last_seen_guid, alert_channels)
		VALUES ('Gamer', 'lg-u1', 'film_fan', 'https://letterboxd.com/lg-film_fan/rss/', 'guid-1', ARRAY['twitch:lg-ch'])`)

	if err := db.LinkLegacyRows(ctx); err != nil {
		t.Fatal(err)
	}

	var linked bool
	check := func(what, query string) {
		t.Helper()
		if err := db.Pool.QueryRow(ctx, query).Scan(&linked); err != nil || !linked {
			t.Errorf("%s: linked %v, err %v", what, linked, err)
		}
	}
	check("game event", `SELECT connection_id IS NOT NULL AND platform_user_id = (SELECT id FROM lys.platform_users WHERE external_id = 'lg-u1') FROM lys.game_events WHERE channel_id = 'lg-ch'`)
	check("feature request", `SELECT connection_id IS NOT NULL AND platform_user_id IS NOT NULL FROM lys.feature_requests WHERE channel_id = 'lg-ch'`)
	check("Sleeper league", `SELECT community_id IS NOT NULL FROM lys.sleeper_leagues WHERE sleeper_league_id = 'lg-league'`)
	check("Sleeper member", `SELECT platform_user_id = (SELECT id FROM lys.platform_users WHERE external_id = 'lg-u3') FROM lys.sleeper_connections WHERE twitch_user_id = 'lg-u3'`)
	check("Letterboxd feed", `SELECT feed_name = 'film_fan' AND last_seen_guid = 'guid-1' AND platform_user_id IS NOT NULL FROM lys.feed_subscriptions WHERE feed_url = 'https://letterboxd.com/lg-film_fan/rss/'`)

	// A channel only the data knows gets a connection Lys doesn't join.
	check("unknown channel", `SELECT NOT pc.authorized AND g.connection_id = pc.id
		FROM lys.game_events g JOIN lys.platform_connections pc ON pc.external_id = 'lg-gone'
		WHERE g.channel_id = 'lg-gone'`)

	// Newer polling state in the old table wins.
	q(`UPDATE lys.letterboxd_feed_subscriptions SET last_seen_guid = 'guid-2', last_alerted_at = now() WHERE user_id = 'lg-u1'`)
	if err := db.LinkLegacyRows(ctx); err != nil {
		t.Fatal(err)
	}
	check("newer state", `SELECT last_seen_guid = 'guid-2' FROM lys.feed_subscriptions WHERE feed_url = 'https://letterboxd.com/lg-film_fan/rss/'`)

	// An unwatch in the old release removes the feed.
	q(`DELETE FROM lys.letterboxd_feed_subscriptions WHERE user_id = 'lg-u1'`)
	if err := db.LinkLegacyRows(ctx); err != nil {
		t.Fatal(err)
	}
	check("removed feed", `SELECT NOT EXISTS (SELECT 1 FROM lys.feed_subscriptions WHERE feed_url = 'https://letterboxd.com/lg-film_fan/rss/')`)
}
