package chat

import (
	"context"
	"sync"
	"time"

	"github.com/lsariol/lys/internal/core"
)

// Twitch's chat limits are per account, across all channels (dev.twitch.tv/docs/chat,
// checked 2026-10-28): 20 messages per 30 seconds, or 100 for messages in
// channels where the bot is a moderator, VIP or broadcaster, which still count
// towards the 20. Going over makes Twitch ignore the bot for an hour wherever
// it isn't privileged. Lys stays a little under both.
const (
	limitWindow       = 30 * time.Second
	limitAll          = 90 // of 100
	limitUnprivileged = 18 // of 20
)

// Per channel, Twitch allows 1 message a second where the bot isn't privileged.
const (
	unprivilegedGap = 1100 * time.Millisecond
	privilegedGap   = 300 * time.Millisecond
)

// limiter is the account-wide limit every channel's sender waits on. When
// messages wait, the highest priority goes first.
type limiter struct {
	window            time.Duration
	all, unprivileged int
	now               func() time.Time

	mu      sync.Mutex
	sent    []time.Time // within the window, oldest first
	waiting map[core.Priority]int
}

func newLimiter() *limiter {
	return &limiter{window: limitWindow, all: limitAll, unprivileged: limitUnprivileged, now: time.Now, waiting: make(map[core.Priority]int)}
}

// wait returns when a message may be sent, and counts it.
func (l *limiter) wait(ctx context.Context, privileged bool, p core.Priority) error {
	l.mu.Lock()
	l.waiting[p]++
	for {
		now := l.now()
		for len(l.sent) > 0 && now.Sub(l.sent[0]) >= l.window {
			l.sent = l.sent[1:]
		}
		room := len(l.sent) < l.all && (privileged || len(l.sent) < l.unprivileged)
		if room && !l.higherWaiting(p) {
			l.waiting[p]--
			l.sent = append(l.sent, now)
			l.mu.Unlock()
			return nil
		}
		d := 20 * time.Millisecond // a higher priority is waiting: check again soon
		if !room {
			d = max(l.sent[0].Add(l.window).Sub(now), 10*time.Millisecond)
		}
		l.mu.Unlock()

		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			l.mu.Lock()
			l.waiting[p]--
			l.mu.Unlock()
			return ctx.Err()
		case <-t.C:
		}
		l.mu.Lock()
	}
}

func (l *limiter) higherWaiting(p core.Priority) bool {
	for q, n := range l.waiting {
		if q > p && n > 0 {
			return true
		}
	}
	return false
}
