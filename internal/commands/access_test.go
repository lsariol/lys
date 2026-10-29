package commands

import (
	"testing"

	"github.com/lsariol/lys/internal/core"
)

func TestAllowed(t *testing.T) {
	req := func(userID, channelID string, roles core.Roles) Request {
		var r Request
		r.User.ID, r.Channel.ID, r.User.Roles = userID, channelID, roles
		return r
	}
	for name, tc := range map[string]struct {
		req  Request
		want map[Access]bool
	}{
		"viewer":      {req("1", "9", 0), map[Access]bool{Everyone: true, VIP: false, Moderator: false, Owner: false}},
		"vip":         {req("1", "9", core.RoleVIP), map[Access]bool{VIP: true, Moderator: false, Owner: false}},
		"moderator":   {req("1", "9", core.RoleModerator), map[Access]bool{VIP: true, Moderator: true, Owner: false}},
		"broadcaster": {req("1", "9", core.RoleBroadcaster), map[Access]bool{Moderator: true, Owner: true}},
		"own channel": {req("9", "9", 0), map[Access]bool{Moderator: true, Owner: true}},
		"no IDs":      {req("", "", 0), map[Access]bool{Moderator: false, Owner: false}},
	} {
		for a, want := range tc.want {
			if got := tc.req.Allowed(a); got != want {
				t.Errorf("%s: Allowed(%s) = %v, want %v", name, a.Who(), got, want)
			}
		}
	}
}
