// Package eventsub handles the entire life cycle of the twitch websocket and its connections
package eventsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/config"
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
	DB          *twitchdb.Store
	Config      *config.TwitchConfig
	Auth        *auth.AuthClient
	Channels    map[string]twitchdb.TwitchChannel
	SessionData SessionData
	out         chan EventSubMessage

	shutdownOnce sync.Once
}

func New(http *http.Client, cfg *config.TwitchConfig, auth *auth.AuthClient, db *twitchdb.Store) *EventSubClient {

	return &EventSubClient{
		HTTP:     http,
		Config:   cfg,
		Auth:     auth,
		DB:       db,
		Channels: make(map[string]twitchdb.TwitchChannel),
		out:      make(chan EventSubMessage, 100),
	}
}

func (c *EventSubClient) OutboundEvents() <-chan EventSubMessage { return c.out }

func (c *EventSubClient) Initilize(ctx context.Context) error {

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

	var e error

	go c.readLoop(ctx)

	return e
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

func (c *EventSubClient) hardReset() {

}

func (c *EventSubClient) readLoop(ctx context.Context) error {

	fmt.Println("[Chat] Running")

	for {
		select {
		case <-ctx.Done():
			c.Shutdown(ctx)
			return nil

		default:
			messageType, data, err := c.WS.ReadMessage()
			if err != nil {
				// Shutdown closed the socket: stop instead of reconnecting.
				if ctx.Err() != nil {
					return nil
				}
				if c.resetWebSocket(ctx, EventSubWSURL) {
					continue
				}

				log.Printf("max attempts (8) reached, performing hard reset")
				//c.hardReset()

			}

			switch {
			case messageType == websocket.TextMessage:
				var event EventSubMessage
				if err := json.Unmarshal(data, &event); err != nil {
					fmt.Println(fmt.Errorf("json unmarshal: %w", err))
					fmt.Println(string(data))
				}

				c.ConsumeEvent(ctx, event)
				continue

			case messageType == websocket.CloseMessage || messageType == -1:
				log.Println("Read error, should be wsarecv")
				log.Println("messagetype == websocket.CloseMessage || messageType == -1 ERROR PATH.")
				log.Printf("%d: %s", messageType, err.Error())
				c.resetWebSocket(ctx, EventSubWSURL)
				continue

			default:
				log.Printf("unknown message type: %d", messageType)
				log.Println("broken input: " + string(data))
			}

		}
	}
}

func (c *EventSubClient) ConsumeEvent(ctx context.Context, event EventSubMessage) {

	switch event.Metadata.MessageType {
	case "session_welcome":
		c.handleSessionWelcome()
		return

	case "session_keepalive":
		//Add a timer to see if the socket is alive and healthy
		return

	case "session_reconnect":

		log.Println("Twitch has asked to reconnect.")
		err := c.reconnectWebSocket(ctx, event)

		if err != nil {
			fmt.Println("error in handleEvent session_reconnect %w", err)
			panic(err)
		}
		return

	case "revocation":

		fmt.Println("Revocation")
		//youll receive the message once and then no longer receive messages for the specified user and subscription type
		// check status field on how to handle

		switch event.Payload.Subscription.Status {

		case "user_removed":
			// user_removed -> user mentioned in the subscription no longer exists. ( Channel banned or whatver)
			fmt.Println("User Removed")
			return

		case "authorization_revoked":
			// authorization_revoked -> user revoked the authorization token that the subscription relied on (user removed bot permissions), remove user from subscription list
			fmt.Println("Auth Revoked")
			return

		case "version_removed":
			// version_removed -> the subscribed to subscription type and version is no longer supported
			fmt.Println("Version removed")
			return
		}
		return

	case "notification":
		//check if message is a command

		switch event.Payload.Subscription.Type {
		case "channel.chat.message":

			c.out <- event

		default:
			fmt.Printf("Unknown notification type '%s'\n", event.Payload.Subscription.Type)
			//log error to db
		}

	default:

	}
}

func (c *EventSubClient) loadChannels(ctx context.Context) error {

	channels, err := c.DB.GetAllChannels(ctx)
	if err != nil {
		return fmt.Errorf("get all channels: %w", err)
	}

	c.Channels = channels

	return nil
}

func (c *EventSubClient) handleSessionWelcome() {

}
