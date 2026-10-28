-- v0.1.0 stops writing the old Twitch ID and name columns, so they become
-- optional. v0.0.0 still fills them, so it runs on this schema too; v0.1.1
-- drops them. Also the two indexes the OneUp stats were missing.

-- +goose Up

ALTER TABLE lys.game_events
    ALTER COLUMN user_id DROP NOT NULL,
    ALTER COLUMN username DROP NOT NULL,
    ALTER COLUMN channel_id DROP NOT NULL,
    ALTER COLUMN channel_name DROP NOT NULL;

ALTER TABLE lys.feature_requests
    ALTER COLUMN user_id DROP NOT NULL,
    ALTER COLUMN username DROP NOT NULL,
    ALTER COLUMN channel_id DROP NOT NULL,
    ALTER COLUMN channel_name DROP NOT NULL;

ALTER TABLE lys.sleeper_leagues ALTER COLUMN twitch_channel_id DROP NOT NULL;
ALTER TABLE lys.sleeper_connections ALTER COLUMN twitch_user_id DROP NOT NULL;

-- Stats by puzzle number, fastest first.
CREATE INDEX game_events_oneuppuzzle_puzzle_time_idx ON lys.game_events
    ((((details ->> 'puzzle_id'))::integer), (((details ->> 'time_seconds'))::integer))
    WHERE game_code = 'oneuppuzzle' AND NOT is_flagged;

-- Stats by a viewer's name.
CREATE INDEX platform_users_platform_name_idx ON lys.platform_users (platform, lower(display_name));
