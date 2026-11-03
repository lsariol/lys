// Package eventsub handles the entire life cycle of the twitch websocket and its connections
package eventsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/core"
)

// CLI URLS
// const (
// 	HelixBaseURL       = "https://api.twitch.tv/helix"
// 	EventSubAPIBaseURL = "http://127.0.0.1:8080/eventsub/subscriptions"
// 	EventSubWSURL      = "ws://127.0.0.1:8080/ws"
// )

const (
	HelixBaseURL       = "https://api.twitch.tv/helix"
	EventSubAPIBaseURL = "https://api.twitch.tv/helix/eventsub/subscriptions"
	EventSubWSURL      = "wss://eventsub.wss.twitch.tv/ws"
)

type EventSubClient struct {
	HTTP        *http.Client
	WS          *websocket.Conn
	Directory   Channels
	Config      *config.TwitchConfig
	Auth        *auth.AuthClient
	Channels    map[string]Channel // by broadcaster ID; guarded by mu once running
	SessionData SessionData
	out         chan EventSubMessage

	mu           sync.Mutex // guards Channels between the read loop's reconnects, Sync and the joins
	syncMu       sync.Mutex // one Sync at a time
	shutdownOnce sync.Once
	connected    atomic.Bool // joined and reading; false while reconnecting
	botRevoked   atomic.Bool // Twitch revoked the bot account's authorization
	unknownTypes sync.Map    // notification types already logged as unknown
	log          *slog.Logger
}

// Channel is a joined broadcaster and its chat subscription.
type Channel struct {
	UserID         string
	Username       string
	SubscriptionID string
}

// Channels lists the channels to join, and records the ones Lys must leave.
type Channels interface {
	core.Directory
	core.Revoker
}

func New(http *http.Client, cfg *config.TwitchConfig, auth *auth.AuthClient, directory Channels, log *slog.Logger) *EventSubClient {

	return &EventSubClient{
		HTTP:      http,
		Config:    cfg,
		Auth:      auth,
		Directory: directory,
		Channels:  make(map[string]Channel),
		out:       make(chan EventSubMessage, 100),
		log:       log.With("component", "eventsub"),
	}
}

func (c *EventSubClient) OutboundEvents() <-chan EventSubMessage { return c.out }

// Connected reports whether the websocket is up, the channels are joined, and
// the bot's authorization still stands.
func (c *EventSubClient) Connected() bool { return c.connected.Load() && !c.botRevoked.Load() }

func (c *EventSubClient) Initialize(ctx context.Context) error {

	if err := c.loadChannels(ctx); err != nil {
		return fmt.Errorf("load channels: %w", err)
	}

	ws, sd, err := c.NewWebSocketConn(ctx, EventSubWSURL)
	if err != nil {
		return fmt.Errorf("dial web socket: %w", err)
	}

	c.WS = ws
	c.SessionData = sd

	return nil
}

func (c *EventSubClient) Run(ctx context.Context) error {

	if err := c.JoinAllChannels(ctx); err != nil {
		return fmt.Errorf("join error: %w", err)
	}
	c.connected.Store(true)

	go c.readLoop(ctx)

	return nil
}

// Shutdown closes the websocket; it's safe to call more than once. The out
// channel stays open, since the read loop may still be sending on it;
// readers stop on the context instead.
func (c *EventSubClient) Shutdown(ctx context.Context) error {

	c.shutdownOnce.Do(func() {
		if c.WS == nil {
			return
		}
		_ = c.WS.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"),
			time.Now().Add(1*time.Second),
		)
		_ = c.WS.Close()
	})

	return nil
}

// keepaliveGrace is added to Twitch's keepalive timeout before a silent
// connection counts as dead.
const keepaliveGrace = 5 * time.Second

// defaultKeepalive is used until Twitch says otherwise.
const defaultKeepalive = 10 * time.Second

