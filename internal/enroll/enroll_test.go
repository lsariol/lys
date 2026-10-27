package enroll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/connections"
	"github.com/lsariol/lys/internal/core"
)

type stubStore struct {
	enrolled []connections.Enrollment
	disabled []string
	blocked  bool
	fail     bool
}

func (s *stubStore) Enroll(ctx context.Context, e connections.Enrollment) (connections.Connection, bool, error) {
	if s.fail {
		return connections.Connection{}, false, errors.New("database down")
	}
	s.enrolled = append(s.enrolled, e)
	return connections.Connection{ID: 5, Platform: e.Platform, ExternalID: e.ExternalID, Name: e.Name,
		Authorized: true, Enabled: true, AdminBlocked: s.blocked}, len(s.enrolled) == 1, nil
}

func (s *stubStore) Disable(ctx context.Context, platform, id string) error {
	if id == "missing" {
		return connections.ErrNotFound
	}
	s.disabled = append(s.disabled, platform+":"+id)
	return nil
}

// stubLookup knows channel 123; channel 500 fails as if the platform were down.
type stubLookup struct{}

func (stubLookup) Platform() string { return "twitch" }

func (stubLookup) LookupChannel(ctx context.Context, id string) (core.ChannelInfo, error) {
	switch id {
	case "123":
		return core.ChannelInfo{ID: "123", Name: "Streamer", Login: "streamer"}, nil
	case "500":
		return core.ChannelInfo{}, errors.New("Twitch: status 503")
	}
	return core.ChannelInfo{}, fmt.Errorf("%w: %s", core.ErrNoSuchChannel, id)
}

func serve(t *testing.T, a *API, method, path, auth, body string) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range a.Routes() {
		mux.Handle(pattern, h)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestEnroll(t *testing.T) {
	store := &stubStore{}
	a := New(store, "secret", stubLookup{})
	const body = `{"platform": "Twitch", "channel_id": "123", "name": "Ignored", "login": "ignored"}`

	for name, tc := range map[string]struct {
		auth, body string
		want       int
	}{
		"no token":       {"", body, http.StatusUnauthorized},
		"wrong token":    {"Bearer nope", body, http.StatusUnauthorized},
		"bad json":       {"Bearer secret", "{", http.StatusBadRequest},
		"bad platform":   {"Bearer secret", `{"platform": "twitch.tv", "channel_id": "123"}`, http.StatusBadRequest},
		"other platform": {"Bearer secret", `{"platform": "discord", "channel_id": "123"}`, http.StatusBadRequest},
		"no channel":     {"Bearer secret", `{"platform": "twitch"}`, http.StatusBadRequest},
		"unknown":        {"Bearer secret", `{"platform": "twitch", "channel_id": "<id>"}`, http.StatusBadRequest},
		"platform down":  {"Bearer secret", `{"platform": "twitch", "channel_id": "500"}`, http.StatusBadGateway},
	} {
		if got, _ := serve(t, a, http.MethodPost, "/channels", tc.auth, tc.body); got != tc.want {
			t.Errorf("%s: got %d, want %d", name, got, tc.want)
		}
	}
	if len(store.enrolled) != 0 {
		t.Fatalf("refused requests enrolled %v", store.enrolled)
	}

	code, out := serve(t, a, http.MethodPost, "/channels", "Bearer secret", body)
	if code != http.StatusOK || out["created"] != true || out["active"] != true || out["blocked"] != false ||
		out["connection_id"] != float64(5) || out["name"] != "Streamer" {
		t.Fatalf("enroll: %d %v", code, out)
	}
	if e := store.enrolled[0]; e.Platform != "twitch" || e.ExternalID != "123" || e.Name != "Streamer" || e.Login != "streamer" {
		t.Fatalf("stored the caller's names, not the platform's: %+v", e)
	}

	store.blocked = true
	if code, out := serve(t, a, http.MethodPost, "/channels", "Bearer secret", body); code != http.StatusOK || out["created"] != false || out["active"] != false || out["blocked"] != true {
		t.Fatalf("blocked channel: %d %v", code, out)
	}

	store.fail = true
	if code, _ := serve(t, a, http.MethodPost, "/channels", "Bearer secret", body); code != http.StatusInternalServerError {
		t.Fatalf("store failure: %d", code)
	}
}

func TestDisable(t *testing.T) {
	store := &stubStore{}
	a := New(store, "secret", stubLookup{})

	if code, _ := serve(t, a, http.MethodDelete, "/channels/twitch/123", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code, out := serve(t, a, http.MethodDelete, "/channels/twitch/123", "Bearer secret", ""); code != http.StatusOK || out["disabled"] != true || store.disabled[0] != "twitch:123" {
		t.Fatalf("disable: %d %v %v", code, out, store.disabled)
	}
	if code, _ := serve(t, a, http.MethodDelete, "/channels/twitch/missing", "Bearer secret", ""); code != http.StatusNotFound {
		t.Fatalf("missing: %d", code)
	}
}

func TestNoToken(t *testing.T) {
	if code, _ := serve(t, New(&stubStore{}, "", stubLookup{}), http.MethodPost, "/channels", "Bearer ", `{}`); code != http.StatusUnauthorized {
		t.Fatalf("an API without a token accepted a request: %d", code)
	}
}
