package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var durationPart = regexp.MustCompile(`(\d+)([dhms])`)

// ParseDuration reads a duration like "10m", "1h30m" or "2d" (days, hours,
// minutes, seconds).
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, errors.New("empty duration")
	}
	matches := durationPart.FindAllStringSubmatchIndex(s, -1)
	if matches == nil {
		return 0, fmt.Errorf("%q isn't a duration like 10m or 1h30m", s)
	}

	var total time.Duration
	pos := 0
	for _, m := range matches {
		if m[0] != pos {
			return 0, fmt.Errorf("%q isn't a duration like 10m or 1h30m", s)
		}
		n, err := strconv.Atoi(s[m[2]:m[3]])
		if err != nil || n > 100000 {
			return 0, fmt.Errorf("%q is too long", s)
		}
		unit := map[string]time.Duration{"d": 24 * time.Hour, "h": time.Hour, "m": time.Minute, "s": time.Second}[s[m[4]:m[5]]]
		total += time.Duration(n) * unit
		pos = m[1]
	}
	if pos != len(s) {
		return 0, fmt.Errorf("%q isn't a duration like 10m or 1h30m", s)
	}
	return total, nil
}

// FormatDuration writes d as days, hours and minutes, e.g. "1 hour 30 minutes".
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	var parts []string
	for _, u := range []struct {
		size time.Duration
		name string
	}{{24 * time.Hour, "day"}, {time.Hour, "hour"}, {time.Minute, "minute"}} {
		if n := int(d / u.size); n > 0 {
			name := u.name
			if n != 1 {
				name += "s"
			}
			parts = append(parts, fmt.Sprintf("%d %s", n, name))
			d -= time.Duration(n) * u.size
		}
	}
	if len(parts) == 0 {
		return "less than a minute"
	}
	return strings.Join(parts, " ")
}