func (c *EventSubClient) readLoop(ctx context.Context) error {

	for {
		if ctx.Err() != nil {
			c.Shutdown(ctx)
			return nil
		}

		// Twitch sends a keepalive when there's nothing else; silence for longer
		// than its timeout means the connection is dead.
		keepalive := time.Duration(c.SessionData.KeepAliveTimeout) * time.Second
		if keepalive <= 0 {
			keepalive = defaultKeepalive
		}
		_ = c.WS.SetReadDeadline(time.Now().Add(keepalive + keepaliveGrace))

		messageType, data, err := c.WS.ReadMessage()
		if err != nil {
			// Shutdown closed the socket: stop instead of reconnecting.
			if ctx.Err() != nil {
				return nil
			}
			c.log.Warn("connection lost; reconnecting", "err", err)
			c.connected.Store(false)
			for !c.resetWebSocket(ctx, EventSubWSURL) {
				c.log.Error("reconnecting failed 8 times, so Lys hears no chat; trying again")
			}
			c.connected.Store(ctx.Err() == nil)
			continue
		}

		if messageType != websocket.TextMessage {
			c.log.Warn("ignoring a websocket message that isn't text", "ws_type", messageType, "length", len(data))
			continue
		}
		var event EventSubMessage
		if err := json.Unmarshal(data, &event); err != nil {
			c.unreadable(data, err)
			continue
		}
		c.ConsumeEvent(ctx, event)
	}
}

// unreadable logs a message Lys can't read by its type and length; never its
// payload, which carries chat.
func (c *EventSubClient) unreadable(data []byte, err error) {
	var meta struct {
		Metadata struct {
			MessageType string `json:"message_type"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(data, &meta)
	c.log.Warn("unreadable message", "message_type", meta.Metadata.MessageType, "length", len(data), "err", err)
}

func (c *EventSubClient) ConsumeEvent(ctx context.Context, event EventSubMessage) {

	switch event.Metadata.MessageType {
	case "session_keepalive":
		// Nothing to do: any message extends the read deadline.

	case "session_welcome":
		// Welcomes are read when connecting; one now is unexpected but harmless.
		c.log.Warn("unexpected welcome")

	case "session_reconnect":
		c.log.Info("Twitch asked to move to a new connection")
		if err := c.reconnectWebSocket(ctx, event); err != nil {
			c.log.Warn("moving to Twitch's new address failed; starting a fresh connection", "err", err)
			c.connected.Store(false)
			c.connected.Store(c.resetWebSocket(ctx, EventSubWSURL) && ctx.Err() == nil)
		}

	case "revocation":
		c.revoked(ctx, event.Payload.Subscription)

	case "notification":
		switch t := event.Payload.Subscription.Type; t {
		case "channel.chat.message":
			select {
			case c.out <- event:
			case <-ctx.Done():
			}
		default:
			if _, seen := c.unknownTypes.LoadOrStore(t, true); !seen {
				c.log.Warn("ignoring notifications Lys doesn't subscribe to", "subscription_type", t)
			}
		}

	default:
		c.log.Warn("ignoring a message of an unknown type", "message_type", event.Metadata.MessageType)
	}
}

// revocationReasons are the revocations that mean Lys must leave that one channel.
var revocationReasons = map[string]string{
	"chat_user_banned": "the bot was banned in the channel",
	"user_removed":     "the channel's account no longer exists",
}

// revoked handles Twitch ending a subscription; after a revocation it sends
// nothing more for it.
func (c *EventSubClient) revoked(ctx context.Context, sub Subscription) {
	channel := sub.Condition.BroadcasterUserId
	name := channel
	c.mu.Lock()
	if ch, ok := c.Channels[channel]; ok {
		name = ch.Username
	}
	c.mu.Unlock()

	if reason, ok := revocationReasons[sub.Status]; ok {
		c.log.Warn("leaving a channel Twitch revoked; lys channel add brings it back", "channel", name, "channel_id", channel, "reason", reason)
		if err := c.Directory.Revoke(ctx, "twitch", channel, reason); err != nil {
			c.log.Error("recording the revoked channel failed", "channel", name, "channel_id", channel, "err", err)
		}
		return
	}

	switch sub.Status {
	case "authorization_revoked":
		// The subscriptions use the bot's own token, so this is the bot account
		// taking back its authorization: every channel stops, not just this one.
		if !c.botRevoked.Swap(true) {
			c.log.Error("Twitch revoked the bot account's authorization, so Lys hears no chat. Run lys bot authorize, then restart Lys.")
		}
	case "version_removed":
		c.log.Error("Twitch retired a subscription version: Lys needs updating to a newer one", "subscription_type", sub.Type, "version", sub.Version)
	default:
		c.log.Warn("subscription ended", "channel", name, "channel_id", channel, "status", sub.Status)
	}
}

func (c *EventSubClient) loadChannels(ctx context.Context) error {

	channels, err := c.Directory.Channels(ctx, "twitch")
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}

	loaded := make(map[string]Channel, len(channels))
	for _, ch := range channels {
		loaded[ch.ID] = Channel{UserID: ch.ID, Username: ch.Name}
	}
	c.mu.Lock()
	c.Channels = loaded
	c.mu.Unlock()

	return nil
}
