package inlet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/core"
)

type recorder chan core.Outbound

func (r recorder) Publish(ctx context.Context, out core.Outbound) error {
	r <- out
	return nil
}

func post(h http.Handler, auth, body string) int {
	req := httptest.NewRequest(http.MethodPost, "/notifications", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

func TestInlet(t *testing.T) {
	in := New("secret")
	pub := make(recorder, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go in.Run(ctx, pub)

	const body = `{"message":"new review","alert_channels":["twitch:123","twitch:456:Name","bad"],"guid":"g1"}`

	for name, tc := range map[string]struct {
		auth string
		body string
		want int
	}{
		"no token":    {"", body, http.StatusUnauthorized},
		"wrong token": {"Bearer nope", body, http.StatusUnauthorized},
		"not bearer":  {"secret", body, http.StatusUnauthorized},
		"bad json":    {"Bearer secret", "{", http.StatusBadRequest},
		"no message":  {"Bearer secret", `{"alert_channels":["twitch:1"]}`, http.StatusBadRequest},
		"no channels": {"Bearer secret", `{"message":"x"}`, http.StatusBadRequest},
	} {
		if got := post(in, tc.auth, tc.body); got != tc.want {
			t.Errorf("%s: got %d, want %d", name, got, tc.want)
		}
	}

	if got := post(in, "Bearer secret", body); got != http.StatusOK {
		t.Fatalf("valid request: got %d", got)
	}
	want := []core.Outbound{
		{Platform: "twitch", Channel: core.Channel{ID: "123"}, Text: "new review"},
		{Platform: "twitch", Channel: core.Channel{ID: "456", Name: "Name"}, Text: "new review"},
	}
	for i, w := range want {
		select {
		case got := <-pub:
			if got != w {
				t.Fatalf("message %d: got %+v, want %+v", i, got, w)
			}
		case <-time.After(time.Second):
			t.Fatalf("message %d wasn't published", i)
		}
	}
}

func TestInletWithoutToken(t *testing.T) {
	if got := post(New(""), "Bearer ", `{"message":"x","alert_channels":["twitch:1"]}`); got != http.StatusUnauthorized {
		t.Fatalf("an inlet without a token accepted a request: %d", got)
	}
}
