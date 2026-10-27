package connections

import (
	"context"
	"errors"
	"testing"
)

func TestFind(t *testing.T) {
	pool := open(t)
	ctx := context.Background()
	s := New(pool)

	for _, id := range []string{"conntest-a", "conntest-b"} {
		if _, _, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: id, Name: "SameName"}); err != nil {
			t.Fatal(err)
		}
	}
	var stale string
	if err := pool.QueryRow(ctx, `SELECT username FROM lys.twitch_channels WHERE user_id = 'conntest-a'`).Scan(&stale); err != nil || stale != "SameName~conntest-a" {
		t.Fatalf("the rollback copy keeps one row per name: %q %v", stale, err)
	}
	if _, err := s.Find(ctx, "twitch", "samename"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("shared name: %v", err)
	}
	if c, err := s.Find(ctx, "twitch", "conntest-b"); err != nil || c.ExternalID != "conntest-b" {
		t.Fatalf("by ID: %+v %v", c, err)
	}
	if _, err := s.Find(ctx, "twitch", "conntest-missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE lys.platform_connections SET name = 'OtherName' WHERE external_id = 'conntest-a'`); err != nil {
		t.Fatal(err)
	}
	if c, err := s.Find(ctx, "twitch", "SAMENAME"); err != nil || c.ExternalID != "conntest-b" {
		t.Fatalf("unique name: %+v %v", c, err)
	}
}

func TestWasAuthorized(t *testing.T) {
	pool := open(t)
	ctx := context.Background()
	s := New(pool)

	if _, _, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-revoked", Name: "Revoked"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, "twitch", "conntest-revoked", "test"); err != nil {
		t.Fatal(err)
	}
	// As LinkLegacyRows adds a channel only seen in feature data.
	if _, err := pool.Exec(ctx, `WITH c AS (INSERT INTO lys.communities (name) VALUES ('Seen') RETURNING id),
		pc AS (INSERT INTO lys.platform_connections (community_id, platform, external_id, name, authorized)
			SELECT id, 'twitch', 'conntest-seen', 'Seen', false FROM c RETURNING id)
		INSERT INTO lys.connection_events (connection_id, event_type, metadata)
			SELECT id, 'enrolled', '{"source": "feature data", "authorized": false}' FROM pc`); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[string]bool{"conntest-revoked": true, "conntest-seen": false} {
		c, err := s.Find(ctx, "twitch", id)
		if err != nil || c.Authorized || c.WasAuthorized != want {
			t.Errorf("%s: %+v %v", id, c, err)
		}
	}
	if _, _, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-seen", Name: "Seen"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.Find(ctx, "twitch", "conntest-seen"); !c.Authorized || !c.WasAuthorized {
		t.Fatalf("after adding it: %+v", c)
	}
}
