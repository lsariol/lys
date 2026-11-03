package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/adapters/twitch/eventsub"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/logtest"
)

// A chat message is logged only with LOG_CHAT=true, and never the bot's secrets.
func TestLogChat(t *testing.T) {
	for _, setting := range []string{"", "false", "true", "TRUE"} {
		t.Setenv("LOG_CHAT", setting)
		cfg := config.NewTwitchConfig()
		cfg.BotAccountID, cfg.ClientSecret = "bot", "client-secret-value"
		log, buf := logtest.New()
		c := &TwitchClient{Config: cfg, log: log}

		events, in := make(chan eventsub.EventSubMessage, 1), make(chan core.Message, 1)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error)
		go func() { done <- c.ingestLoop(ctx, events, in) }()
		var msg eventsub.EventSubMessage
		ev := &msg.Payload.Event
		ev.BroadcasterUserID, ev.BroadcasterUserName = "1", "Streamer"
		ev.ChatterUserID, ev.ChatterUserName = "2", "Viewer"
		ev.Message.Text = "hello chat"
		events <- msg
		<-in
		cancel()
		<-done

		on := strings.EqualFold(setting, "true")
		recs := buf.Records(t)
		switch {
		case !on && len(recs) != 0:
			t.Errorf("LOG_CHAT=%q logged %v", setting, recs)
		case on && (len(recs) != 1 || recs[0]["msg"] != "chat message" || recs[0]["level"] != "INFO" ||
			recs[0]["channel"] != "Streamer" || recs[0]["user"] != "Viewer" || recs[0]["text"] != "hello chat"):
			t.Errorf("LOG_CHAT=%q: %v, want the message", setting, recs)
		}
		if strings.Contains(buf.String(), "client-secret-value") {
			t.Errorf("the client secret was logged: %s", buf)
		}
	}
}

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
