-- The lys schema as it was in prod before migrations existed (taken from the
-- dev copy, which matches it). Every object is created only if it's missing,
-- so on prod this changes nothing, and on an empty database it builds the
-- whole schema. The grants are safe to repeat.
--
-- Needs, from the Admin setup (scripts/db/setup.sql): the lys roles, and the
-- citext extension in lys_db.

-- +goose Up

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citext') THEN
    RAISE EXCEPTION 'database: the citext extension is missing. As Admin, run CREATE EXTENSION citext; in lys_db (scripts/db/setup.sql does it).';
  END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA lys TO lys_app, lys_reader;

-- Tables created from here on are usable by the app and the reader.
ALTER DEFAULT PRIVILEGES FOR ROLE lys_owner IN SCHEMA lys GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO lys_app;
ALTER DEFAULT PRIVILEGES FOR ROLE lys_owner IN SCHEMA lys GRANT SELECT ON TABLES TO lys_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE lys_owner IN SCHEMA lys GRANT SELECT, USAGE ON SEQUENCES TO lys_app;

-- goose made its table before the default privileges above existed.
GRANT SELECT ON lys.goose_db_version TO lys_app, lys_reader;

-- +goose StatementBegin
DO $$
BEGIN
  IF to_regtype('lys.channel_action') IS NULL THEN
    CREATE TYPE lys.channel_action AS ENUM ('grant', 'revoke', 'ban', 'unban');
  END IF;

  IF to_regtype('lys.request_status') IS NULL THEN
    CREATE TYPE lys.request_status AS ENUM ('new', 'in_progress', 'completed', 'rejected');
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $do$
BEGIN
  -- The function bodies are exactly prod's: Postgres stores them as written.
  IF to_regprocedure('lys.create_twitch_settings_entry()') IS NULL THEN
    CREATE FUNCTION lys.create_twitch_settings_entry() RETURNS trigger
        LANGUAGE plpgsql
        AS $fn$
BEGIN
    INSERT INTO lys.twitch_settings (user_id)
    VALUES (NEW.user_id)
    ON CONFLICT (user_id) DO NOTHING;
    RETURN NEW;
END;
$fn$;
  END IF;

  IF to_regprocedure('lys.log_twitch_channel_changes()') IS NULL THEN
    CREATE FUNCTION lys.log_twitch_channel_changes() RETURNS trigger
        LANGUAGE plpgsql
        AS $fn$
DECLARE
    v_action lys.channel_action;
BEGIN
    -- Case 1: New channel added -> always log 'grant'
    IF TG_OP = 'INSERT' THEN
        INSERT INTO lys.twitch_channel_events (user_id, username, action)
        VALUES (NEW.user_id, NEW.username, 'grant'::lys.channel_action);
        RETURN NEW;
    END IF;

    -- Case 2: Updates to 'active' or 'banned'
    IF TG_OP = 'UPDATE' THEN
        -- If 'active' changed, log grant/revoke
        IF NEW.active IS DISTINCT FROM OLD.active THEN
            v_action := CASE WHEN NEW.active
                             THEN 'grant'::lys.channel_action
                             ELSE 'revoke'::lys.channel_action
                        END;
            INSERT INTO lys.twitch_channel_events (user_id, username, action)
            VALUES (NEW.user_id, NEW.username, v_action);
        END IF;

        -- If 'banned' changed, log ban/unban
        IF NEW.banned IS DISTINCT FROM OLD.banned THEN
            v_action := CASE WHEN NEW.banned
                             THEN 'ban'::lys.channel_action
                             ELSE 'unban'::lys.channel_action
                        END;
            INSERT INTO lys.twitch_channel_events (user_id, username, action)
            VALUES (NEW.user_id, NEW.username, v_action);
        END IF;

        RETURN NEW;
    END IF;

    RETURN NEW;
END;
$fn$;
  END IF;
