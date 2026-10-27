package connections

import (
	"context"
	"testing"
)

func TestRevoke(t *testing.T) {
	pool := open(t)
	ctx := context.Background()
	s := New(pool)

	c, _, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-1", Name: "ConnTest"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // a second revocation is a no-op
		if err := s.Revoke(ctx, "twitch", "conntest-1", "the bot was banned in the channel"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Revoke(ctx, "twitch", "conntest-unknown", "x"); err != nil {
		t.Fatalf("unknown channel: %v", err)
	}

	got, err := s.Find(ctx, "twitch", "conntest-1")
	if err != nil || got.Authorized || got.Active() {
		t.Fatalf("after revoke: %+v, %v", got, err)
	}
	chans, err := s.Channels(ctx, "twitch")
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chans {
		if ch.ID == "conntest-1" {
			t.Fatal("a revoked channel is still in the directory")
		}
	}
	var legacyActive bool
	if err := pool.QueryRow(ctx, `SELECT active FROM lys.twitch_channels WHERE user_id = 'conntest-1'`).Scan(&legacyActive); err != nil || legacyActive {
		t.Fatalf("rollback copy: active %v, %v", legacyActive, err)
	}

	if _, _, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-1", Name: "ConnTest"}); err != nil {
		t.Fatal(err)
	}
	var events []string
	rows, err := pool.Query(ctx, `SELECT event_type::text FROM lys.connection_events WHERE connection_id = $1 ORDER BY id`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if len(events) != 3 || events[1] != "authorization_revoked" || events[2] != "reauthorized" {
		t.Fatalf("events: %v", events)
	}
}
