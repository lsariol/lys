package activity

import (
	"testing"
	"time"

	"github.com/lsariol/lys/internal/core"
)

func TestTracker(t *testing.T) {
	start := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	now := start
	tr := New(3)
	tr.now = func() time.Time { return now }
	say := func(channelID string) {
		now = now.Add(time.Minute)
		tr.Observe(core.Message{Platform: "twitch", Channel: core.Channel{ID: channelID}})
	}

	for range 5 {
		say("1")
	}
	say("2")

	if got := tr.Since("twitch", "1", start); got != 3 {
		t.Errorf("keeps only the last 3: got %d", got)
	}
	if got := tr.Since("twitch", "1", start.Add(4*time.Minute)); got != 1 {
		t.Errorf("since minute 4: got %d", got)
	}
	if got := tr.Since("twitch", "2", start); got != 1 {
		t.Errorf("other channel: got %d", got)
	}
	if got := tr.Since("discord", "1", start); got != 0 {
		t.Errorf("other platform: got %d", got)
	}
}
