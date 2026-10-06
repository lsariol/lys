-- ============================================================================
-- Lys Twitch Schema - Rewrite Reference
-- ============================================================================
--
-- Purpose
-- -------
-- This file defines the Twitch-specific database model recommended for the
-- Lys rewrite.
--
-- It is intentionally written as a CANONICAL TARGET SCHEMA, not as an ALTER
-- migration from the current production schema. The goal is to document the
-- model Lys should end up with after the rewrite.
--
-- Major design decisions
-- ----------------------
-- 1. Twitch channel identity / authorization is separate from per-channel
--    configuration.
--
-- 2. The LysLink bot identity is separate from the channels it operates in.
--
-- 3. Twitch access tokens and refresh tokens are SECRETS and should remain in
--    Cove. PostgreSQL stores only the Cove secret keys used to retrieve them.
--
-- 4. A channel is not deleted merely because it disconnects Lys. Authorization
--    state is tracked explicitly so settings and history can survive a
--    disconnect/reconnect cycle.
--
-- 5. "Authorized", "enabled", and "administratively blocked" are intentionally
--    different states:
--
--      channel_bot_authorized
--          Twitch currently recognizes the broadcaster's channel:bot grant.
--
--      bot_enabled
--          The channel owner wants Lys operating in the channel.
--
--      admin_blocked
--          Lys itself refuses to operate in the channel.
--
-- 6. Twitch user IDs are the durable identifiers. Twitch logins/usernames are
--    mutable cached attributes and are never used as primary keys.
--
-- 7. Per-channel settings remain in their own 1:1 table. This prevents
--    twitch_channels from becoming a mixture of identity, authorization,
--    lifecycle state, and an ever-growing set of configuration fields.
--
-- 8. Database triggers are used only for mechanical bookkeeping such as
--    updated_at. Application/domain workflows (authorize a channel, create its
--    settings, append an event) should be explicit transactions in Go.
--
-- 9. The app access token is intentionally NOT persisted here. It is runtime
--    cacheable state and can be reissued using the Twitch client credentials.
--    The Twitch client secret itself should remain in Cove/application config.
--
-- Notes
-- -----
-- * This file assumes the "lys" schema is the application's schema.
-- * citext is used for Twitch login names because Twitch logins are
--   case-insensitive for application purposes.
-- * Adapt enum values and optional metadata as Lys gains additional features.
--
-- ============================================================================

BEGIN;


-- ============================================================================
-- Extensions / schema
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS lys;

CREATE EXTENSION IF NOT EXISTS citext;


-- ============================================================================
-- Shared updated_at helper
-- ============================================================================
--
-- updated_at is mechanical bookkeeping and is a good fit for a database
-- trigger. Unlike domain behavior, there is no ambiguity about what should
-- happen whenever a row changes.
--

CREATE OR REPLACE FUNCTION lys.set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;


-- ============================================================================
-- Channel event type
-- ============================================================================
--
-- The event name is typed so callers cannot invent inconsistent spellings.
-- Event-specific context belongs in the JSONB metadata column on the event.
--
-- Add new values with migrations as Lys gains new channel lifecycle events.
--

CREATE TYPE lys.twitch_channel_event_type AS ENUM (
    'authorized',
    'reauthorized',
    'authorization_revoked',

    'enabled',
    'disabled',

    'admin_blocked',
    'admin_unblocked',

    'moderator_granted',
    'moderator_revoked'
);


-- ============================================================================
-- Twitch bot accounts
-- ============================================================================
--
-- Represents Twitch identities that Lys itself can operate AS.
--
-- Today this will normally contain one row:
--
--     LysLink_dev
--
-- This is intentionally modeled as a table instead of a generic
-- "admin_settings" table because these fields describe a concrete domain
-- entity: a Twitch bot identity.
--
-- IMPORTANT:
--     The actual access token and refresh token do NOT belong in PostgreSQL.
--     They should be stored in Cove. The *_secret_key columns below contain
--     only the Cove keys used to retrieve those secrets.
--
-- Keeping this as a table also means Lys can support more than one bot
-- identity later without redesigning a singleton settings blob.
--

