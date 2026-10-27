package database

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/config"
)

// These tests need a Postgres set up with scripts/db/setup.sql
// (scripts/test-db.sh does it). Without these variables they're skipped.
var (
	migratorURL = os.Getenv("LYS_TEST_MIGRATOR_DATABASE_URL")
	appURL      = os.Getenv("LYS_TEST_DATABASE_URL")
	// goldenSourceURL, with -update, writes the golden file from that database
	// instead of migrating: point it at a copy of prod to capture prod's schema.
	goldenSourceURL = os.Getenv("LYS_TEST_GOLDEN_SOURCE_URL")
)

var update = flag.Bool("update", false, "rewrite testdata/schema.txt")

const goldenFile = "testdata/schema.txt"

// TestMigrations checks that migrating twice works, the app role then passes
// CheckSchemaVersion, and the schema matches testdata/schema.txt (prod's
// schema plus every migration since the baseline).
func TestMigrations(t *testing.T) {
	if migratorURL == "" || appURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL and LYS_TEST_DATABASE_URL aren't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()

	if *update && goldenSourceURL != "" {
		writeGolden(t, describeURL(t, goldenSourceURL))
		return
	}

	for i := range 2 {
		if err := Migrate(ctx, migratorURL); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}

	db := NewDatabase(&config.DatabaseConfig{ConnectionString: appURL})
	if err := db.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	if err := db.CheckSchemaVersion(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := describeSchema(ctx, db.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		writeGolden(t, got)
		return
	}
	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("the schema doesn't match %s:\n%s", goldenFile, lineDiff(string(want), got))
	}
}

// TestBaselineTriggers checks, as the app role, that adding a channel creates
// its settings row and logs a grant.
func TestBaselineTriggers(t *testing.T) {
	if migratorURL == "" || appURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL and LYS_TEST_DATABASE_URL aren't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()
	if err := Migrate(ctx, migratorURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	const id = "test-channel-0"
	cleanup := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM lys.twitch_channels WHERE user_id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx, `INSERT INTO lys.twitch_channels (user_id, username, access_token, refresh_token) VALUES ($1, 'Test_Channel_0', '', '')`, id); err != nil {
		t.Fatal(err)
	}

	var prefix string
	if err := pool.QueryRow(ctx, `SELECT command_prefix FROM lys.twitch_settings WHERE user_id = $1`, id).Scan(&prefix); err != nil {
		t.Fatalf("settings row: %v", err)
	}
	if prefix != "!" {
		t.Fatalf("command prefix: got %q, want \"!\"", prefix)
	}

	var action string
	if err := pool.QueryRow(ctx, `SELECT action::text FROM lys.twitch_channel_events WHERE user_id = $1`, id).Scan(&action); err != nil {
		t.Fatalf("channel event: %v", err)
	}
	if action != "grant" {
		t.Fatalf("channel event: got %q, want \"grant\"", action)
	}
}

