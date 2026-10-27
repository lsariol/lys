package connections

import (
	"context"
	"testing"
	"time"
)

func TestWatch(t *testing.T) {
	pool := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes := make(chan string, 10)
	watcher := New(pool)
	done := make(chan error)
	go func() { done <- watcher.Watch(ctx, time.Hour, func(p string) { changes <- p }) }()

	// Another store, as a CLI in another process would use.
	s := New(pool)
	expect := func(what string, change func() error) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			if err := change(); err != nil {
				t.Fatal(err)
			}
			select {
			case p := <-changes:
				if p != "twitch" {
					t.Fatalf("%s: got platform %q", what, p)
				}
				return
			case <-time.After(200 * time.Millisecond):
				// The listener may not be listening yet; change again.
			case <-deadline:
				t.Fatalf("%s: no notification", what)
			}
		}
	}

	var id int64
	expect("enroll", func() error {
		c, _, err := s.Enroll(context.Background(), Enrollment{Platform: "twitch", ExternalID: "conntest-1", Name: "ConnTest"})
		id = c.ID
		return err
	})
	expect("block", func() error { return s.SetBlocked(context.Background(), id, true, "") })
	expect("prefix", func() error { return s.SetPrefix(context.Background(), "twitch", "conntest-1", "?") })

	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("Watch returned %v", err)
	}
}
