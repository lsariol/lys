package chat

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// statsEvery is how often the Send Chat Message rate limit is logged while sending.
const statsEvery = time.Minute

// helixStats sums Send Chat Message's Ratelimit headers, a bucket separate from the chat limits.
type helixStats struct {
	mu            sync.Mutex
	since         time.Time
	sent, limited int
	limit, lowest int
	log           *slog.Logger
}

func (s *helixStats) record(h http.Header, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.since.IsZero() {
		s.since, s.lowest = now, -1
	}
	s.sent++
	if status == http.StatusTooManyRequests {
		s.limited++
	}
	if n, err := strconv.Atoi(h.Get("Ratelimit-Limit")); err == nil {
		s.limit = n
	}
	if n, err := strconv.Atoi(h.Get("Ratelimit-Remaining")); err == nil && (s.lowest < 0 || n < s.lowest) {
		s.lowest = n
	}
	if now.Sub(s.since) >= statsEvery {
		s.log.Info("Helix send stats", "sent", s.sent, "period_s", int(now.Sub(s.since).Seconds()),
			"bucket", s.limit, "lowest_remaining", s.lowest, "rate_limited", s.limited)
		s.since, s.sent, s.limited, s.lowest = now, 0, 0, -1
	}
}
