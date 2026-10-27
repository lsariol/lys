-- The platform-neutral model: Lys accounts, the people and channels Lys
-- sees on each platform, and the bot accounts it speaks as.
--
--   accounts             a person, across platforms (empty until the website links them)
--   platform_users       one person on one platform, e.g. a Twitch user ID
--   communities          a streamer's community, which can span platforms
--   platform_connections one community on one platform, e.g. a Twitch channel
--   twitch_connection_details, connection_settings: per-connection extras
--   bot_accounts         who Lys speaks as on each platform; token values stay in Cove
--
-- Existing Twitch channels and every user the feature tables mention are
-- copied in. The old tables stay and are still written for rollback; nothing
-- here changes or removes them.

-- +goose Up

-- Mechanical bookkeeping only: domain changes happen in Go transactions.
-- +goose StatementBegin
CREATE FUNCTION lys.set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TABLE lys.accounts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    display_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE lys.platform_users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    platform text NOT NULL,
    external_id text NOT NULL,
    display_name text,
    account_id bigint REFERENCES lys.accounts(id) ON DELETE SET NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT platform_users_platform_external_id_key UNIQUE (platform, external_id)
);
CREATE INDEX platform_users_account_id_idx ON lys.platform_users (account_id) WHERE account_id IS NOT NULL;

