package chat

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/core"
)

// fakeHelix answers token requests, and Send Chat Message with the next
// scripted status (200 once the script runs out).
type fakeHelix struct {
	mu       sync.Mutex
	statuses []int
	sends    int
	tokens   int
}

func (f *fakeHelix) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(status int, body string, header http.Header) (*http.Response, error) {
		if header == nil {
			header = make(http.Header)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: header}, nil
	}
	if strings.HasSuffix(req.URL.Path, "/oauth2/token") {
		f.tokens++
		return reply(http.StatusOK, `{"access_token": "app-token"}`, nil)
	}

	f.sends++
	status := http.StatusOK
	if len(f.statuses) > 0 {
		status, f.statuses = f.statuses[0], f.statuses[1:]
	}
	switch status {
	case http.StatusOK:
		return reply(status, `{"data": [{"message_id": "m", "is_sent": true}]}`, nil)
	case http.StatusTooManyRequests:
		h := make(http.Header)
		h.Set("Ratelimit-Reset", strconv.FormatInt(time.Now().Unix()+1, 10))
		return reply(status, `{"status": 429, "message": "slow down"}`, h)
	default:
		return reply(status, `{"status": `+strconv.Itoa(status)+`, "message": "no"}`, nil)
	}
}

func TestDeliver(t *testing.T) {
	for name, tc := range map[string]struct {
		statuses  []int
		wantPause bool
		wantErr   bool
		wantSends int
	}{
		"sent":                {nil, false, false, 1},
		"banned":              {[]int{403}, true, true, 1},
		"fresh token fixes":   {[]int{401}, false, false, 2},
		"fresh token doesn't": {[]int{401, 401}, true, true, 2},
		"rate limited":        {[]int{429}, false, false, 2},
		"too long":            {[]int{422}, false, true, 1},
	} {
		t.Run(name, func(t *testing.T) {
			helix := &fakeHelix{statuses: tc.statuses}
			client := &http.Client{Transport: helix}
			cfg := &config.TwitchConfig{ClientID: "id", BotAccountID: "bot"}
			c := New(client, cfg, auth.New(nil, cfg, client), nil)

			pause, err := c.deliver(context.Background(), core.Outbound{Channel: core.Channel{ID: "1"}, Text: "hi"}, true)
			if (pause > 0) != tc.wantPause || (err != nil) != tc.wantErr || helix.sends != tc.wantSends {
				t.Fatalf("pause %s, err %v, sends %d", pause, err, helix.sends)
			}
		})
	}
}
