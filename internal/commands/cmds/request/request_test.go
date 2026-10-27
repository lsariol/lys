package request

import (
	"context"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
)

type stubStore struct {
	saved   []FeatureRequest
	updates map[int64]string
}

func (s *stubStore) StoreRequest(ctx context.Context, fr FeatureRequest) (int64, error) {
	s.saved = append(s.saved, fr)
	return int64(len(s.saved)), nil
}

func (s *stubStore) UpdateStatus(ctx context.Context, status string, id int64) error {
	s.updates[id] = status
	return nil
}

func (s *stubStore) RecentRequests(ctx context.Context, limit int) ([]FeatureRequest, error) {
	return s.saved, nil
}

func (s *stubStore) NewRequests(ctx context.Context, limit int) ([]FeatureRequest, error) {
	return nil, nil
}

func TestRequest(t *testing.T) {
	store := &stubStore{updates: map[int64]string{}}
	r := New(store)
	run := func(user string, args ...string) string {
		reply, _ := r.Run(context.Background(), commands.Request{
			Message: core.Message{Platform: "twitch", Channel: core.Channel{ID: "1", Name: "chan"}, User: core.User{ID: "9", Name: user}},
			Args:    args,
		})
		return reply.Text
	}

	if got := run("viewer", "dark", "mode"); got != "Your request has been saved (ID 1)" {
		t.Errorf("create: %q", got)
	}
	if fr := store.saved[0]; fr.Body != "dark mode" || fr.Username != "viewer" || fr.ChannelName != "chan" || fr.Platform != "twitch" {
		t.Errorf("saved: %+v", fr)
	}
	if got := run("viewer", "get", "recent"); got != "" {
		t.Errorf("a viewer listed requests: %q", got)
	}
	if got := run("Mobasity", "get", "recent"); !strings.Contains(got, "[0] viewer: dark mode") {
		t.Errorf("owner list: %q", got)
	}
	if got := run("Mobasity", "get", "new"); got != "There are no pending requests to review." {
		t.Errorf("owner new: %q", got)
	}
	if got := run("mobasity", "set", "1", "completed"); !strings.Contains(got, "[1]") || store.updates[1] != "completed" {
		t.Errorf("set: %q, %v", got, store.updates)
	}
	if got := run("viewer"); !strings.Contains(got, "Usage") {
		t.Errorf("no arguments: %q", got)
	}
}
