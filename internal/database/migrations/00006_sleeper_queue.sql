-- Sleeper's announcements wait in sleeper_events until the sender posts them,
-- one per league every 90 seconds. send_after marks a row as queued: rows
-- recorded without announcing them (a league's first check, an alert that's
-- off) leave it empty, so they're never sent. v0.1.0 ignores the column.

-- +goose Up

ALTER TABLE lys.sleeper_events ADD COLUMN send_after timestamptz;

CREATE INDEX sleeper_events_queue_idx ON lys.sleeper_events (sleeper_league_id, send_after)
    WHERE published_at IS NULL AND send_after IS NOT NULL;
