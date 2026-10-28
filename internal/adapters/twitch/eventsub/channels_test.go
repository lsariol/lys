package eventsub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/core"
)

func TestDiffChannels(t *testing.T) {
	joined := map[string]Channel{"1": {UserID: "1"}, "2": {UserID: "2"}}
	join, leave := diffChannels(joined, []core.Channel{{ID: "2"}, {ID: "3", Name: "New"}})
	sort.Strings(leave)
	if len(join) != 1 || join[0].ID != "3" || join[0].Name != "New" || len(leave) != 1 || leave[0] != "1" {
		t.Fatalf("join %v, leave %v", join, leave)
	}
	if join, leave := diffChannels(joined, []core.Channel{{ID: "1"}, {ID: "2"}}); len(join)+len(leave) != 0 {
		t.Fatalf("no change: join %v, leave %v", join, leave)
	}
}

func TestSubscribeError(t *testing.T) {
	resp := func(status int, body string) *http.Response {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
	}
	for _, tc := range []struct {
		resp *http.Response
		want string
	}{
		{resp(400, `{"error":"Bad Request","status":400,"message":"invalid broadcaster_user_id"}`), "status 400: invalid broadcaster_user_id (check the channel's Twitch ID"},
		{resp(403, `{"message":"subscription missing proper authorization"}`), "status 403: subscription missing proper authorization (the bot needs user:read:chat"},
		{resp(500, ``), "status 500: unreadable answer"},
		{resp(503, `{}`), "status 503: Twitch gave no reason"},
	} {
		if got := subscribeError(tc.resp).Error(); !strings.Contains(got, tc.want) {
			t.Errorf("got %q, want it to contain %q", got, tc.want)
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestJoinInParallel(t *testing.T) {
	var mu sync.Mutex
	inFlight, most := 0, 0
	c := New(&http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		var body struct {
			Condition map[string]string `json:"condition"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		mu.Lock()
		inFlight++
		most = max(most, inFlight)
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		id := body.Condition["broadcaster_user_id"]
		if id == "refused" {
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"message":"no"}`))}, nil
		}
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"sub-` + id + `"}]}`))}, nil
	})}, &config.TwitchConfig{BotAccountID: "bot"}, auth.New(nil, &config.TwitchConfig{}, nil), nil)

	var channels []core.Channel
	for i := range 40 {
		channels = append(channels, core.Channel{ID: strconv.Itoa(i), Name: "c" + strconv.Itoa(i)})
	}
	channels = append(channels, core.Channel{ID: "refused", Name: "Refused"})

	start := time.Now()
	c.join(context.Background(), channels)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("41 joins at 50 ms each took %s", d)
	}
	if most > joinAtOnce {
		t.Fatalf("%d joins at once, want at most %d", most, joinAtOnce)
	}
	if len(c.Channels) != 40 || c.Channels["7"].SubscriptionID != "sub-7" || c.Channels["7"].Username != "c7" {
		t.Fatalf("recorded %d channels; 7 is %+v", len(c.Channels), c.Channels["7"])
	}
	if _, ok := c.Channels["refused"]; ok {
		t.Fatal("a refused join was recorded")
	}
}

func TestSubscribeUnauthorized(t *testing.T) {
	err := subscribeError(&http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"message":"Invalid OAuth token"}`))})
	if !errors.Is(err, errUnauthorized) || !strings.Contains(err.Error(), "Invalid OAuth token") {
		t.Fatalf("got %v", err)
	}
}
