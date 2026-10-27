package database

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

// legacyLinks fill the new ID columns from the old Twitch IDs. Each only
// touches rows that aren't linked yet, so running them again is cheap.
var legacyLinks = []struct {
	what  string
	query string
}{
	{"platform users", `
		INSERT INTO lys.platform_users (platform, external_id, display_name, first_seen_at, last_seen_at)
		SELECT platform, user_id, (array_agg(name ORDER BY seen DESC) FILTER (WHERE name <> ''))[1], min(seen), max(seen)
		FROM (
			SELECT 'twitch' AS platform, user_id, username::text AS name, occurred_at AS seen FROM lys.game_events WHERE platform_user_id IS NULL
			UNION ALL
			SELECT platform, user_id, username, occurred_at FROM lys.feature_requests WHERE platform_user_id IS NULL
			UNION ALL
			SELECT 'twitch', twitch_user_id, COALESCE(twitch_username, ''), updated_at FROM lys.sleeper_connections WHERE platform_user_id IS NULL
			UNION ALL
			SELECT 'twitch', user_id, username, COALESCE(created_at, now()) FROM lys.letterboxd_feed_subscriptions
		) seen
		WHERE user_id <> ''
		GROUP BY platform, user_id
		ON CONFLICT (platform, external_id) DO NOTHING`},
	// Channels the data mentions that have no connection (e.g. they left before
	// connections existed) get one, not authorized, so Lys doesn't join them.
	{"channels", `
		DO $$
		DECLARE
			ch record;
			community bigint;
			conn bigint;
		BEGIN
			FOR ch IN
				SELECT channel_id, (array_agg(name ORDER BY seen DESC))[1] AS name
				FROM (
					SELECT channel_id, channel_name AS name, occurred_at AS seen FROM lys.game_events WHERE connection_id IS NULL
					UNION ALL
					SELECT channel_id, channel_name, occurred_at FROM lys.feature_requests WHERE connection_id IS NULL AND platform = 'twitch'
					UNION ALL
					SELECT twitch_channel_id, twitch_channel_id, created_at FROM lys.sleeper_leagues WHERE community_id IS NULL
					UNION ALL
					SELECT split_part(a, ':', 2), split_part(a, ':', 2), COALESCE(l.created_at, now())
					FROM lys.letterboxd_feed_subscriptions l CROSS JOIN LATERAL unnest(l.alert_channels) AS a
					WHERE split_part(a, ':', 1) = 'twitch'
				) seen
				WHERE channel_id <> ''
				  AND NOT EXISTS (SELECT 1 FROM lys.platform_connections pc WHERE pc.platform = 'twitch' AND pc.external_id = seen.channel_id)
				GROUP BY channel_id
			LOOP
				INSERT INTO lys.communities (name) VALUES (ch.name) RETURNING id INTO community;
				INSERT INTO lys.platform_connections (community_id, platform, external_id, name, authorized)
					VALUES (community, 'twitch', ch.channel_id, ch.name, false) RETURNING id INTO conn;
				INSERT INTO lys.twitch_connection_details (connection_id, broadcaster_login) VALUES (conn, lower(ch.name));
				INSERT INTO lys.connection_settings (connection_id) VALUES (conn);
				INSERT INTO lys.connection_events (connection_id, event_type, metadata)
					VALUES (conn, 'enrolled', '{"source": "feature data", "authorized": false}');
				RAISE LOG 'lys: added channel % (%) from feature data, not authorized', ch.name, ch.channel_id;
			END LOOP;
		END $$`},
	{"game events (channel)", `
		UPDATE lys.game_events g SET connection_id = pc.id
		FROM lys.platform_connections pc
		WHERE g.connection_id IS NULL AND pc.platform = 'twitch' AND pc.external_id = g.channel_id`},
	{"game events (user)", `
		UPDATE lys.game_events g SET platform_user_id = pu.id
		FROM lys.platform_users pu
		WHERE g.platform_user_id IS NULL AND pu.platform = 'twitch' AND pu.external_id = g.user_id`},
	{"feature requests (channel)", `
		UPDATE lys.feature_requests f SET connection_id = pc.id
		FROM lys.platform_connections pc
		WHERE f.connection_id IS NULL AND pc.platform = f.platform AND pc.external_id = f.channel_id`},
	{"feature requests (user)", `
		UPDATE lys.feature_requests f SET platform_user_id = pu.id
		FROM lys.platform_users pu
		WHERE f.platform_user_id IS NULL AND pu.platform = f.platform AND pu.external_id = f.user_id`},
	{"Sleeper leagues", `
		UPDATE lys.sleeper_leagues l SET community_id = pc.community_id
		FROM lys.platform_connections pc
		WHERE l.community_id IS NULL AND pc.platform = 'twitch' AND pc.external_id = l.twitch_channel_id`},
	{"Sleeper members", `
		UPDATE lys.sleeper_connections c SET platform_user_id = pu.id
		FROM lys.platform_users pu
		WHERE c.platform_user_id IS NULL AND pu.platform = 'twitch' AND pu.external_id = c.twitch_user_id`},
	// letterboxd_feed_subscriptions is kept current alongside feed_subscriptions,
	// so it says which Letterboxd feeds exist. Its polling state wins when it's newer.
	{"Letterboxd feeds", `
		INSERT INTO lys.feed_subscriptions (kind, feed_url, feed_name, connection_id, platform_user_id, last_seen_guid, last_alerted_at, created_at)
		SELECT 'letterboxd', l.feed_url, l.letterboxd_username, pc.id, pu.id, l.last_seen_guid, l.last_alerted_at, COALESCE(l.created_at, now())
		FROM lys.letterboxd_feed_subscriptions l
		CROSS JOIN LATERAL unnest(l.alert_channels) AS ch(value)
		JOIN lys.platform_connections pc
			ON pc.platform = split_part(ch.value, ':', 1) AND pc.external_id = split_part(ch.value, ':', 2)
		LEFT JOIN lys.platform_users pu ON pu.platform = 'twitch' AND pu.external_id = l.user_id
		ON CONFLICT (kind, feed_url, connection_id) DO UPDATE
			SET last_seen_guid = EXCLUDED.last_seen_guid, last_alerted_at = EXCLUDED.last_alerted_at
			WHERE EXCLUDED.last_alerted_at IS NOT NULL
			  AND (lys.feed_subscriptions.last_alerted_at IS NULL OR EXCLUDED.last_alerted_at > lys.feed_subscriptions.last_alerted_at)`},
	{"Letterboxd feeds removed", `
		DELETE FROM lys.feed_subscriptions f
		WHERE f.kind = 'letterboxd'
		  AND NOT EXISTS (SELECT 1 FROM lys.letterboxd_feed_subscriptions l WHERE l.feed_url = f.feed_url)`},
}

// LinkLegacyRows fills the new ID columns for rows that don't have them yet:
// all of them on the first start, and afterwards only rows an older release
// wrote (after a rollback). It runs in one transaction.
func (d *Database) LinkLegacyRows(ctx context.Context) error {
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		for _, l := range legacyLinks {
			tag, err := tx.Exec(ctx, l.query)
			if err != nil {
				return fmt.Errorf("database: link %s to the new keys: %w", l.what, err)
			}
			if n := tag.RowsAffected(); n > 0 {
				log.Printf("[Database] linked %d %s to the new keys", n, l.what)
			}
		}
		return nil
	})
}
