package oneuppuzzle

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

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DELETE FROM lys.game_events WHERE channel_id = 'oute-ch'`)
	exec(`DELETE FROM lys.communities WHERE name = 'OneUpTest'`)
	exec(`DELETE FROM lys.platform_users WHERE external_id LIKE 'oute-%'`)

	var community, conn, fast, slow int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('OneUpTest') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_connections (community_id, platform, external_id, name) VALUES ($1, 'twitch', 'oute-ch', 'OneUpTest') RETURNING id`, community).Scan(&conn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_users (platform, external_id, display_name) VALUES ('twitch', 'oute-fast', 'Speedy') RETURNING id`).Scan(&fast); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_users (platform, external_id, display_name) VALUES ('twitch', 'oute-slow', 'Snail') RETURNING id`).Scan(&slow); err != nil {
		t.Fatal(err)
	}

	s := &store{pool: pool}
	game := func(user int64, name string, puzzle, seconds int) OneUpGame {
		return OneUpGame{
			UserID: "x", Username: name, ChannelID: "oute-ch", ChannelName: "OneUpTest", GameCode: "oneuppuzzle", RawMessage: "x",
			Details: OneUpGameDetails{PuzzleID: puzzle, TimeSeconds: seconds}, ConnectionID: conn, PlatformUserID: user,
		}
	}
	for _, g := range []OneUpGame{game(fast, "Speedy", 990001, 60), game(slow, "Snail", 990001, 600), game(fast, "Speedy", 990002, 90)} {
		if err := s.storeGame(ctx, g, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.storeGame(ctx, game(slow, "Snail", 990003, 5), true); err != nil {
		t.Fatal(err)
	}

	if recorded, err := s.isPuzzleRecorded(ctx, fast, 990001); err != nil || !recorded {
		t.Fatalf("recorded: %v, %v", recorded, err)
	}
	if recorded, err := s.isPuzzleRecorded(ctx, slow, 990002); err != nil || recorded {
		t.Fatalf("not recorded: %v, %v", recorded, err)
	}

	// Speedy renames; their stats and records follow them.
	exec(`UPDATE lys.platform_users SET display_name = 'Speedier' WHERE id = $1`, fast)

	cp, ok, err := s.puzzleStatsByChannel(ctx, conn)
	if err != nil || !ok || *cp.GamesCompleted != 3 || *cp.FastestUser != "Speedier" || *cp.SlowestUser != "Snail" || *cp.Completions.FastestTime != 60 {
		t.Fatalf("channel stats: %+v ok %v err %v", cp, ok, err)
	}

	up, ok, err := s.puzzleStatsByUser(ctx, "twitch", "speedier")
	if err != nil || !ok || *up.GamesCompleted != 2 || *up.Completions.SlowestTime != 90 {
		t.Fatalf("user stats: %+v ok %v err %v", up, ok, err)
	}
	if _, ok, err := s.puzzleStatsByUser(ctx, "twitch", "Speedy"); err != nil || ok {
		t.Fatalf("old name still matched: ok %v err %v", ok, err)
	}

	gp, ok, err := s.puzzleStatsByPuzzleID(ctx, 990001)
	if err != nil || !ok || *gp.GamesCompleted != 2 || *gp.FastestUser != "Speedier" {
		t.Fatalf("puzzle stats: %+v ok %v err %v", gp, ok, err)
	}

	if _, ok, err := s.puzzleStatsByChannel(ctx, 0); err != nil || ok {
		t.Fatalf("unknown channel: ok %v err %v", ok, err)
	}
}
