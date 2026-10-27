package eventsub

import (
	"context"
	"sync"
	"testing"

	"github.com/lsariol/lys/internal/core"
)

// fakeChannels records revocations.
type fakeChannels struct {
	mu      sync.Mutex
	revoked map[string]string
}

func (f *fakeChannels) Channels(ctx context.Context, platform string) ([]core.Channel, error) {
	return nil, nil
}

func (f *fakeChannels) Revoke(ctx context.Context, platform, channelID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked[channelID] = reason
	return nil
}

func revocation(status, channel string) EventSubMessage {
	var m EventSubMessage
	m.Metadata.MessageType = "revocation"
	m.Payload.Subscription.Status = status
	m.Payload.Subscription.Type = "channel.chat.message"
	m.Payload.Subscription.Condition.BroadcasterUserId = channel
	return m
}

func TestConsumeEvent(t *testing.T) {
	channels := &fakeChannels{revoked: map[string]string{}}
	c := New(nil, nil, nil, channels)
	c.Channels["1"] = Channel{UserID: "1", Username: "Banned"}
	c.connected.Store(true)
	ctx := context.Background()

	c.ConsumeEvent(ctx, revocation("chat_user_banned", "1"))
	c.ConsumeEvent(ctx, revocation("user_removed", "2"))
	c.ConsumeEvent(ctx, revocation("version_removed", "3"))
	if len(channels.revoked) != 2 || channels.revoked["1"] == "" || channels.revoked["2"] == "" {
		t.Fatalf("revoked: %v", channels.revoked)
	}
	if !c.Connected() {
		t.Fatal("a channel's revocation took the whole connection down")
	}

	// The bot's own authorization going is every channel at once.
	c.ConsumeEvent(ctx, revocation("authorization_revoked", "4"))
	if c.Connected() {
		t.Fatal("still healthy after the bot's authorization was revoked")
	}
	if _, ok := channels.revoked["4"]; ok {
		t.Fatal("a bot-wide revocation was recorded against one channel")
	}

	var chat EventSubMessage
	chat.Metadata.MessageType = "notification"
	chat.Payload.Subscription.Type = "channel.chat.message"
	c.ConsumeEvent(ctx, chat)
	if len(c.out) != 1 {
		t.Fatal("a chat message wasn't forwarded")
	}

	other := chat
	other.Payload.Subscription.Type = "channel.follow"
	c.ConsumeEvent(ctx, other)
	c.ConsumeEvent(ctx, other)
	var keepalive EventSubMessage
	keepalive.Metadata.MessageType = "session_keepalive"
	c.ConsumeEvent(ctx, keepalive)
	var unknown EventSubMessage
	unknown.Metadata.MessageType = "something_new"
	c.ConsumeEvent(ctx, unknown)
	if len(c.out) != 1 {
		t.Fatal("something other than chat was forwarded")
	}
}
