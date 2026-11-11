// Package backoff spaces retries out: doubling waits up to a cap, with jitter so retries don't line up.
package backoff

import (
	"math/rand/v2"
	"time"
)

// Delay is the wait before retry attempt (1 first): between half and all of base doubled per attempt, at most max.
func Delay(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := max
	if attempt < 32 {
		if doubled := base << (attempt - 1); doubled > 0 && doubled < max {
			d = doubled
		}
	}
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(d-half)+1))
}
