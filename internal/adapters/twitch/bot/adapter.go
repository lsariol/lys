// Package bot is the Twitch adapter: it forwards chat messages to the router
// and sends what the router gives it.
package bot

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/lsariol/coveclient"
	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	"github.com/lsariol/lys/internal/adapters/twitch/chat"
	"github.com/lsariol/lys/internal/adapters/twitch/eventsub"
	"github.com/lsariol/lys/internal/botaccounts"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/core"
)

// Platform is the name Twitch messages carry.
const Platform = "twitch"

// BotAccounts records the bot account once Twitch has confirmed its token.
type BotAccounts interface {
	SaveDefault(ctx context.Context, a botaccounts.Account) error
}

type TwitchClient struct {
	Chat     *chat.ChatClient
	EventSub *eventsub.EventSubClient
	Auth     *auth.AuthClient
	Config   *config.TwitchConfig
	Bots     BotAccounts
}

// New returns the Twitch adapter. It joins the channels that channels lists for
// "twitch", and revokes the ones Twitch says it may no longer be in.
func New(http *http.Client, cfg *config.TwitchConfig, cove *coveclient.Client, channels eventsub.Channels, bots BotAccounts) *TwitchClient {

	auth := auth.New(cove, cfg, http)

	return &TwitchClient{
		Chat:     chat.New(http, cfg, auth),
		EventSub: eventsub.New(http, cfg, auth, channels),
		Auth:     auth,
		Config:   cfg,
		Bots:     bots,
	}
}

func (c *TwitchClient) Platform() string { return Platform }

// Healthy reports whether the EventSub connection is up.
func (c *TwitchClient) Healthy() bool { return c.EventSub.Connected() }

// Run connects to Twitch and forwards chat messages into in until ctx is cancelled.
func (c *TwitchClient) Run(ctx context.Context, in chan<- core.Message) error {

	if err := c.Initilize(ctx); err != nil {
		return fmt.Errorf("initilization error: %w", err)
	}
	defer c.Shutdown(ctx)

	if err := c.EventSub.Run(ctx); err != nil {
		return fmt.Errorf("eventsub: %w", err)
	}

	return c.ingestLoop(ctx, c.EventSub.OutboundEvents(), in)
}

// SyncChannels joins and leaves channels to match the directory.
func (c *TwitchClient) SyncChannels(ctx context.Context) error {
	return c.EventSub.Sync(ctx)
}

// Send queues a message for its channel.
func (c *TwitchClient) Send(ctx context.Context, out core.Outbound) error {
	return c.Chat.Send(ctx, out)
}

// Shutdown closes the EventSub connection.
func (c *TwitchClient) Shutdown(ctx context.Context) {
	if err := c.EventSub.Shutdown(ctx); err != nil {
		log.Printf("[Twitch] EventSub shutdown: %v", err)
	}
}
