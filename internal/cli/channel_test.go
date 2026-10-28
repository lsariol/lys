package cli

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/connections"
	"github.com/lsariol/lys/internal/database"
)

var countLine = regexp.MustCompile(`(?m)^\d+ channels?\.$`)

// TestChannelCommands needs a Postgres set up by scripts/test-db.sh.
func TestChannelCommands(t *testing.T) {
	migratorURL, appURL := os.Getenv("LYS_TEST_MIGRATOR_DATABASE_URL"), os.Getenv("LYS_TEST_DATABASE_URL")
	if migratorURL == "" || appURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL and LYS_TEST_DATABASE_URL aren't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()
	if err := database.Migrate(ctx, migratorURL); err != nil {
		t.Fatal(err)
	}
	db := database.NewDatabase(&config.DatabaseConfig{ConnectionString: appURL})
	if err := db.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	for _, q := range []string{
		`DELETE FROM lys.communities WHERE id IN (SELECT community_id FROM lys.platform_connections WHERE external_id = 'clitest-1')`,
	} {
		if _, err := db.Pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := connections.New(db.Pool).Enroll(ctx, connections.Enrollment{Platform: "twitch", ExternalID: "clitest-1", Name: "CliTest"}); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (string, string, int) {
		env, out, errOut := testEnv(t)
		env.Services = func(context.Context) (*Services, error) {
			return &Services{Database: func(context.Context) (*database.Database, error) { return db, nil }}, nil
		}
		code := Run(ctx, env, args)
		return out.String(), errOut.String(), code
	}
	row := func(list string) string {
		for _, l := range strings.Split(list, "\n") {
			if strings.Contains(l, "clitest-1") {
				return l
			}
		}
		return ""
	}

	out, _, code := run("channel", "list")
	if code != 0 || !strings.Contains(row(out), "active") || !countLine.MatchString(out) {
		t.Fatalf("list: exit %d\n%s", code, out)
	}

	if _, errOut, code := run("channel", "block", "clitest"); code != 1 || !strings.Contains(errOut, "Block cancelled") {
		t.Fatalf("block without --yes: exit %d, %q", code, errOut)
	}
	if _, errOut, code := run("channel", "block", "CliTest", "--reason", "testing", "-y"); code != 0 || !strings.Contains(errOut, `✓ Blocked "CliTest"`) {
		t.Fatalf("block: exit %d, %q", code, errOut)
	}
	if out, _, _ := run("channel", "list"); !strings.Contains(row(out), "blocked (testing)") {
		t.Fatalf("list after block:\n%s", out)
	}
	if _, errOut, code := run("channel", "unblock", "clitest-1"); code != 0 || !strings.Contains(errOut, "Unblocked") {
		t.Fatalf("unblock: exit %d, %q", code, errOut)
	}
	if _, errOut, code := run("channel", "block", "nobody-here", "-y"); code != 1 || !strings.Contains(errOut, "✗ no such channel") {
		t.Fatalf("missing channel: exit %d, %q", code, errOut)
	}

	if _, errOut, code := run("channel", "prefix", "CliTest", "?"); code != 0 || !strings.Contains(errOut, `✓ "CliTest" now uses "?" (was "!")`) {
		t.Fatalf("prefix: exit %d, %q", code, errOut)
	}
	if out, _, _ := run("channel", "list"); !strings.Contains(row(out), " ? ") {
		t.Fatalf("list after prefix:\n%s", out)
	}
	for _, bad := range []string{"???", "a b"} {
		if _, errOut, code := run("channel", "prefix", "CliTest", bad); code != 1 || !strings.Contains(errOut, "Usage: lys channel prefix") {
			t.Fatalf("prefix %q: exit %d, %q", bad, code, errOut)
		}
	}
}
