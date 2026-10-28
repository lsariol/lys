package chat

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// statsEvery is how often the Send Chat Message rate limit is logged while sending.
const statsEvery = time.Minute

// helixStats sums up Helix's Ratelimit headers on Send Chat Message: its own
// bucket, separate from the chat limits the limiter keeps to.
type helixStats struct {
	mu            sync.Mutex
	since         time.Time
	sent, limited int
	limit, lowest int
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
		log.Printf("[Chat] sent %d in %s; Helix's bucket %d, lowest remaining %d, %d rate limited", s.sent, now.Sub(s.since).Round(time.Second), s.limit, s.lowest, s.limited)
		s.since, s.sent, s.limited, s.lowest = now, 0, 0, -1
	}
}
