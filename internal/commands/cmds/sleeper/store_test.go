package sleeper

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/database"
)

// TestStore needs a Postgres set up by scripts/test-db.sh.
func TestStore(t *testing.T) {
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
	sl := &Sleeper{db: pool}

	for _, q := range []string{
		`DELETE FROM lys.sleeper_leagues WHERE sleeper_league_id = 'slte-league'`,
		`DELETE FROM lys.communities WHERE name = 'SleeperTest'`,
		`DELETE FROM lys.platform_users WHERE external_id = 'slte-u1'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var community, member int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('SleeperTest') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_users (platform, external_id, display_name) VALUES ('twitch', 'slte-u1', 'Manager') RETURNING id`).Scan(&member); err != nil {
		t.Fatal(err)
	}

	if l, err := getLeague(ctx, community, sl); err != nil || l != nil {
		t.Fatalf("no league yet: %+v, %v", l, err)
	}
	if err, ok := addLeagueLink(&League{LeagueID: "slte-league", Name: "Test League", Season: "2026"}, "slte-ch", community, ctx, sl); !ok {
		t.Fatal(err)
	}
	league, err := getLeague(ctx, community, sl)
	if err != nil || league == nil || league.SleeperLeagueID != "slte-league" || league.CommunityID != community || league.LeagueName != "Test League" {
		t.Fatalf("league: %+v, %v", league, err)
	}
	if conflict, err := FindLeagueLinkConflict("other-league", community, ctx, sl); err != nil || conflict == nil || conflict.CommunityID != community {
		t.Fatalf("conflict by community: %+v, %v", conflict, err)
	}

	if err := linkUser(ctx, league.ID, member, "slte-u1", "Manager", "s-1", "sleeperName", 4, sl); err != nil {
		t.Fatal(err)
	}
	byID, err := getConnectionByUserID(ctx, league.ID, member, sl)
	if err != nil || byID == nil || byID.SleeperRosterID != 4 || byID.DisplayName != "Manager" {
		t.Fatalf("by ID: %+v, %v", byID, err)
	}

	// Lookups by name use the current chat name.
	if _, err := pool.Exec(ctx, `UPDATE lys.platform_users SET display_name = 'NewManager' WHERE id = $1`, member); err != nil {
		t.Fatal(err)
	}
	byName, err := getConnectionByUsername(ctx, league.ID, "@newmanager", sl)
	if err != nil || byName == nil || byName.PlatformUserID != member {
		t.Fatalf("by name: %+v, %v", byName, err)
	}

	if removed, err := removeUser(ctx, league.ID, member, sl); err != nil || !removed {
		t.Fatalf("remove: %v, %v", removed, err)
	}
	if err := removeLeagueLink(ctx, league.ID, sl); err != nil {
		t.Fatal(err)
	}
}
