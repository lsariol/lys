-- Feature data onto the platform-neutral keys: each row says which connection
-- (channel) and which platform user it belongs to, instead of raw Twitch IDs
-- and names. The old columns stay and are still written; nothing reads them.
--
-- Filling the new columns isn't done here: database.LinkLegacyRows does it
-- on every startup, so rows a rolled-back release writes are linked too.

-- +goose Up

ALTER TABLE lys.game_events
    ADD COLUMN connection_id bigint REFERENCES lys.platform_connections(id) ON DELETE SET NULL,
    ADD COLUMN platform_user_id bigint REFERENCES lys.platform_users(id) ON DELETE SET NULL;
CREATE INDEX game_events_oneuppuzzle_connection_time_idx ON lys.game_events
    (connection_id, (((details ->> 'time_seconds'::text))::integer))
    WHERE game_code = 'oneuppuzzle' AND is_flagged = false;
CREATE INDEX game_events_oneuppuzzle_user_puzzle_v2_idx ON lys.game_events
    (platform_user_id, (((details ->> 'puzzle_id'::text))::integer))
    WHERE game_code = 'oneuppuzzle';

ALTER TABLE lys.feature_requests
    ADD COLUMN connection_id bigint REFERENCES lys.platform_connections(id) ON DELETE SET NULL,
    ADD COLUMN platform_user_id bigint REFERENCES lys.platform_users(id) ON DELETE SET NULL;

ALTER TABLE lys.sleeper_leagues
    ADD COLUMN community_id bigint REFERENCES lys.communities(id) ON DELETE CASCADE;
CREATE INDEX sleeper_leagues_community_idx ON lys.sleeper_leagues (community_id);

ALTER TABLE lys.sleeper_connections
    ADD COLUMN platform_user_id bigint REFERENCES lys.platform_users(id) ON DELETE CASCADE;
CREATE INDEX sleeper_connections_league_user_idx ON lys.sleeper_connections (league_id, platform_user_id);

-- One row per feed and channel it posts to. LetterboxdWatcher's state
-- (last_seen_guid, last_alerted_at) moves here when Lys takes over polling.
CREATE TABLE lys.feed_subscriptions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL,
    feed_url text NOT NULL,
    feed_name text NOT NULL,
    connection_id bigint NOT NULL REFERENCES lys.platform_connections(id) ON DELETE CASCADE,
    platform_user_id bigint REFERENCES lys.platform_users(id) ON DELETE SET NULL,
    last_seen_guid text,
    last_alerted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT feed_subscriptions_kind_feed_connection_key UNIQUE (kind, feed_url, connection_id)
);
CREATE INDEX feed_subscriptions_user_idx ON lys.feed_subscriptions (platform_user_id, kind);
CREATE TRIGGER feed_subscriptions_set_updated_at BEFORE UPDATE ON lys.feed_subscriptions
    FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
