package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/config"
)

type roundTrip func(*http.Request) *http.Response

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func answer(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func TestLookupUserByID(t *testing.T) {
	refreshes := 0
	c := New(nil, &config.TwitchConfig{ClientID: "cid", ClientSecret: "secret"}, &http.Client{Transport: roundTrip(func(r *http.Request) *http.Response {
		if r.URL.String() == tokenURL {
			refreshes++
			return answer(200, `{"access_token": "fresh"}`)
		}
		if r.Header.Get("Authorization") != "Bearer fresh" {
			return answer(401, `{"message": "invalid token"}`)
		}
		switch r.URL.Query().Get("id") {
		case "1":
			return answer(200, `{"data": [{"id": "1", "login": "streamer", "display_name": "Streamer"}]}`)
		case "2":
			return answer(200, `{"data": []}`)
		case "bad":
			return answer(400, `{"message": "Invalid id"}`)
		}
		return answer(500, ``)
	})})
	ctx := context.Background()

	u, err := c.LookupUserByID(ctx, "1")
	if err != nil || u.Login != "streamer" || u.DisplayName != "Streamer" || refreshes != 1 {
		t.Fatalf("first lookup, with an expired app token: %+v %v (refreshes %d)", u, err, refreshes)
	}
	for _, id := range []string{"2", "bad"} {
		if _, err := c.LookupUserByID(ctx, id); !errors.Is(err, ErrNoUser) {
			t.Errorf("%s: %v", id, err)
		}
	}
	if _, err := c.LookupUserByID(ctx, "500"); err == nil || errors.Is(err, ErrNoUser) {
		t.Errorf("Twitch failing: %v", err)
	}
	if refreshes != 1 {
		t.Errorf("refreshed the app token %d times", refreshes)
	}
}
