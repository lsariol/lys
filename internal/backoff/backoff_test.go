package backoff

import (
	"testing"
	"time"
)

func TestDelay(t *testing.T) {
	for _, tc := range []struct {
		attempt  int
		min, max time.Duration
	}{
		{0, 500 * time.Millisecond, time.Second},
		{1, 500 * time.Millisecond, time.Second},
		{3, 2 * time.Second, 4 * time.Second},
		{10, 30 * time.Second, time.Minute},
		{100, 30 * time.Second, time.Minute},
	} {
		for range 50 {
			if d := Delay(tc.attempt, time.Second, time.Minute); d < tc.min || d > tc.max {
				t.Fatalf("attempt %d: %s, want %s to %s", tc.attempt, d, tc.min, tc.max)
			}
		}
	}
}
