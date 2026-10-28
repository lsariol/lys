#!/bin/sh
# A release's migrations rehearsed on a copy of prod, in a throwaway Postgres.
#   rehearse.sh up <lys_db dump>   restore it as prod has it, migrate, check
#   rehearse.sh down               remove the container and all its data
# The dump is pgAdmin's Backup… of prod lys_db (Format Custom), kept outside
# any repository. Run from the Lys repo (Git Bash). Passwords exist only in
# this container.
set -eu
export MSYS_NO_PATHCONV=1

NAME=lys-rehearsal
PORT=55434
URL="127.0.0.1:$PORT"
TMP="${TMPDIR:-/tmp}"

sql() { docker exec -i "$NAME" psql -U Admin -v ON_ERROR_STOP=1 -X -q "$@"; }

# counts prints "table rows" for every table in the lys schema, sorted.
counts() {
	sql -d lys_db -At -c "SELECT string_agg(format('SELECT %L, count(*) FROM lys.%I', tablename, tablename), ' UNION ALL ') FROM pg_tables WHERE schemaname = 'lys'" |
		sql -d lys_db -At -F' ' | sort
}

case "${1:-}" in
up)
	DUMP="${2:?Usage: rehearse.sh up <lys_db dump>}"
	[ -s "$DUMP" ] || { echo "✗ $DUMP is missing or empty" >&2; exit 1; }

	echo "== 1. A fresh Postgres 16.4 with prod's roles"
	docker rm -fv "$NAME" >/dev/null 2>&1 || true
	docker run -d --name "$NAME" -p "$URL:5432" -e POSTGRES_USER=Admin -e POSTGRES_PASSWORD=admin postgres:16.4 >/dev/null
	until docker exec "$NAME" pg_isready -U Admin -q 2>/dev/null; do sleep 1; done
	sleep 2
	until docker exec "$NAME" pg_isready -U Admin -q 2>/dev/null; do sleep 1; done
	sql -d postgres <<'EOF'
CREATE ROLE lys_owner NOLOGIN;
CREATE ROLE lys_migrator LOGIN PASSWORD 'migrator';
CREATE ROLE lys_app LOGIN PASSWORD 'app';
CREATE ROLE lys_reader LOGIN PASSWORD 'reader';
GRANT lys_owner TO lys_migrator;
ALTER ROLE lys_migrator SET role = 'lys_owner';
CREATE DATABASE lys_db OWNER lys_owner;
EOF

	echo "== 2. Restore the dump (pg_restore 18 reads pgAdmin's format)"
	docker run --rm -i postgres:18-alpine pg_restore -f - <"$DUMP" |
		grep -v -e '^SET transaction_timeout' -e '^\\restrict ' -e '^\\unrestrict ' | sql -d lys_db >/dev/null
	counts >"$TMP/rehearsal-before.txt"

	echo "== 3. Lys's startup steps"
	REHEARSAL_MIGRATOR_URL="postgres://lys_migrator:migrator@$URL/lys_db?sslmode=disable" \
		REHEARSAL_APP_URL="postgres://lys_app:app@$URL/lys_db?sslmode=disable" \
		go run ./scripts/rehearse

	echo "== 4. Rows per table (before after; '-' where the table didn't exist)"
	counts >"$TMP/rehearsal-after.txt"
	join -a1 -a2 -e - -o 0,1.2,2.2 "$TMP/rehearsal-before.txt" "$TMP/rehearsal-after.txt"
	rm -f "$TMP/rehearsal-before.txt" "$TMP/rehearsal-after.txt"

	echo "== 5. The schema matches testdata/schema.txt"
	LYS_TEST_MIGRATOR_DATABASE_URL="postgres://lys_migrator:migrator@$URL/lys_db?sslmode=disable" \
		LYS_TEST_DATABASE_URL="postgres://lys_app:app@$URL/lys_db?sslmode=disable" \
		go test -count=1 ./internal/database -run '^TestMigrations$'

	echo "✓ Rehearsal passed. Remove it with: sh scripts/rehearse/rehearse.sh down"
	;;
down)
	docker rm -fv "$NAME" >/dev/null
	echo "✓ Rehearsal database removed."
	;;
*)
	echo "Usage: rehearse.sh up <lys_db dump> | down" >&2
	exit 2
	;;
esac
