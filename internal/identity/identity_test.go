package identity

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/database"
)

// TestResolve needs a Postgres set up by scripts/test-db.sh.
func TestResolve(t *testing.T) {
	migratorURL, appURL := os.Getenv("LYS_TEST_MIGRATOR_DATABASE_URL"), os.Getenv("LYS_TEST_DATABASE_URL")
	if migratorURL == "" || appURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL and LYS_TEST_DATABASE_URL aren't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()
	if err := database.Migrate(ctx, migratorURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DELETE FROM lys.platform_users WHERE external_id LIKE 'idtest-%'`); err != nil {
		t.Fatal(err)
	}

	s := New(pool)
	first, err := s.Resolve(ctx, "twitch", core.User{ID: "idtest-1", Name: "First"})
	if err != nil || first.PlatformUserID == 0 || first.AccountID != 0 {
		t.Fatalf("first: %+v, %v", first, err)
	}

	// A new display name goes to the database, under the same ID.
	renamed, err := s.Resolve(ctx, "twitch", core.User{ID: "idtest-1", Name: "Renamed"})
	if err != nil || renamed != first {
		t.Fatalf("renamed: %+v, %v; want %+v", renamed, err, first)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT display_name FROM lys.platform_users WHERE id = $1`, first.PlatformUserID).Scan(&name); err != nil || name != "Renamed" {
		t.Fatalf("display name: %q, %v", name, err)
	}

	// The same ID on another platform is another person.
	other, err := s.Resolve(ctx, "discord", core.User{ID: "idtest-1", Name: "First"})
	if err != nil || other.PlatformUserID == first.PlatformUserID {
		t.Fatalf("other platform: %+v, %v", other, err)
	}

	if _, err := s.Resolve(ctx, "twitch", core.User{}); err == nil {
		t.Fatal("a user without an ID was accepted")
	}
}
