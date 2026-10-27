package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/adapters/twitch/eventsub"
	"github.com/lsariol/lys/internal/core"
)

// ingestLoop turns each chat message into a core.Message for the router,
// skipping the bot's own messages so it never answers itself.
func (c *TwitchClient) ingestLoop(ctx context.Context, events <-chan eventsub.EventSubMessage, in chan<- core.Message) error {

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg := <-events:
			logMessage(msg)
			if msg.Payload.Event.ChatterUserID == c.Config.BotAccountID {
				continue
			}

			select {
			case in <- toMessage(msg):
			case <-ctx.Done():
				return ctx.Err()
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
