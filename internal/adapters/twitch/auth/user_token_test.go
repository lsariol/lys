package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/config"
)

func TestEnsureUserToken(t *testing.T) {
	calls := 0
	c := New(nil, &config.TwitchConfig{}, &http.Client{Transport: roundTrip(func(*http.Request) *http.Response {
		calls++
		return answer(503, `{"message":"down"}`)
	})})

	c.userExpires = time.Now().Add(time.Hour)
	if err := c.EnsureUserToken(context.Background()); err != nil || calls != 0 {
		t.Fatalf("a token good for an hour was refreshed: %d calls, %v", calls, err)
	}
	c.userExpires = time.Now().Add(5 * time.Minute)
	if err := c.EnsureUserToken(context.Background()); err == nil || calls != 1 {
		t.Fatalf("a token about to run out wasn't refreshed: %d calls, %v", calls, err)
	}
	c.userExpires = time.Time{}
	if err := c.EnsureUserToken(context.Background()); err == nil || calls != 2 {
		t.Fatalf("a token with no known expiry wasn't refreshed: %d calls", calls)
	}
}
