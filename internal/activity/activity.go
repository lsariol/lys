// Package activity remembers when each channel's latest chat messages arrived, to tell active channels from quiet ones.
package activity

import (
	"sync"
	"time"

	"github.com/lsariol/lys/internal/core"
)

// Tracker keeps each channel's latest message times in memory, so after a restart every channel starts out quiet.
type Tracker struct {
	mu    sync.Mutex
	keep  int
	now   func() time.Time
	times map[channel][]time.Time // oldest first
}

type channel struct{ platform, id string }

// New returns a tracker that remembers each channel's last keep messages.
func New(keep int) *Tracker {
	return &Tracker{keep: max(keep, 1), now: time.Now, times: make(map[channel][]time.Time)}
}

// Observe records that a message arrived; it implements router.Observer.
func (t *Tracker) Observe(m core.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := channel{m.Platform, m.Channel.ID}
	times := append(t.times[k], t.now())
	if len(times) > t.keep {
		times = times[len(times)-t.keep:]
	}
	t.times[k] = times
}

// Since is how many of the channel's messages arrived after after, up to the number it keeps.
func (t *Tracker) Since(platform, channelID string, after time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, at := range t.times[channel{platform, channelID}] {
		if at.After(after) {
			n++
		}
	}
	return n
}
