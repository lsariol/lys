package chat

import (
	"context"
	"encoding/json"
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
	texts    []string // the messages sent, in order
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
	var sent struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(req.Body).Decode(&sent); err == nil {
		f.texts = append(f.texts, sent.Message)
	}
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

func TestDrainSplitsLongMessages(t *testing.T) {
	helix := &fakeHelix{}
	client := &http.Client{Transport: helix}
	cfg := &config.TwitchConfig{ClientID: "id", BotAccountID: "bot"}
	c := New(client, cfg, auth.New(nil, cfg, client), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	long := strings.Repeat("word ", 200) // 1000 characters
	if err := c.Send(ctx, core.Outbound{Channel: core.Channel{ID: "bot"}, Text: long}); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, core.Outbound{Channel: core.Channel{ID: "bot"}, Text: "next"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		helix.mu.Lock()
		texts := append([]string(nil), helix.texts...)
		helix.mu.Unlock()
		if len(texts) == 4 {
			if strings.Join(texts[:3], " ") != strings.TrimSpace(long) || texts[3] != "next" {
				t.Fatalf("sent %q", texts)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sent %d messages, want 4: %q", len(texts), texts)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
