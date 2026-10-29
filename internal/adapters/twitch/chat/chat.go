// Package chat sends the bot's messages through Helix: one queue per channel,
// paced for the bot's role there, under one limit for the whole account.
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

// queueSize is how many messages may wait per channel before new ones are dropped.
const queueSize = 32

// Roles keeps, across restarts, whether the bot is a moderator, VIP or
// broadcaster in each channel.
type Roles interface {
	BotRoles(ctx context.Context) (map[string]bool, error)
	SetBotRole(ctx context.Context, channelID string, privileged bool) error
}

type ChatClient struct {
	HTTP   *http.Client
	Auth   *auth.AuthClient
	Config *config.TwitchConfig

	limit *limiter
	roles Roles
	stats helixStats

	mu         sync.Mutex
	queues     map[string]chan core.Outbound // by broadcaster ID
	privileged map[string]bool               // by broadcaster ID
}

func New(http *http.Client, cfg *config.TwitchConfig, auth *auth.AuthClient, roles Roles) *ChatClient {
	return &ChatClient{
		HTTP:       http,
		Auth:       auth,
		Config:     cfg,
		limit:      newLimiter(),
		roles:      roles,
		queues:     make(map[string]chan core.Outbound),
		privileged: make(map[string]bool),
	}
}

// LoadRoles reads the bot's last known role in each channel.
func (c *ChatClient) LoadRoles(ctx context.Context) error {
	if c.roles == nil {
		return nil
	}
	roles, err := c.roles.BotRoles(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	for id, privileged := range roles {
		c.privileged[id] = privileged
	}
	c.mu.Unlock()
	return nil
}

// SetRoles records the bot's roles in a channel, as its own messages there show them.
func (c *ChatClient) SetRoles(channelID string, roles core.Roles) {
	privileged := roles&(core.RoleBroadcaster|core.RoleModerator|core.RoleVIP) != 0
	c.mu.Lock()
	was, known := c.privileged[channelID]
	c.privileged[channelID] = privileged
	c.mu.Unlock()
	if known && was == privileged {
		return
	}
	if privileged {
		log.Printf("[Chat] the bot is a moderator, VIP or broadcaster in %q: sending faster there", channelID)
	} else if known {
		log.Printf("[Chat] the bot is no longer a moderator, VIP or broadcaster in %q", channelID)
	}
	if c.roles == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.roles.SetBotRole(ctx, channelID, privileged); err != nil {
			log.Printf("[Chat] %v", err)
		}
	}()
}

// isPrivileged reports whether the bot may send faster in the channel: its
// own channel, or one it was last seen moderating there (or as a VIP).
func (c *ChatClient) isPrivileged(channelID string) bool {
	if channelID == c.Config.BotAccountID {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.privileged[channelID]
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

// pauseFor is how long a channel's messages are dropped after Twitch refuses
// the bot there (403, or a 401 a fresh token doesn't fix), instead of failing
// on every message.
const pauseFor = 10 * time.Minute

// maxRateLimitWait caps how long a 429 waits for Twitch's reset time.
const maxRateLimitWait = time.Minute

// drain sends one channel's messages in order, paced for the bot's role there
// and within the account's limit. A message too long for Twitch goes out in
// parts, one after another.
func (c *ChatClient) drain(ctx context.Context, q <-chan core.Outbound) {
	var pausedUntil time.Time
	dropped := 0
	for {
		var out core.Outbound
		select {
		case <-ctx.Done():
			return
		case out = <-q:
		}

		for _, part := range split(out.Text) {
			if time.Now().Before(pausedUntil) {
				dropped++
				break
			}
			if dropped > 0 {
				log.Printf("[Chat] %q: dropped %d messages while paused", out.Channel.ID, dropped)
				dropped = 0
			}

			privileged := c.isPrivileged(out.Channel.ID)
			if err := c.limit.wait(ctx, privileged, out.Priority); err != nil {
				return
			}
			msg := out
			msg.Text = part
			pause, err := c.deliver(ctx, msg, true)
			if err != nil {
				log.Printf("[Chat] %v", err)
			}
			if pause > 0 {
				pausedUntil = time.Now().Add(pause)
				log.Printf("! [Chat] pausing %q for %s", out.Channel.ID, pause)
			}

			gap := unprivilegedGap
			if privileged {
				gap = privilegedGap
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(gap):
			}
		}
	}
}

// deliver sends out. On a 401 it refreshes the app token and, if retry is
// set, tries once more; on a 429 it waits for Twitch's reset and tries once
// more. pause is how long to stop sending to the channel.
func (c *ChatClient) deliver(ctx context.Context, out core.Outbound, retry bool) (pause time.Duration, err error) {
	sent, helixErr, err := c.SendChat(ctx, out.Channel.ID, out.Text)
	if err != nil {
		return 0, fmt.Errorf("Twitch: send to %q: %w", out.Channel.ID, err)
	}

	if helixErr == nil {
		for _, d := range sent.Data {
			if !d.IsSent && d.DropReason != nil {
				return 0, fmt.Errorf("Twitch: message to %q dropped: %s (%s)", out.Channel.ID, d.DropReason.Message, d.DropReason.Code)
			}
		}
		return 0, nil
	}

	switch helixErr.Status {
	case http.StatusUnauthorized:
		if !retry {
			return pauseFor, fmt.Errorf("Twitch: send to %q: 401 with a fresh app token: %s (check the bot's user:write:chat and user:bot scopes, and that the channel granted channel:bot or made the bot a moderator)", out.Channel.ID, helixErr.Message)
		}
		if err := c.Auth.RefreshAppAccessToken(ctx); err != nil {
			return 0, fmt.Errorf("refresh app access token: %w", err)
		}
		return c.deliver(ctx, out, false)
	case http.StatusForbidden:
		return pauseFor, fmt.Errorf("Twitch: send to %q: 403, the bot may not chat there (banned or timed out): %s", out.Channel.ID, helixErr.Message)
	case http.StatusTooManyRequests:
		if !retry {
			return 0, fmt.Errorf("Twitch: send to %q: rate limited again after waiting", out.Channel.ID)
		}
		wait := min(max(helixErr.RetryAfter, time.Second), maxRateLimitWait)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(wait):
		}
		return c.deliver(ctx, out, false)
	case http.StatusUnprocessableEntity:
		return 0, fmt.Errorf("Twitch: send to %q: 422, the message is too long: %s", out.Channel.ID, helixErr.Message)
	default:
		return 0, fmt.Errorf("Twitch: send to %q: %d: %s", out.Channel.ID, helixErr.Status, helixErr.Message)
	}
}
