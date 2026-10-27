package database

import (
	"context"
	"database/sql"
	"testing"
)

// TestIdentityCopy runs before the other tests (file order): it migrates to
// the baseline, adds old-style rows, then checks 00002 copies them.
func TestIdentityCopy(t *testing.T) {
	if migratorURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL isn't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()

	provider, db, err := newMigrationProvider(ctx, migratorURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version > 1 {
		t.Skipf("the database is already at version %d; this test needs a fresh one (scripts/test-db.sh down, then up)", version)
	}
	if _, err := provider.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}

	exec(t, db, `INSERT INTO lys.twitch_channels (user_id, username, access_token, refresh_token, active, banned, times_joined, date_joined)
		VALUES ('ct-1', 'CopyOne', 'a', 'r', true, false, 3, now() - interval '2 days'),
		       ('ct-2', 'CopyBanned', 'a', 'r', true, true, 1, now() - interval '1 day'),
		       ('ct-3', 'CopyRevoked', 'a', 'r', false, false, 1, now())`)
	exec(t, db, `UPDATE lys.twitch_settings SET command_prefix = '?' WHERE user_id = 'ct-1'`)
	exec(t, db, `INSERT INTO lys.game_events (user_id, username, channel_id, channel_name, game_code, raw_message, occurred_at)
		VALUES ('cu-1', 'OldName', 'ct-1', 'CopyOne', 'oneuppuzzle', 'x', now() - interval '1 day'),
		       ('cu-1', 'NewName', 'ct-1', 'CopyOne', 'oneuppuzzle', 'x', now())`)
	exec(t, db, `INSERT INTO lys.feature_requests (user_id, username, channel_id, channel_name, platform, body)
		VALUES ('cu-2', 'Requester', 'ct-1', 'CopyOne', 'twitch', 'idea')`)

	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		id, name, prefix    string
		authorized, blocked bool
	}{
		{"ct-1", "CopyOne", "?", true, false},
		{"ct-2", "CopyBanned", "!", true, true},
		{"ct-3", "CopyRevoked", "!", false, false},
	} {
		var name, prefix, login string
		var authorized, blocked bool
		var events int
		err := db.QueryRowContext(ctx, `
			SELECT pc.name, pc.authorized, pc.admin_blocked, cs.command_prefix, td.broadcaster_login::text,
			       (SELECT count(*) FROM lys.connection_events e WHERE e.connection_id = pc.id AND e.event_type = 'enrolled')
			FROM lys.platform_connections pc
			JOIN lys.connection_settings cs ON cs.connection_id = pc.id
			JOIN lys.twitch_connection_details td ON td.connection_id = pc.id
			JOIN lys.communities c ON c.id = pc.community_id
			WHERE pc.platform = 'twitch' AND pc.external_id = $1`, tc.id).
			Scan(&name, &authorized, &blocked, &prefix, &login, &events)
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if name != tc.name || login != tc.name || prefix != tc.prefix || authorized != tc.authorized || blocked != tc.blocked || events != 1 {
			t.Errorf("%s: got name %q login %q prefix %q authorized %v blocked %v events %d", tc.id, name, login, prefix, authorized, blocked, events)
		}
	}

	for id, want := range map[string]string{"cu-1": "NewName", "cu-2": "Requester", "ct-1": "CopyOne"} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT display_name FROM lys.platform_users WHERE platform = 'twitch' AND external_id = $1`, id).Scan(&name); err != nil {
			t.Fatalf("platform user %s: %v", id, err)
		}
		if name != want {
			t.Errorf("platform user %s: got %q, want %q", id, name, want)
		}
	}
}

func exec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query); err != nil {
		t.Fatal(err)
	}
}
