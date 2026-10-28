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

// RecentRequests names requesters by their Lys user, as the database does.
func (s *stubStore) RecentRequests(ctx context.Context, limit int) ([]FeatureRequest, error) {
	out := make([]FeatureRequest, len(s.saved))
	for i, fr := range s.saved {
		fr.Username = map[int64]string{12: "viewer"}[fr.PlatformUserID]
		out[i] = fr
	}
	return out, nil
}

func (s *stubStore) NewRequests(ctx context.Context, limit int) ([]FeatureRequest, error) {
	return nil, nil
}

func TestRequest(t *testing.T) {
	store := &stubStore{updates: map[int64]string{}}
	r := New(store)
	// "Mobasity" with the owner's ID is the owner; the same name with another ID isn't.
	identity := core.Identity{PlatformUserID: 12}
	run := func(user string, args ...string) string {
		id := "9"
		if user == "Mobasity" {
			id = "42217464"
		}
		reply, _ := r.Run(context.Background(), commands.Request{
			Message:    core.Message{Platform: "twitch", Channel: core.Channel{ID: "1", Name: "chan"}, User: core.User{ID: id, Name: user}},
			Identity:   identity,
			Connection: core.Connection{ID: 3},
			Args:       args,
		})
		return reply.Text
	}

	if got := run("viewer", "dark", "mode"); got != "Your request has been saved (ID 1)" {
		t.Errorf("create: %q", got)
	}
	if fr := store.saved[0]; fr.Body != "dark mode" || fr.Platform != "twitch" || fr.PlatformUserID != 12 || fr.ConnectionID != 3 {
		t.Errorf("saved: %+v", fr)
	}
	identity = core.Identity{}
	if got := run("viewer", "no", "identity"); strings.Contains(got, "saved") || len(store.saved) != 1 {
		t.Errorf("saved without a Lys user: %q", got)
	}
	identity = core.Identity{PlatformUserID: 12}
	if got := run("viewer", "get", "recent"); got != "" {
		t.Errorf("a viewer listed requests: %q", got)
	}
	if got := run("Mobasity", "get", "recent"); !strings.Contains(got, "[0] viewer: dark mode") {
		t.Errorf("owner list: %q", got)
	}
	if got := run("Mobasity", "get", "new"); got != "There are no pending requests to review." {
		t.Errorf("owner new: %q", got)
	}
	if got := run("mobasity", "set", "1", "rejected"); got != "" || store.updates[1] != "" {
		t.Errorf("the owner's name without the owner's ID changed a request: %q", got)
	}
	if got := run("Mobasity", "set", "1", "completed"); !strings.Contains(got, "[1]") || store.updates[1] != "completed" {
		t.Errorf("set: %q, %v", got, store.updates)
	}
	if got := run("viewer"); !strings.Contains(got, "Usage") {
		t.Errorf("no arguments: %q", got)
	}
}
