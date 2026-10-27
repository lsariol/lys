package bot

import (
	"testing"

	"github.com/lsariol/lys/internal/adapters/twitch/eventsub"
	"github.com/lsariol/lys/internal/core"
)

func TestToMessage(t *testing.T) {
	var msg eventsub.EventSubMessage
	msg.Metadata.MessageTimestamp = "2026-10-26T20:00:00.123456789Z"
	ev := &msg.Payload.Event
	ev.BroadcasterUserID, ev.BroadcasterUserName = "1", "Streamer"
	ev.ChatterUserID, ev.ChatterUserName = "2", "Viewer"
	ev.MessageID = "m1"
	ev.Message.Text = "!ping"
	ev.Badges = []eventsub.ChatBadge{{SetID: "moderator"}, {SetID: "founder"}, {SetID: "premium"}}

	m := toMessage(msg)
	if m.Platform != "twitch" || m.ID != "m1" || m.Text != "!ping" ||
		m.Channel != (core.Channel{ID: "1", Name: "Streamer"}) ||
		m.User.ID != "2" || m.User.Name != "Viewer" {
		t.Fatalf("got %+v", m)
	}
	if !m.User.Roles.Has(core.RoleModerator|core.RoleSubscriber) || m.User.Roles.Has(core.RoleBroadcaster) {
		t.Fatalf("roles: got %b", m.User.Roles)
	}
	if m.Time.Year() != 2026 {
		t.Fatalf("time: got %v", m.Time)
	}
}
