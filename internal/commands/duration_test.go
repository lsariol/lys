package commands

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"10m":    10 * time.Minute,
		"1h30m":  90 * time.Minute,
		"2d":     48 * time.Hour,
		"1D2H":   26 * time.Hour,
		"45s":    45 * time.Second,
		"1h0m5s": time.Hour + 5*time.Second,
	} {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("%q: got %s, %v; want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "10", "m", "ten minutes", "10m x", "x10m", "1h-5m", "999999h"} {
		if got, err := ParseDuration(in); err == nil {
			t.Errorf("%q: got %s, want an error", in, got)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		90 * time.Minute:                "1 hour 30 minutes",
		26*time.Hour + time.Minute:      "1 day 2 hours 1 minute",
		10 * time.Second:                "less than a minute",
		2 * time.Hour:                   "2 hours",
		29*time.Minute + 50*time.Second: "30 minutes",
	} {
		if got := FormatDuration(d); got != want {
			t.Errorf("%s: got %q, want %q", d, got, want)
		}
	}
}
