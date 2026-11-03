package bot

import (
	"context"
	"time"

	"github.com/lsariol/lys/internal/adapters/twitch/eventsub"
	"github.com/lsariol/lys/internal/core"
)

// dropReportEvery is how often messages dropped for a full router are logged.
const dropReportEvery = time.Minute

// ingestLoop turns each chat message into a core.Message for the router,
// skipping the bot's own messages so it never answers itself. It never waits
// on the router: a message the router has no room for is dropped and counted,
// so the EventSub socket keeps being read and Twitch's pings answered.
func (c *TwitchClient) ingestLoop(ctx context.Context, events <-chan eventsub.EventSubMessage, in chan<- core.Message) error {
	report := time.NewTicker(dropReportEvery)
	defer report.Stop()
	dropped := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-report.C:
			if dropped > 0 {
				c.log.Warn("router full: dropped chat messages", "dropped", dropped, "period_s", int(dropReportEvery.Seconds()))
				dropped = 0
			}

		case msg := <-events:
			if c.Config.LogChat {
				c.logMessage(msg)
			}
			if ev := msg.Payload.Event; ev.ChatterUserID == c.Config.BotAccountID {
				c.Chat.SetRoles(ev.BroadcasterUserID, roles(ev.Badges)) // its badges say whether it may send faster
				continue
			}

			select {
			case in <- toMessage(msg):
			default:
				dropped++
			}
		}
	}
}

func toMessage(msg eventsub.EventSubMessage) core.Message {
	ev := msg.Payload.Event

	sent, err := time.Parse(time.RFC3339Nano, msg.Metadata.MessageTimestamp)
	if err != nil {
		sent = time.Now()
	}

	return core.Message{
		Platform: Platform,
		ID:       ev.MessageID,
		Channel:  core.Channel{ID: ev.BroadcasterUserID, Name: ev.BroadcasterUserName},
		User:     core.User{ID: ev.ChatterUserID, Name: ev.ChatterUserName, Roles: roles(ev.Badges)},
		Text:     ev.Message.Text,
		Time:     sent,
	}
}

// roles reads the sender's roles from their chat badges.
func roles(badges []eventsub.ChatBadge) core.Roles {
	var r core.Roles
	for _, b := range badges {
		switch b.SetID {
		case "broadcaster":
			r |= core.RoleBroadcaster
		case "moderator":
			r |= core.RoleModerator
		case "vip":
			r |= core.RoleVIP
		case "subscriber", "founder":
			r |= core.RoleSubscriber
		}
	}
	return r
}

// logMessage logs a chat message; only with LOG_CHAT=true, since chat is
// most of what Lys would log otherwise.
func (c *TwitchClient) logMessage(e eventsub.EventSubMessage) {
	ev := e.Payload.Event
	c.log.Info("chat message", "channel", ev.BroadcasterUserName, "user", ev.ChatterUserName, "text", ev.Message.Text)
}
