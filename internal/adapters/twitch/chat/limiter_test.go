package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/core"
)

// testLimiter allows 4 messages per window, 2 of them where the bot isn't privileged.
func testLimiter(window time.Duration) *limiter {
	l := newLimiter()
	l.window, l.all, l.unprivileged = window, 4, 2
	return l
}

func TestLimiterBuckets(t *testing.T) {
	l := testLimiter(300 * time.Millisecond)
	ctx := context.Background()
	start := time.Now()
	for range 2 {
		if err := l.wait(ctx, false, core.PriorityNormal); err != nil {
			t.Fatal(err)
		}
	}
	// The unprivileged bucket is full; privileged messages may still go.
	for range 2 {
		if err := l.wait(ctx, true, core.PriorityNormal); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("4 messages within the limits waited %s", d)
	}
	// Both buckets are full: the next waits for the window.
	if err := l.wait(ctx, true, core.PriorityNormal); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 250*time.Millisecond {
		t.Fatalf("a fifth message went out after %s, inside the window", d)
	}
}

func TestLimiterNeverExceeds(t *testing.T) {
	const window = 200 * time.Millisecond
	l := testLimiter(window)
	var mu sync.Mutex
	var sent []time.Time
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.wait(context.Background(), i%2 == 0, core.PriorityNormal); err != nil {
				t.Error(err)
			}
			mu.Lock()
			sent = append(sent, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()
	for i := range sent {
		in := 0
		for j := range sent {
			if d := sent[j].Sub(sent[i]); d >= 0 && d < window-20*time.Millisecond {
				in++
			}
		}
		if in > 4 {
			t.Fatalf("%d messages within one window", in)
		}
	}
}

func TestLimiterPriority(t *testing.T) {
	l := testLimiter(300 * time.Millisecond)
	ctx := context.Background()
	for range 4 {
		if err := l.wait(ctx, true, core.PriorityNormal); err != nil {
			t.Fatal(err)
		}
	}
	order := make(chan core.Priority, 2)
	go func() {
		_ = l.wait(ctx, true, core.PriorityLow)
		order <- core.PriorityLow
	}()
	time.Sleep(20 * time.Millisecond) // the autopost waits first
	go func() {
		_ = l.wait(ctx, true, core.PriorityReply)
		order <- core.PriorityReply
	}()
	if first := <-order; first != core.PriorityReply {
		t.Fatalf("an autopost went before a reply that came later")
	}
	<-order
}

func TestLimiterCancel(t *testing.T) {
	l := testLimiter(time.Hour)
	for range 4 {
		_ = l.wait(context.Background(), true, core.PriorityNormal)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := l.wait(ctx, true, core.PriorityReply); err == nil {
		t.Fatal("waited past the context")
	}
	if l.higherWaiting(core.PriorityLow) {
		t.Fatal("a cancelled wait still counts as waiting")
	}
}

func TestRoles(t *testing.T) {
	c := &ChatClient{Config: nil, privileged: map[string]bool{}}
	c.Config = testConfig()
	if c.isPrivileged("1") {
		t.Fatal("privileged before the bot spoke")
	}
	c.SetRoles("1", core.RoleModerator)
	if !c.isPrivileged("1") {
		t.Fatal("a moderator isn't privileged")
	}
	c.SetRoles("1", core.RoleSubscriber)
	if c.isPrivileged("1") {
		t.Fatal("still privileged after losing moderator")
	}
	if !c.isPrivileged(c.Config.BotAccountID) {
		t.Fatal("the bot's own channel isn't privileged")
	}
}

func testConfig() *config.TwitchConfig {
	return &config.TwitchConfig{ClientID: "id", BotAccountID: "bot"}
}
