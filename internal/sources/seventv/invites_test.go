package seventv

import (
	"context"
	"testing"

	"github.com/lsariol/lys/internal/core"
	api "github.com/lsariol/lys/internal/features/seventv"
	"github.com/lsariol/lys/internal/features/seventv/seventvtest"
)

type directory []core.Channel

func (d directory) Channels(context.Context, string) ([]core.Channel, error) { return d, nil }

// 11: Lys accepts a pending invite from a channel it serves, and leaves one
// from anyone else pending.
func TestAcceptsServedChannels(t *testing.T) {
	f := seventvtest.New(t)
	f.Token = "session"
	f.Self = api.Account{ID: "lys", Login: "lyslink"}
	f.Invites = []api.Invite{
		{UserID: "served7tv", TwitchID: "100", Login: "served", State: api.Pending},
		{UserID: "stranger7tv", TwitchID: "666", Login: "stranger", State: api.Pending},
		{UserID: "nolink7tv", State: api.Pending},
		{UserID: "done7tv", TwitchID: "101", Login: "done", State: api.Accepted},
	}
	s := NewInvites(f.Client(), directory{{ID: "100", Name: "served"}, {ID: "101", Name: "done"}})
	s.check(context.Background())
	s.check(context.Background())

	want := map[string]string{"served7tv": api.Accepted, "stranger7tv": api.Pending, "nolink7tv": api.Pending, "done7tv": api.Accepted}
	for _, in := range f.Invites {
		if in.State != want[in.UserID] {
			t.Errorf("%s: %s, want %s", in.UserID, in.State, want[in.UserID])
		}
	}
	if f.Calls["AcceptInvite"] != 1 {
		t.Errorf("accepted %d times, want once", f.Calls["AcceptInvite"])
	}
	if !s.logged["stranger7tv"] || !s.logged["nolink7tv"] {
		t.Error("the invites left pending weren't logged")
	}
}
