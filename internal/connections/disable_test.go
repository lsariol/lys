package connections

import (
	"context"
	"errors"
	"testing"
)

func TestDisable(t *testing.T) {
	pool := open(t)
	ctx := context.Background()
	s := New(pool)

	c, _, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-1", Name: "ConnTest"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // turning it off twice is fine
		if err := s.Disable(ctx, "twitch", "conntest-1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Disable(ctx, "twitch", "conntest-missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing channel: %v", err)
	}
	if got, _ := s.Find(ctx, "twitch", "conntest-1"); got.Enabled || got.Active() || !got.Authorized {
		t.Fatalf("after disable: %+v", got)
	}

	if _, _, err := s.Enroll(ctx, Enrollment{Platform: "twitch", ExternalID: "conntest-1", Name: "ConnTest"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Find(ctx, "twitch", "conntest-1"); !got.Active() {
		t.Fatalf("after enrolling again: %+v", got)
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
	if len(events) != 3 || events[1] != "disabled" || events[2] != "enabled" {
		t.Fatalf("events: %v", events)
	}
}
