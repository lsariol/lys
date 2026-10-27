// Package chat sends the bot's messages through Helix, one queue per channel.
package chat

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/core"
)

// sendInterval spaces messages in one channel: Twitch allows a bot that isn't
// a moderator 20 messages per 30 seconds.
const sendInterval = 1500 * time.Millisecond

// queueSize is how many messages may wait per channel before new ones are dropped.
const queueSize = 32

type ChatClient struct {
	HTTP   *http.Client
	Auth   *auth.AuthClient
	Config *config.TwitchConfig

	mu     sync.Mutex
	queues map[string]chan core.Outbound // by broadcaster ID
}

func New(http *http.Client, cfg *config.TwitchConfig, auth *auth.AuthClient) *ChatClient {
	return &ChatClient{
		HTTP:   http,
		Auth:   auth,
		Config: cfg,
		queues: make(map[string]chan core.Outbound),
	}
}

// Send queues a message for its channel. The queue's sender runs until ctx is cancelled.
func (c *ChatClient) Send(ctx context.Context, out core.Outbound) error {
	select {
	case c.queue(ctx, out.Channel.ID) <- out:
		return nil
	default:
		return fmt.Errorf("Twitch: the send queue for %q is full; dropped a message", out.Channel.ID)
	}
}

func (c *ChatClient) queue(ctx context.Context, channelID string) chan core.Outbound {
	c.mu.Lock()
	defer c.mu.Unlock()
	q, ok := c.queues[channelID]
	if !ok {
		q = make(chan core.Outbound, queueSize)
		c.queues[channelID] = q
		go c.drain(ctx, q)
	}
	return q
}

// drain sends one channel's messages in order, sendInterval apart.
func (c *ChatClient) drain(ctx context.Context, q <-chan core.Outbound) {
	for {
		select {
		case <-ctx.Done():
			return
		case out := <-q:
			if err := c.deliver(ctx, out, true); err != nil {
				log.Printf("[Chat] %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sendInterval):
		}
	}
}

// deliver sends out; on a 401 it refreshes the app token and, if retry is set, tries once more.
func (c *ChatClient) deliver(ctx context.Context, out core.Outbound, retry bool) error {
	sent, helixErr, err := c.SendChat(ctx, out.Channel.ID, out.Text)
	if err != nil {
		return fmt.Errorf("Twitch: send to %q: %w", out.Channel.ID, err)
	}

	if helixErr == nil {
		for _, d := range sent.Data {
			if !d.IsSent && d.DropReason != nil {
				return fmt.Errorf("Twitch: message to %q dropped: %s (%s)", out.Channel.ID, d.DropReason.Message, d.DropReason.Code)
			}
		}
		return nil
	}

	switch helixErr.Status {
	case http.StatusUnauthorized:
		if !retry {
			return fmt.Errorf("Twitch: send to %q: 401 with a fresh app token: %s (check the bot's user:write:chat and user:bot scopes and the channel's channel:bot grant)", out.Channel.ID, helixErr.Message)
		}
		if err := c.Auth.RefreshAppAccessToken(ctx); err != nil {
			return fmt.Errorf("refresh app access token: %w", err)
		}
		return c.deliver(ctx, out, false)
	case http.StatusForbidden:
		return fmt.Errorf("Twitch: send to %q: 403, the bot may not chat there (banned or timed out): %s", out.Channel.ID, helixErr.Message)
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("Twitch: send to %q: 422, the message is too long: %s", out.Channel.ID, helixErr.Message)
	default:
		return fmt.Errorf("Twitch: send to %q: %d: %s", out.Channel.ID, helixErr.Status, helixErr.Message)
	}
}
