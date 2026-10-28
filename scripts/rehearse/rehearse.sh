#!/bin/sh
# The v0.0.0 rehearsal (notes/v0.0.0-cutover.md, Appendix A) on a throwaway
# Postgres: restores a prod dump with prod's roles, runs the runbook's rename
# and ownership SQL, then Lys's startup steps, and checks the result.
#
#   scripts/rehearse/rehearse.sh up <prod dump>   (Git Bash, in the Lys repo)
#   scripts/rehearse/rehearse.sh down             remove it, with all its data
#
# The dump holds tokens: keep it outside any repository. The passwords below
# exist only in this container, reachable only from this machine.
set -eu
export MSYS_NO_PATHCONV=1

NAME=lys-rehearsal
PORT=55434
URL="127.0.0.1:$PORT"

sql() { docker exec -i "$NAME" psql -U Admin -v ON_ERROR_STOP=1 -X -q "$@"; }

counts() {
	sql -d "$1" -At -F' ' <<'EOF'
SELECT 'old twitch_channels', count(*) FROM lys.twitch_channels
UNION ALL SELECT 'old twitch_settings', count(*) FROM lys.twitch_settings
UNION ALL SELECT 'old twitch_channel_events', count(*) FROM lys.twitch_channel_events
UNION ALL SELECT 'old letterboxd_feed_subscriptions', count(*) FROM lys.letterboxd_feed_subscriptions
UNION ALL SELECT 'old letterboxd alert channels', count(*) FROM lys.letterboxd_feed_subscriptions, unnest(alert_channels)
UNION ALL SELECT 'game_events', count(*) FROM lys.game_events
UNION ALL SELECT 'feature_requests', count(*) FROM lys.feature_requests
UNION ALL SELECT 'sleeper_connections', count(*) FROM lys.sleeper_connections
UNION ALL SELECT 'sleeper_leagues', count(*) FROM lys.sleeper_leagues
UNION ALL SELECT 'sleeper_events', count(*) FROM lys.sleeper_events;
EOF
}

case "${1:-}" in
up)
	DUMP="${2:?Usage: scripts/rehearse/rehearse.sh up <prod dump>}"
	[ -s "$DUMP" ] || { echo "✗ $DUMP is missing or empty" >&2; exit 1; }

	echo "== 1. A fresh Postgres 16.4 with prod's roles"
	docker rm -fv "$NAME" >/dev/null 2>&1 || true
	docker run -d --name "$NAME" -p "$URL:5432" -e POSTGRES_USER=Admin -e POSTGRES_PASSWORD=admin postgres:16.4 >/dev/null
	until docker exec "$NAME" pg_isready -U Admin -q 2>/dev/null; do sleep 1; done
	sleep 2
	until docker exec "$NAME" pg_isready -U Admin -q 2>/dev/null; do sleep 1; done
	sql -d postgres <<'EOF'
CREATE ROLE botsuite_owner NOLOGIN;
CREATE ROLE botsuite_migrator LOGIN PASSWORD 'migrator';
CREATE ROLE botsuite_app LOGIN PASSWORD 'app';
CREATE ROLE botsuite_reader LOGIN PASSWORD 'reader';
GRANT botsuite_owner TO botsuite_migrator;
ALTER ROLE botsuite_migrator SET role = 'botsuite_owner';
CREATE DATABASE botsuite_db;
EOF

	echo "== 2. Restore the dump (pg_restore 18 reads pgAdmin's format)"
	docker run --rm -i postgres:18-alpine pg_restore -f - <"$DUMP" |
		grep -v -e '^SET transaction_timeout' -e '^\\restrict ' -e '^\\unrestrict ' | sql -d botsuite_db >/dev/null

	echo "== 3. Window step 4a: database and roles"
	sql -d postgres <<'EOF'
ALTER DATABASE botsuite_db RENAME TO lys_db;
ALTER ROLE botsuite_owner    RENAME TO lys_owner;
ALTER ROLE botsuite_migrator RENAME TO lys_migrator;
ALTER ROLE botsuite_app      RENAME TO lys_app;
ALTER ROLE botsuite_reader   RENAME TO lys_reader;
ALTER ROLE lys_migrator SET role = 'lys_owner';
EOF

	echo "== 4. Window step 4b: schema and functions"
	sql -d lys_db <<'EOF'
ALTER SCHEMA botsuite RENAME TO lys;
DO $$
DECLARE
  f   oid;
  def text;
BEGIN
  FOR f IN
    SELECT p.oid
    FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'lys' AND p.prokind = 'f'
  LOOP
    def := pg_get_functiondef(f);
    IF def LIKE '%botsuite.%' THEN
      EXECUTE replace(def, 'botsuite.', 'lys.');
    END IF;
  END LOOP;
END $$;
EOF
	left=$(sql -d lys_db -At -c "SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'lys' AND p.prokind = 'f' AND pg_get_functiondef(p.oid) LIKE '%botsuite.%'")
	echo "functions still naming botsuite: $left (want 0)"

	echo "== 5. Window step 5: hand everything to lys_owner"
	sql -d lys_db <<'EOF'
