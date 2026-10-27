package letterboxd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
)

// stubStore keeps subscriptions in a map from user ID to Letterboxd username.
type stubStore struct {
	subs     map[string]string
	channels map[string]string
}

func (s *stubStore) AlreadyWatching(ctx context.Context, userID string) (bool, error) {
	_, ok := s.subs[userID]
	return ok, nil
}

func (s *stubStore) AddSubscription(ctx context.Context, username, userID, lbUsername, feedURL, alertChannel string) error {
	s.subs[userID] = lbUsername
	s.channels[userID] = alertChannel
	return nil
}

func (s *stubStore) RemoveSubscription(ctx context.Context, userID string) (string, bool, error) {
	lb, ok := s.subs[userID]
	delete(s.subs, userID)
	return lb, ok, nil
}

// letterboxdSite answers 404 for /missing/rss/ and 200 for everything else.
func letterboxdSite(t *testing.T) *http.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/missing/") {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	// Send letterboxd.com requests to the test server.
	return &http.Client{Transport: rewrite{srv.URL}}
}

type rewrite struct{ to string }

func (rw rewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	req := r.Clone(r.Context())
	u := *r.URL
	u.Scheme, u.Host = "http", strings.TrimPrefix(rw.to, "http://")
	req.URL = &u
	return http.DefaultTransport.RoundTrip(req)
}

func TestLetterboxd(t *testing.T) {
	store := &stubStore{subs: map[string]string{}, channels: map[string]string{}}
	l := New(store, letterboxdSite(t))
	run := func(args ...string) string {
		r, err := l.Run(context.Background(), commands.Request{
			Message: core.Message{Platform: "twitch", Channel: core.Channel{ID: "100"}, User: core.User{ID: "7", Name: "viewer"}},
			Args:    args,
		})
		if err != nil {
			t.Fatal(err)
		}
		return r.Text
	}

	if got := run("watch", "missing"); !strings.Contains(got, "couldn't find") {
		t.Errorf("missing user: %q", got)
	}
	if got := run("unwatch"); !strings.Contains(got, "don't have") {
		t.Errorf("unwatch before watching: %q", got)
	}
	if got := run("watch", "https://letterboxd.com/film_fan/"); !strings.Contains(got, "subscribed") || store.subs["7"] != "film_fan" || store.channels["7"] != "twitch:100" {
		t.Errorf("watch: %q, store %v %v", got, store.subs, store.channels)
	}
	if got := run("watch", "other"); !strings.Contains(got, "already") {
		t.Errorf("second watch: %q", got)
	}
	if got := run("unwatch"); !strings.Contains(got, "No longer watching film_fan") {
		t.Errorf("unwatch: %q", got)
	}
	if got := run(); !strings.Contains(got, "Usage") {
		t.Errorf("no arguments: %q", got)
	}
}