CREATE TABLE lys.twitch_bot_accounts (
    bot_user_id               text PRIMARY KEY,

    -- Cached Twitch identity fields. bot_user_id is authoritative.
    login                     citext NOT NULL,
    display_name              text,

    -- Cove secret keys, NOT the token values themselves.
    access_token_secret_key   text NOT NULL,
    refresh_token_secret_key  text NOT NULL,

    -- The scopes granted to this bot identity's USER access token.
    -- This is operational metadata; Twitch remains the source of truth.
    oauth_scopes              text[] NOT NULL DEFAULT '{}'::text[],

    -- Access-token metadata. expires_at is useful for diagnostics/planning,
    -- but runtime code should still handle Twitch 401 responses correctly.
    access_token_expires_at   timestamptz,
    last_refreshed_at         timestamptz,
    last_validated_at         timestamptz,

    -- If Twitch authorization is revoked and refresh can no longer recover,
    -- this records when Lys detected that condition.
    authorization_revoked_at  timestamptz,

    enabled                   boolean NOT NULL DEFAULT true,

    -- Lets the system identify the normal bot account without hardcoding a
    -- Twitch user ID throughout the application.
    is_default                boolean NOT NULL DEFAULT false,

    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE lys.twitch_bot_accounts IS
'Bot identities that Lys operates as on Twitch. Token values live in Cove; this table stores identity and token metadata only.';

COMMENT ON COLUMN lys.twitch_bot_accounts.bot_user_id IS
'Stable Twitch user ID for the bot account, e.g. LysLink_dev.';

COMMENT ON COLUMN lys.twitch_bot_accounts.access_token_secret_key IS
'Cove secret key containing this bot account''s Twitch user access token.';

COMMENT ON COLUMN lys.twitch_bot_accounts.refresh_token_secret_key IS
'Cove secret key containing this bot account''s Twitch refresh token.';

COMMENT ON COLUMN lys.twitch_bot_accounts.oauth_scopes IS
'Cached scopes currently expected on the bot user token. Twitch validation remains authoritative.';

COMMENT ON COLUMN lys.twitch_bot_accounts.is_default IS
'Marks the bot identity Lys should normally use when no explicit bot account is selected.';

-- There should never be two default bot identities at the same time.
CREATE UNIQUE INDEX twitch_bot_accounts_one_default_idx
    ON lys.twitch_bot_accounts (is_default)
    WHERE is_default = true;

CREATE INDEX twitch_bot_accounts_login_idx
    ON lys.twitch_bot_accounts (login);

CREATE TRIGGER twitch_bot_accounts_set_updated_at
BEFORE UPDATE ON lys.twitch_bot_accounts
FOR EACH ROW
EXECUTE FUNCTION lys.set_updated_at();


-- ============================================================================
-- Twitch channels
-- ============================================================================
--
-- Represents a Twitch broadcaster/channel that has established a relationship
-- with Lys.
--
-- A row should survive normal disconnect/reconnect cycles. Do NOT delete the
-- row simply because the broadcaster revokes Twitch authorization.
--
-- Why broadcaster_id instead of user_id?
-- --------------------------------------
-- "user_id" is ambiguous once the codebase contains:
--   * the broadcaster
--   * the bot user
--   * chat users
--   * moderators
--
-- "broadcaster_id" makes SQL and Go code self-documenting.
--
-- Why is login not UNIQUE?
-- ------------------------
-- Twitch user IDs are durable; logins can change and can eventually be reused.
-- A stale historical row should not prevent another Twitch user ID from being
-- represented simply because a mutable login was reassigned. Code should use
-- broadcaster_id as identity and treat login/display_name as cached metadata.
--

CREATE TABLE lys.twitch_channels (
    broadcaster_id            text PRIMARY KEY,

    -- Cached Twitch profile information.
    login                     citext NOT NULL,
    display_name              text,

    -- Twitch authorization state.
    --
    -- This specifically means the broadcaster has authorized the Twitch
    -- application for the channel-level bot relationship (for example,
    -- channel:bot in the Cloud Chatbot model).
    channel_bot_authorized    boolean NOT NULL DEFAULT true,

    -- First time this Twitch channel established authorization with Lys.
    authorized_at             timestamptz NOT NULL DEFAULT now(),

    -- Most recent successful authorization / reauthorization.
    last_authorized_at        timestamptz NOT NULL DEFAULT now(),

    -- Last time Lys independently verified the authorization with Twitch.
    authorization_checked_at  timestamptz,

    -- Set when authorization is known to have been revoked.
    -- Clear this when the channel successfully reauthorizes.
    authorization_revoked_at  timestamptz,

    -- Replaces the old "times_joined" concept with terminology tied to the
    -- actual domain event: authorization.
    authorization_count       integer NOT NULL DEFAULT 1
                               CHECK (authorization_count >= 1),

    -- Channel-owner preference. A channel can remain authorized while the
    -- owner temporarily disables Lys.
    bot_enabled               boolean NOT NULL DEFAULT true,

    -- Administrative control owned by Lys, not Twitch and not the streamer.
    -- This should be used for abuse/suspension decisions rather than overloading
    -- bot_enabled or authorization state.
    admin_blocked             boolean NOT NULL DEFAULT false,
    admin_blocked_at          timestamptz,
    admin_block_reason        text,

    -- Cached capability information.
    --
    -- Many moderation APIs require the bot account to actually be a moderator
    -- in the target channel. This is cached state and should be periodically
    -- verified when moderation features depend on it.
    bot_is_moderator          boolean NOT NULL DEFAULT false,
    moderator_checked_at      timestamptz,

    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),

    -- Keep block metadata internally consistent.
    CONSTRAINT twitch_channels_admin_block_metadata_chk CHECK (
        (admin_blocked = true)
        OR
        (admin_blocked_at IS NULL AND admin_block_reason IS NULL)
    ),

    -- If authorization is currently valid, there should not be a current
    -- revoked timestamp.
    CONSTRAINT twitch_channels_authorization_state_chk CHECK (
        (channel_bot_authorized = false)
        OR
        (authorization_revoked_at IS NULL)
    )
);