END $do$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('lys.twitch_channels') IS NULL THEN
    CREATE TABLE lys.twitch_channels (
        user_id text NOT NULL,
        username public.citext NOT NULL,
        access_token text NOT NULL,
        refresh_token text NOT NULL,
        date_joined timestamp with time zone DEFAULT now() NOT NULL,
        last_joined timestamp with time zone DEFAULT now() NOT NULL,
        active boolean DEFAULT true NOT NULL,
        times_joined integer DEFAULT 1 NOT NULL,
        banned boolean DEFAULT false NOT NULL,
        CONSTRAINT channels_pkey PRIMARY KEY (user_id),
        CONSTRAINT channels_username_key UNIQUE (username)
    );
    CREATE INDEX idx_channels_active ON lys.twitch_channels USING btree (active);
    CREATE TRIGGER twitch_channels_after_insert AFTER INSERT ON lys.twitch_channels
        FOR EACH ROW EXECUTE FUNCTION lys.create_twitch_settings_entry();
    CREATE TRIGGER twitch_channels_after_insert_log AFTER INSERT ON lys.twitch_channels
        FOR EACH ROW EXECUTE FUNCTION lys.log_twitch_channel_changes();
    CREATE TRIGGER twitch_channels_after_update_log AFTER UPDATE OF active, banned ON lys.twitch_channels
        FOR EACH ROW WHEN (((old.active IS DISTINCT FROM new.active) OR (old.banned IS DISTINCT FROM new.banned)))
        EXECUTE FUNCTION lys.log_twitch_channel_changes();
  END IF;

  IF to_regclass('lys.twitch_channel_events') IS NULL THEN
    CREATE TABLE lys.twitch_channel_events (
        event_id uuid DEFAULT gen_random_uuid() NOT NULL,
        user_id text NOT NULL,
        username public.citext NOT NULL,
        action lys.channel_action NOT NULL,
        occurred_at timestamp with time zone DEFAULT now() NOT NULL,
        CONSTRAINT channel_events_pkey PRIMARY KEY (event_id),
        CONSTRAINT twitch_channel_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES lys.twitch_channels(user_id) ON DELETE CASCADE
    );
    CREATE INDEX idx_channel_events_recent ON lys.twitch_channel_events USING btree (occurred_at DESC);
  END IF;

  IF to_regclass('lys.twitch_settings') IS NULL THEN
    CREATE TABLE lys.twitch_settings (
        user_id text NOT NULL,
        command_prefix text DEFAULT '!'::text NOT NULL,
        date_created timestamp without time zone DEFAULT now(),
        last_modified timestamp without time zone DEFAULT now(),
        CONSTRAINT twitch_settings_pkey PRIMARY KEY (user_id),
        CONSTRAINT twitch_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES lys.twitch_channels(user_id) ON DELETE CASCADE
    );
  END IF;

  IF to_regclass('lys.feature_requests') IS NULL THEN
    CREATE SEQUENCE lys.feature_requests_id_seq START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;
    CREATE TABLE lys.feature_requests (
        id bigint DEFAULT nextval('lys.feature_requests_id_seq'::regclass) NOT NULL,
        occurred_at timestamp with time zone DEFAULT now() NOT NULL,
        user_id text NOT NULL,
        username text NOT NULL,
        channel_id text NOT NULL,
        channel_name text NOT NULL,
        platform text NOT NULL,
        body text NOT NULL,
        status lys.request_status DEFAULT 'new'::lys.request_status NOT NULL,
        CONSTRAINT feature_requests_pkey PRIMARY KEY (id)
    );
    ALTER SEQUENCE lys.feature_requests_id_seq OWNED BY lys.feature_requests.id;
    CREATE INDEX feature_requests_occurred_at_desc_idx ON lys.feature_requests USING btree (occurred_at DESC);
  END IF;

  -- The sequence keeps the table's old name, feeds.
  IF to_regclass('lys.letterboxd_feed_subscriptions') IS NULL THEN
    CREATE SEQUENCE lys.feeds_id_seq START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;
    CREATE TABLE lys.letterboxd_feed_subscriptions (
        id bigint DEFAULT nextval('lys.feeds_id_seq'::regclass) NOT NULL,
        username text NOT NULL,
        user_id text NOT NULL,
        letterboxd_username character varying(255) NOT NULL,
        feed_url text NOT NULL,
        last_seen_guid text,
        last_alerted_at timestamp with time zone,
        created_at timestamp with time zone DEFAULT now(),
        alert_channels text[] DEFAULT '{}'::text[] NOT NULL,
        CONSTRAINT feeds_pkey PRIMARY KEY (id),
        CONSTRAINT feeds_feed_url_key UNIQUE (feed_url),
        CONSTRAINT feeds_user_id_letterboxd_username_key UNIQUE (user_id, letterboxd_username)
    );
    ALTER SEQUENCE lys.feeds_id_seq OWNED BY lys.letterboxd_feed_subscriptions.id;
  END IF;

  IF to_regclass('lys.game_events') IS NULL THEN
    CREATE SEQUENCE lys.game_events_id_seq START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;
    CREATE TABLE lys.game_events (
        id bigint DEFAULT nextval('lys.game_events_id_seq'::regclass) NOT NULL,
        occurred_at timestamp with time zone DEFAULT now() NOT NULL,
        user_id text NOT NULL,
        username public.citext NOT NULL,
        channel_id text NOT NULL,
        channel_name text NOT NULL,
        game_code text NOT NULL,
        raw_message text NOT NULL,
        details jsonb,
        is_flagged boolean DEFAULT false NOT NULL,
        CONSTRAINT game_events_pkey PRIMARY KEY (id)
    );
    ALTER SEQUENCE lys.game_events_id_seq OWNED BY lys.game_events.id;
    CREATE INDEX game_events_oneuppuzzle_channel_time_idx ON lys.game_events USING btree (channel_id, (((details ->> 'time_seconds'::text))::integer)) WHERE ((game_code = 'oneuppuzzle'::text) AND (is_flagged = false));
    CREATE INDEX game_events_oneuppuzzle_user_puzzle_idx ON lys.game_events USING btree (user_id, (((details ->> 'puzzle_id'::text))::integer)) WHERE (game_code = 'oneuppuzzle'::text);
    CREATE INDEX game_events_oneuppuzzle_username_idx ON lys.game_events USING btree (username) WHERE ((game_code = 'oneuppuzzle'::text) AND (is_flagged = false));
  END IF;

  IF to_regclass('lys.sleeper_leagues') IS NULL THEN
    CREATE TABLE lys.sleeper_leagues (
        id uuid DEFAULT gen_random_uuid() NOT NULL,
        twitch_channel_id text NOT NULL,
        sleeper_league_id text NOT NULL,
        league_name text,
        season text,
        enabled boolean DEFAULT true NOT NULL,
        current_week integer,
        last_polled_at timestamp with time zone,
        last_transaction_at timestamp with time zone,
        last_waiver_week integer,
        last_recap_week integer,
        notifications jsonb DEFAULT '{}'::jsonb NOT NULL,
        created_at timestamp with time zone DEFAULT now() NOT NULL,
        updated_at timestamp with time zone DEFAULT now() NOT NULL,
        CONSTRAINT sleeper_leagues_pkey PRIMARY KEY (id),
        CONSTRAINT sleeper_leagues_twitch_channel_id_sleeper_league_id_key UNIQUE (twitch_channel_id, sleeper_league_id)
    );
  END IF;

  IF to_regclass('lys.sleeper_connections') IS NULL THEN
    CREATE TABLE lys.sleeper_connections (
        id uuid DEFAULT gen_random_uuid() NOT NULL,
        league_id uuid NOT NULL,
        twitch_user_id text NOT NULL,
        sleeper_user_id text NOT NULL,
        sleeper_username text,
        sleeper_roster_id integer NOT NULL,
        created_at timestamp with time zone DEFAULT now() NOT NULL,
        updated_at timestamp with time zone DEFAULT now() NOT NULL,
        twitch_username text,
        CONSTRAINT sleeper_connections_pkey PRIMARY KEY (id),
        CONSTRAINT sleeper_connections_league_id_sleeper_roster_id_key UNIQUE (league_id, sleeper_roster_id),
        CONSTRAINT sleeper_connections_league_id_twitch_user_id_key UNIQUE (league_id, twitch_user_id),
        CONSTRAINT sleeper_connections_league_id_fkey FOREIGN KEY (league_id) REFERENCES lys.sleeper_leagues(id) ON DELETE CASCADE
    );
    CREATE INDEX sleeper_connections_league_twitch_username_idx ON lys.sleeper_connections USING btree (league_id, lower(twitch_username));
  END IF;

  IF to_regclass('lys.sleeper_events') IS NULL THEN
    CREATE TABLE lys.sleeper_events (
        id uuid DEFAULT gen_random_uuid() NOT NULL,
        sleeper_league_id text NOT NULL,
        event_type text NOT NULL,
        external_event_id text,
        event_data jsonb,
        occurred_at timestamp with time zone NOT NULL,
        published_at timestamp with time zone,
        created_at timestamp with time zone DEFAULT now() NOT NULL,
        CONSTRAINT sleeper_events_pkey PRIMARY KEY (id),
        CONSTRAINT sleeper_events_sleeper_league_id_event_type_external_event__key UNIQUE (sleeper_league_id, event_type, external_event_id)
    );
  END IF;
END $$;
-- +goose StatementEnd