func describeURL(t *testing.T, url string) string {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s, err := describeSchema(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeGolden(t *testing.T, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(goldenFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goldenFile, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", goldenFile)
}

// describeSchema lists everything in the lys schema except goose's own table,
// one fact per line in a stable order.
func describeSchema(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	rows, err := pool.Query(ctx, describeQuery)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var key, line string
		if err := rows.Scan(&key, &line); err != nil {
			return "", err
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String(), rows.Err()
}

// lineDiff lists the lines only in want (-) or only in got (+).
func lineDiff(want, got string) string {
	in := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(s, "\n") {
			m[l] = true
		}
		return m
	}
	w, g := in(want), in(got)
	var b strings.Builder
	for _, l := range strings.Split(want, "\n") {
		if !g[l] {
			b.WriteString("- " + l + "\n")
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if !w[l] {
			b.WriteString("+ " + l + "\n")
		}
	}
	return b.String()
}

const describeQuery = `
WITH ns AS (SELECT oid FROM pg_namespace WHERE nspname = 'lys'),
rel AS (
  SELECT c.oid, c.relname, c.relkind, c.relowner, c.relacl FROM pg_class c
  WHERE c.relnamespace = (SELECT oid FROM ns) AND c.relname NOT LIKE 'goose_db_version%'
)
SELECT key, line FROM (
  SELECT '0' AS key, format('schema lys owner=%s acl=%s', pg_get_userbyid(nspowner),
         coalesce((SELECT string_agg(a::text, ' ' ORDER BY a::text) FROM unnest(nspacl) a), '-')) AS line
  FROM pg_namespace WHERE nspname = 'lys'

  UNION ALL
  SELECT '1 ' || pg_get_userbyid(defaclrole) || defaclobjtype::text,
         format('default privileges for=%s on=%s acl=%s', pg_get_userbyid(defaclrole), defaclobjtype,
         (SELECT string_agg(a::text, ' ' ORDER BY a::text) FROM unnest(defaclacl) a))
  FROM pg_default_acl WHERE defaclnamespace = (SELECT oid FROM ns)

  UNION ALL
  SELECT '2 ' || t.typname, format('type %s owner=%s enum=%s', t.typname, pg_get_userbyid(t.typowner),
         (SELECT string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder) FROM pg_enum e WHERE e.enumtypid = t.oid))
  FROM pg_type t WHERE t.typnamespace = (SELECT oid FROM ns) AND t.typtype = 'e'

  UNION ALL
  SELECT '3 ' || p.oid::regprocedure::text, format('function %s owner=%s%s%s', p.oid::regprocedure, pg_get_userbyid(p.proowner),
         E'\n', pg_get_functiondef(p.oid))
  FROM pg_proc p WHERE p.pronamespace = (SELECT oid FROM ns)

  UNION ALL
  SELECT '4 ' || r.relname || ' 0', format('table %s owner=%s acl=%s', r.relname, pg_get_userbyid(r.relowner),
         coalesce((SELECT string_agg(a::text, ' ' ORDER BY a::text) FROM unnest(r.relacl) a), '-'))
  FROM rel r WHERE r.relkind IN ('r', 'p')

  UNION ALL
  SELECT '4 ' || r.relname || ' 1 ' || lpad(a.attnum::text, 4, '0'),
         format('  column %s %s%s%s', a.attname, format_type(a.atttypid, a.atttypmod),
         CASE WHEN a.attnotnull THEN ' not null' ELSE '' END,
         coalesce(' default ' || pg_get_expr(d.adbin, d.adrelid), ''))
  FROM rel r JOIN pg_attribute a ON a.attrelid = r.oid
  LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
  WHERE r.relkind IN ('r', 'p') AND a.attnum > 0 AND NOT a.attisdropped

  UNION ALL
  SELECT '4 ' || r.relname || ' 2 ' || co.conname, format('  constraint %s %s', co.conname, pg_get_constraintdef(co.oid))
  FROM rel r JOIN pg_constraint co ON co.conrelid = r.oid

  UNION ALL
  SELECT '4 ' || r.relname || ' 3 ' || ic.relname, format('  index %s', pg_get_indexdef(i.indexrelid))
  FROM rel r JOIN pg_index i ON i.indrelid = r.oid JOIN pg_class ic ON ic.oid = i.indexrelid

  UNION ALL
  SELECT '4 ' || r.relname || ' 4 ' || tg.tgname, format('  trigger %s', pg_get_triggerdef(tg.oid))
  FROM rel r JOIN pg_trigger tg ON tg.tgrelid = r.oid WHERE NOT tg.tgisinternal

  UNION ALL
  SELECT '5 ' || r.relname, format('sequence %s owner=%s acl=%s type=%s start=%s increment=%s min=%s max=%s cache=%s cycle=%s owned_by=%s',
         r.relname, pg_get_userbyid(r.relowner),
         coalesce((SELECT string_agg(a::text, ' ' ORDER BY a::text) FROM unnest(r.relacl) a), '-'),
         s.seqtypid::regtype, s.seqstart, s.seqincrement, s.seqmin, s.seqmax, s.seqcache, s.seqcycle,
         coalesce((SELECT dc.relname || '.' || da.attname FROM pg_depend dep
                   JOIN pg_class dc ON dc.oid = dep.refobjid
                   JOIN pg_attribute da ON da.attrelid = dep.refobjid AND da.attnum = dep.refobjsubid
                   WHERE dep.objid = r.oid AND dep.classid = 'pg_class'::regclass AND dep.deptype IN ('a', 'i')), '-'))
  FROM rel r JOIN pg_sequence s ON s.seqrelid = r.oid
) facts
ORDER BY key`