COMMENT ON TABLE lys.twitch_channels IS
'Twitch broadcaster channels that have established a relationship with Lys. Stores identity and lifecycle/authorization state, not per-channel preferences or user OAuth tokens.';

COMMENT ON COLUMN lys.twitch_channels.broadcaster_id IS
'Stable Twitch user ID for the broadcaster/channel and the canonical identifier used by Lys.';

COMMENT ON COLUMN lys.twitch_channels.login IS
'Cached Twitch login. Mutable and not an identity key.';

COMMENT ON COLUMN lys.twitch_channels.channel_bot_authorized IS
'Whether the broadcaster''s channel-level bot authorization is currently known to be valid.';

COMMENT ON COLUMN lys.twitch_channels.bot_enabled IS
'Whether the broadcaster currently wants Lys enabled in the channel. Independent of Twitch authorization.';

COMMENT ON COLUMN lys.twitch_channels.admin_blocked IS
'Whether Lys administrators/system policy have blocked operation in this channel. Independent of Twitch authorization and broadcaster preference.';

COMMENT ON COLUMN lys.twitch_channels.bot_is_moderator IS
'Cached indication that the Lys bot account is currently a moderator in this broadcaster''s channel.';

-- Login is useful for lookup/display but is intentionally not unique.
CREATE INDEX twitch_channels_login_idx
    ON lys.twitch_channels (login);

-- Common startup/runtime query:
-- "Which channels should Lys currently operate in?"
CREATE INDEX twitch_channels_operational_idx
    ON lys.twitch_channels (broadcaster_id)
    WHERE channel_bot_authorized = true
      AND bot_enabled = true
      AND admin_blocked = false;