BEGIN;
ALTER DATABASE lys_db OWNER TO lys_owner;
ALTER TABLE lys.feature_requests OWNER TO lys_owner;
ALTER TABLE lys.game_events OWNER TO lys_owner;
ALTER TABLE lys.letterboxd_feed_subscriptions OWNER TO lys_owner;
ALTER TABLE lys.sleeper_connections OWNER TO lys_owner;
ALTER TABLE lys.sleeper_events OWNER TO lys_owner;
ALTER TABLE lys.sleeper_leagues OWNER TO lys_owner;
ALTER TYPE lys.request_status OWNER TO lys_owner;
ALTER FUNCTION lys.log_twitch_channel_changes() OWNER TO lys_owner;
GRANT SELECT ON SEQUENCE lys.feature_requests_id_seq, lys.feeds_id_seq TO lys_app;
ALTER DEFAULT PRIVILEGES FOR ROLE "Admin" IN SCHEMA lys REVOKE ALL ON TABLES FROM lys_app, lys_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE "Admin" IN SCHEMA lys REVOKE ALL ON SEQUENCES FROM lys_app;
COMMIT;
EOF
	echo "not owned by lys_owner (want none):"
	sql -d lys_db -At -F' ' <<'EOF'
SELECT 'database', datname, pg_get_userbyid(datdba) FROM pg_database WHERE datname = 'lys_db' AND pg_get_userbyid(datdba) <> 'lys_owner'
UNION ALL SELECT 'schema', nspname, pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = 'lys' AND pg_get_userbyid(nspowner) <> 'lys_owner'
UNION ALL SELECT 'table', tablename, tableowner FROM pg_tables WHERE schemaname = 'lys' AND tableowner <> 'lys_owner'
UNION ALL SELECT 'sequence', sequencename, sequenceowner FROM pg_sequences WHERE schemaname = 'lys' AND sequenceowner <> 'lys_owner'
UNION ALL SELECT 'type', t.typname, pg_get_userbyid(t.typowner) FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
  WHERE n.nspname = 'lys' AND t.typtype IN ('e', 'd') AND pg_get_userbyid(t.typowner) <> 'lys_owner'
UNION ALL SELECT 'function', p.proname, pg_get_userbyid(p.proowner) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
  WHERE n.nspname = 'lys' AND pg_get_userbyid(p.proowner) <> 'lys_owner';
EOF
	before=$(mktemp)
	counts lys_db >"$before"

	echo "== 6. Lys's startup steps"
	REHEARSAL_MIGRATOR_URL="postgres://lys_migrator:migrator@$URL/lys_db?sslmode=disable" \
		REHEARSAL_APP_URL="postgres://lys_app:app@$URL/lys_db?sslmode=disable" \
		go run ./scripts/rehearse

	echo "== 7. Old tables untouched (before | after)"
	counts lys_db | paste -d'|' "$before" -
	rm -f "$before"

	echo "== 8. The new tables"
	sql -d lys_db -At -F' ' <<'EOF'
SELECT 'communities', count(*) FROM lys.communities
UNION ALL SELECT 'platform_connections', count(*) FROM lys.platform_connections
UNION ALL SELECT '  authorized', count(*) FROM lys.platform_connections WHERE authorized
UNION ALL SELECT '  from feature data (not authorized)', count(*) FROM lys.connection_events WHERE metadata->>'source' = 'feature data'
UNION ALL SELECT 'connection_settings', count(*) FROM lys.connection_settings
UNION ALL SELECT 'connection_events', count(*) FROM lys.connection_events
UNION ALL SELECT 'bot_accounts', count(*) FROM lys.bot_accounts
UNION ALL SELECT 'platform_users', count(*) FROM lys.platform_users
UNION ALL SELECT 'feed_subscriptions', count(*) FROM lys.feed_subscriptions
UNION ALL SELECT 'unlinked game_events (want 0)', count(*) FROM lys.game_events WHERE connection_id IS NULL OR platform_user_id IS NULL
UNION ALL SELECT 'unlinked feature_requests (want 0)', count(*) FROM lys.feature_requests WHERE connection_id IS NULL OR platform_user_id IS NULL
UNION ALL SELECT 'unlinked sleeper_leagues (want 0)', count(*) FROM lys.sleeper_leagues WHERE community_id IS NULL
UNION ALL SELECT 'unlinked sleeper_connections (want 0)', count(*) FROM lys.sleeper_connections WHERE platform_user_id IS NULL;
EOF

	echo "== 9. The schema matches testdata/schema.txt"
	LYS_TEST_MIGRATOR_DATABASE_URL="postgres://lys_migrator:migrator@$URL/lys_db?sslmode=disable" \
		LYS_TEST_DATABASE_URL="postgres://lys_app:app@$URL/lys_db?sslmode=disable" \
		go test -count=1 ./internal/database -run '^TestMigrations$'

	echo "✓ Rehearsal passed. Remove it with: scripts/rehearse/rehearse.sh down"
	;;
down)
	docker rm -fv "$NAME" >/dev/null
	echo "✓ Rehearsal database removed."
	;;
*)
	echo "Usage: scripts/rehearse/rehearse.sh up <prod dump> | down" >&2
	exit 2
	;;
esac