-- Linking is verified by OAuth on the website, never guessed from names.
CREATE TABLE lys.account_link_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint REFERENCES lys.accounts(id) ON DELETE SET NULL,
    platform_user_id bigint REFERENCES lys.platform_users(id) ON DELETE SET NULL,
    event text NOT NULL CHECK (event IN ('linked', 'unlinked')),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE lys.communities (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    owner_account_id bigint REFERENCES lys.accounts(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- authorized: the platform still honours the channel's grant to the bot.
-- enabled: the channel owner wants Lys there. admin_blocked: Lys refuses to be there.
CREATE TABLE lys.platform_connections (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    community_id bigint NOT NULL REFERENCES lys.communities(id) ON DELETE CASCADE,
    platform text NOT NULL,
    external_id text NOT NULL,
    name text NOT NULL,
    authorized boolean NOT NULL DEFAULT true,
    enabled boolean NOT NULL DEFAULT true,
    admin_blocked boolean NOT NULL DEFAULT false,
    admin_block_reason text,
    connected_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT platform_connections_platform_external_id_key UNIQUE (platform, external_id),
    CONSTRAINT platform_connections_block_reason_chk CHECK (admin_blocked OR admin_block_reason IS NULL)
);
CREATE INDEX platform_connections_active_idx ON lys.platform_connections (platform)
    WHERE authorized AND enabled AND NOT admin_blocked;

CREATE TABLE lys.twitch_connection_details (
    connection_id bigint PRIMARY KEY REFERENCES lys.platform_connections(id) ON DELETE CASCADE,
    broadcaster_login public.citext NOT NULL,
    bot_is_moderator boolean NOT NULL DEFAULT false,
    times_joined integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE lys.connection_settings (
    connection_id bigint PRIMARY KEY REFERENCES lys.platform_connections(id) ON DELETE CASCADE,
    command_prefix text NOT NULL DEFAULT '!' CHECK (char_length(command_prefix) BETWEEN 1 AND 10),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TYPE lys.connection_event_type AS ENUM (
    'enrolled', 'reauthorized', 'authorization_revoked',
    'enabled', 'disabled', 'admin_blocked', 'admin_unblocked'
);

CREATE TABLE lys.connection_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    connection_id bigint REFERENCES lys.platform_connections(id) ON DELETE SET NULL,
    event_type lys.connection_event_type NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX connection_events_connection_idx ON lys.connection_events (connection_id, occurred_at DESC);

-- Token values live in Cove; this stores the Cove key names.
CREATE TABLE lys.bot_accounts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    platform text NOT NULL,
    external_id text NOT NULL,
    login public.citext NOT NULL,
    access_token_key text NOT NULL,
    refresh_token_key text,
    scopes text[] NOT NULL DEFAULT '{}'::text[],
    is_default boolean NOT NULL DEFAULT false,
    last_authorized_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bot_accounts_platform_external_id_key UNIQUE (platform, external_id)
);
CREATE UNIQUE INDEX bot_accounts_one_default_idx ON lys.bot_accounts (platform) WHERE is_default;

CREATE TRIGGER accounts_set_updated_at BEFORE UPDATE ON lys.accounts FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
CREATE TRIGGER platform_users_set_updated_at BEFORE UPDATE ON lys.platform_users FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
CREATE TRIGGER communities_set_updated_at BEFORE UPDATE ON lys.communities FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
CREATE TRIGGER platform_connections_set_updated_at BEFORE UPDATE ON lys.platform_connections FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
CREATE TRIGGER twitch_connection_details_set_updated_at BEFORE UPDATE ON lys.twitch_connection_details FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
CREATE TRIGGER connection_settings_set_updated_at BEFORE UPDATE ON lys.connection_settings FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
CREATE TRIGGER bot_accounts_set_updated_at BEFORE UPDATE ON lys.bot_accounts FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();

-- Each Twitch channel becomes a community with one connection. active=false
-- meant the grant was revoked; banned meant Lys blocked the channel.
-- Broadcaster tokens are not copied: nothing uses them.
-- +goose StatementBegin
DO $$
DECLARE
    ch record;
    community bigint;
    conn bigint;
BEGIN
    FOR ch IN
        SELECT tc.user_id, tc.username::text AS name, tc.date_joined, tc.last_joined,
               tc.active, tc.banned, tc.times_joined, COALESCE(ts.command_prefix, '!') AS prefix
        FROM lys.twitch_channels tc
        LEFT JOIN lys.twitch_settings ts ON ts.user_id = tc.user_id
        ORDER BY tc.date_joined, tc.user_id
    LOOP
        INSERT INTO lys.communities (name, created_at) VALUES (ch.name, ch.date_joined)
            RETURNING id INTO community;
        INSERT INTO lys.platform_connections
            (community_id, platform, external_id, name, authorized, admin_blocked, admin_block_reason, connected_at, created_at)
            VALUES (community, 'twitch', ch.user_id, ch.name, ch.active, ch.banned,
                    CASE WHEN ch.banned THEN 'banned before the move to platform_connections' END,
                    ch.last_joined, ch.date_joined)
            RETURNING id INTO conn;
        INSERT INTO lys.twitch_connection_details (connection_id, broadcaster_login, times_joined)
            VALUES (conn, ch.name, ch.times_joined);
        INSERT INTO lys.connection_settings (connection_id, command_prefix) VALUES (conn, ch.prefix);
        INSERT INTO lys.connection_events (connection_id, event_type, metadata, occurred_at)
            VALUES (conn, 'enrolled', '{"source": "twitch_channels"}', ch.date_joined);
    END LOOP;
END $$;
-- +goose StatementEnd

-- Everyone the existing data mentions, with their latest name.
INSERT INTO lys.platform_users (platform, external_id, display_name, first_seen_at, last_seen_at)
SELECT platform, user_id, (array_agg(name ORDER BY seen DESC) FILTER (WHERE name <> ''))[1], min(seen), max(seen)
FROM (
    SELECT 'twitch' AS platform, user_id, username::text AS name, occurred_at AS seen FROM lys.game_events
    UNION ALL
    SELECT platform, user_id, username, occurred_at FROM lys.feature_requests
    UNION ALL
    SELECT 'twitch', twitch_user_id, COALESCE(twitch_username, ''), updated_at FROM lys.sleeper_connections
    UNION ALL
    SELECT 'twitch', user_id, username, COALESCE(created_at, now()) FROM lys.letterboxd_feed_subscriptions
    UNION ALL
    SELECT 'twitch', user_id, username::text, date_joined FROM lys.twitch_channels
) seen
WHERE user_id <> ''
GROUP BY platform, user_id;