-- Useful for authorization reconciliation jobs.
CREATE INDEX twitch_channels_authorization_check_idx
    ON lys.twitch_channels (authorization_checked_at)
    WHERE channel_bot_authorized = true;

-- Useful when moderation capability needs periodic reconciliation.
CREATE INDEX twitch_channels_moderator_check_idx
    ON lys.twitch_channels (moderator_checked_at)
    WHERE bot_enabled = true
      AND admin_blocked = false;

CREATE TRIGGER twitch_channels_set_updated_at
BEFORE UPDATE ON lys.twitch_channels
FOR EACH ROW
EXECUTE FUNCTION lys.set_updated_at();


-- ============================================================================
-- Twitch per-channel settings
-- ============================================================================
--
-- This table is intentionally separate from twitch_channels.
--
-- twitch_channels answers:
--     "Who is this channel, and what is its current relationship with Lys?"
--
-- twitch_channel_settings answers:
--     "How does this channel want Lys configured?"
--
-- Keeping the concerns separate prevents twitch_channels from growing a new
-- column every time Lys gains a user-configurable feature.
--
-- Use typed columns for important settings. Avoid a generic key/value settings
-- table and avoid dumping the entire settings model into JSONB. Typed columns
-- provide better validation, discoverability, migrations, and Go type safety.
--
-- If a future subsystem becomes large enough (for example moderation with many
-- settings), it can receive its own focused 1:1 settings table rather than
-- making this table arbitrarily large.
--

