package bot

import (
	"context"
	"fmt"
	"log"
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
				log.Printf("[Twitch] the router had no room: dropped %d chat messages in the last minute", dropped)
				dropped = 0
			}

		case msg := <-events:
			logMessage(msg)
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

func logMessage(e eventsub.EventSubMessage) {
	fmt.Printf("[%s] %-10.10s: @%-10.10s - %s\n",
		time.Now().Format("2006-01-02 15:04:05.00"),
		e.Payload.Event.BroadcasterUserName,
		e.Payload.Event.ChatterUserName,
		e.Payload.Event.Message.Text,
	)
}
