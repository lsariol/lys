-- Jobs that run at a time: reminders, repeating channel autoposts, and whatever
-- comes later. A job survives restarts; whichever Lys instance claims a due
-- job first runs it, with a lease so a crashed run is picked up again.

-- +goose Up

CREATE TABLE lys.scheduled_jobs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL,
    connection_id bigint NOT NULL REFERENCES lys.platform_connections(id) ON DELETE CASCADE,
    created_by bigint REFERENCES lys.platform_users(id) ON DELETE SET NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    run_at timestamptz NOT NULL,
    repeat_every interval CHECK (repeat_every IS NULL OR repeat_every >= interval '1 minute'),
    attempts integer NOT NULL DEFAULT 0,
    last_error text,
    claimed_until timestamptz,
    done_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX scheduled_jobs_due_idx ON lys.scheduled_jobs (run_at) WHERE done_at IS NULL;
CREATE INDEX scheduled_jobs_connection_idx ON lys.scheduled_jobs (connection_id, kind) WHERE done_at IS NULL;
CREATE INDEX scheduled_jobs_creator_idx ON lys.scheduled_jobs (created_by, kind) WHERE done_at IS NULL;
CREATE TRIGGER scheduled_jobs_set_updated_at BEFORE UPDATE ON lys.scheduled_jobs
    FOR EACH ROW EXECUTE FUNCTION lys.set_updated_at();