CREATE TABLE lys.twitch_channel_settings (
    broadcaster_id  text PRIMARY KEY
                    REFERENCES lys.twitch_channels(broadcaster_id)
                    ON DELETE CASCADE,

    command_prefix  text NOT NULL DEFAULT '!'
                    CHECK (char_length(command_prefix) BETWEEN 1 AND 10),

    -- Future examples belong here only when they are genuinely general
    -- channel-level preferences:
    --
    -- respond_to_mentions      boolean NOT NULL DEFAULT true,
    -- delete_command_messages  boolean NOT NULL DEFAULT false,
    -- allow_user_commands      boolean NOT NULL DEFAULT true,

    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE lys.twitch_channel_settings IS
'User-configurable Twitch settings for one Lys-enabled broadcaster channel. One-to-one with twitch_channels.';

COMMENT ON COLUMN lys.twitch_channel_settings.broadcaster_id IS
'Broadcaster whose Lys Twitch settings these are.';

COMMENT ON COLUMN lys.twitch_channel_settings.command_prefix IS
'Prefix used to invoke text commands in this Twitch channel.';

CREATE TRIGGER twitch_channel_settings_set_updated_at
BEFORE UPDATE ON lys.twitch_channel_settings
FOR EACH ROW
EXECUTE FUNCTION lys.set_updated_at();


-- ============================================================================
-- Twitch channel lifecycle / audit events
-- ============================================================================
--
-- This is an append-oriented event/audit table for significant changes in the
-- relationship between a Twitch channel and Lys.
--
-- event_type contains the stable, queryable event name.
-- metadata is reserved for event-specific context.
--
-- Example metadata:
--
--     event_type = 'authorized'
--     metadata   = {"scopes":["channel:bot"]}
--
--     event_type = 'admin_blocked'
--     metadata   = {"reason":"abuse"}
--
-- broadcaster_login is stored as a snapshot so an old event still reads
-- sensibly after the Twitch account later changes its login.
--
-- ON DELETE SET NULL is intentional:
-- normal disconnects should not delete twitch_channels at all, but if a row is
-- deliberately hard-deleted, audit events can survive without retaining a
-- relational dependency on a channel that no longer exists.
--
-- If Lys later implements a privacy purge, that workflow can explicitly delete
-- or anonymize the associated event rows as part of that dedicated operation.
--

CREATE TABLE lys.twitch_channel_events (
    event_id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    broadcaster_id      text
                        REFERENCES lys.twitch_channels(broadcaster_id)
                        ON DELETE SET NULL,

    broadcaster_login   citext,

    event_type          lys.twitch_channel_event_type NOT NULL,

    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,

    occurred_at         timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE lys.twitch_channel_events IS
'Append-oriented audit/history of significant Twitch channel lifecycle events.';

COMMENT ON COLUMN lys.twitch_channel_events.broadcaster_id IS
'Broadcaster related to the event. Nullable only so history can survive a deliberate hard delete.';

COMMENT ON COLUMN lys.twitch_channel_events.broadcaster_login IS
'Snapshot of the Twitch login at event time for historical readability.';

COMMENT ON COLUMN lys.twitch_channel_events.metadata IS
'Event-specific structured context. Stable event identity belongs in event_type, not in this JSON object.';

CREATE INDEX twitch_channel_events_broadcaster_time_idx
    ON lys.twitch_channel_events (broadcaster_id, occurred_at DESC);

CREATE INDEX twitch_channel_events_recent_idx
    ON lys.twitch_channel_events (occurred_at DESC);

CREATE INDEX twitch_channel_events_type_time_idx
    ON lys.twitch_channel_events (event_type, occurred_at DESC);


-- ============================================================================
-- Optional convenience view
-- ============================================================================
--
-- Centralizes the exact definition of a channel in which Lys should currently
-- operate. Application code may use the base table directly if preferred.
--
-- A channel can be absent from this view for three very different reasons:
--   * Twitch authorization is no longer valid
--   * the broadcaster disabled Lys
--   * Lys administratively blocked the channel
--

CREATE VIEW lys.operational_twitch_channels AS
SELECT
    broadcaster_id,
    login,
    display_name,
    bot_is_moderator,
    moderator_checked_at,
    authorization_checked_at
FROM lys.twitch_channels
WHERE channel_bot_authorized = true
  AND bot_enabled = true
  AND admin_blocked = false;

COMMENT ON VIEW lys.operational_twitch_channels IS
'Channels in which Lys is currently authorized, enabled by the broadcaster, and not administratively blocked.';


-- ============================================================================
-- Expected application workflows
-- ============================================================================
--
-- These comments are documentation only; they intentionally remain explicit
-- application transactions rather than hidden database triggers.
--
-- --------------------------------------------------------------------------
-- A. FIRST-TIME CHANNEL AUTHORIZATION
-- --------------------------------------------------------------------------
--
-- BEGIN;
--
-- INSERT INTO lys.twitch_channels (
--     broadcaster_id,
--     login,
--     display_name
-- )
-- VALUES ($1, $2, $3);
--
-- INSERT INTO lys.twitch_channel_settings (broadcaster_id)
-- VALUES ($1);
--
-- INSERT INTO lys.twitch_channel_events (
--     broadcaster_id,
--     broadcaster_login,
--     event_type,
--     metadata
-- )
-- VALUES (
--     $1,
--     $2,
--     'authorized',
--     jsonb_build_object('scopes', ARRAY['channel:bot'])
-- );
--
-- COMMIT;
--
--
-- --------------------------------------------------------------------------
-- B. CHANNEL REAUTHORIZATION
-- --------------------------------------------------------------------------
--
-- BEGIN;
--
-- UPDATE lys.twitch_channels
-- SET
--     login                     = $2,
--     display_name              = $3,
--     channel_bot_authorized    = true,
--     last_authorized_at        = now(),
--     authorization_checked_at  = now(),
--     authorization_revoked_at  = NULL,
--     authorization_count       = authorization_count + 1
-- WHERE broadcaster_id = $1;
--
-- INSERT INTO lys.twitch_channel_events (
--     broadcaster_id,
--     broadcaster_login,
--     event_type
-- )
-- VALUES ($1, $2, 'reauthorized');
--
-- COMMIT;
--
--
-- --------------------------------------------------------------------------
-- C. TWITCH AUTHORIZATION REVOKED
-- --------------------------------------------------------------------------
--
-- BEGIN;
--
-- UPDATE lys.twitch_channels
-- SET
--     channel_bot_authorized    = false,
--     authorization_checked_at  = now(),
--     authorization_revoked_at  = now()
-- WHERE broadcaster_id = $1;
--
-- INSERT INTO lys.twitch_channel_events (
--     broadcaster_id,
--     broadcaster_login,
--     event_type
-- )
-- VALUES ($1, $2, 'authorization_revoked');
--
-- COMMIT;
--
--
-- --------------------------------------------------------------------------
-- D. BROADCASTER TEMPORARILY DISABLES LYS
-- --------------------------------------------------------------------------
--
-- BEGIN;
--
-- UPDATE lys.twitch_channels
-- SET bot_enabled = false
-- WHERE broadcaster_id = $1;
--
-- INSERT INTO lys.twitch_channel_events (
--     broadcaster_id,
--     broadcaster_login,
--     event_type
-- )
-- VALUES ($1, $2, 'disabled');
--
-- COMMIT;
--
--
-- --------------------------------------------------------------------------
-- E. ADMINISTRATIVE BLOCK
-- --------------------------------------------------------------------------
--
-- BEGIN;
--
-- UPDATE lys.twitch_channels
-- SET
--     admin_blocked      = true,
--     admin_blocked_at   = now(),
--     admin_block_reason = $3
-- WHERE broadcaster_id = $1;
--
-- INSERT INTO lys.twitch_channel_events (
--     broadcaster_id,
--     broadcaster_login,
--     event_type,
--     metadata
-- )
-- VALUES (
--     $1,
--     $2,
--     'admin_blocked',
--     jsonb_build_object('reason', $3)
-- );
--
-- COMMIT;
--
--
-- --------------------------------------------------------------------------
-- F. BOT TOKEN REFRESH
-- --------------------------------------------------------------------------
--
-- The actual sequence should be:
--
--   1. Read refresh_token_secret_key from twitch_bot_accounts.
--   2. Retrieve the current refresh token from Cove.
--   3. Exchange it with Twitch for a new access token + refresh token.
--   4. Update BOTH secrets in Cove.
--   5. Update only non-secret metadata in PostgreSQL:
--
-- UPDATE lys.twitch_bot_accounts
-- SET
--     oauth_scopes             = $2,
--     access_token_expires_at  = $3,
--     last_refreshed_at        = now(),
--     last_validated_at        = now(),
--     authorization_revoked_at = NULL
-- WHERE bot_user_id = $1;
--
-- Do not copy the new token values into PostgreSQL.
--
--
-- ============================================================================
-- Deliberately omitted from this schema
-- ============================================================================
--
-- 1. Broadcaster access_token / refresh_token columns
--
--    They are not required for Lys's current model where all runtime actions
--    are performed as the LysLink bot identity. If Lys later adds a feature
--    that must act AS THE BROADCASTER, model that authorization separately
--    rather than putting user OAuth secrets back into twitch_channels.
--
--    A future table could look conceptually like:
--
--        lys.twitch_broadcaster_oauth
--
--    and would still store Cove secret references rather than token values.
--
--
-- 2. App access token storage
--
--    App access tokens are runtime credentials that can be recreated from the
--    Twitch application's client credentials. Keep the client secret in Cove
--    and cache the app access token in memory.
--
--
-- 3. Automatic "create settings row" trigger
--
--    Channel authorization is a domain workflow. The Go service should create
--    twitch_channels, twitch_channel_settings, and the initial event together
--    in one explicit transaction so the behavior is visible and testable.
--
--
-- 4. A generic admin_settings table
--
--    Generic settings tables become dumping grounds for unrelated concepts.
--    Twitch bot identity/authentication has enough meaning to deserve a
--    purpose-built model.
--
--
-- ============================================================================
-- End
-- ============================================================================

COMMIT;
