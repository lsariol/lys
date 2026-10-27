package eventsub

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// welcomeTimeout is how long a new connection may take to send its welcome.
const welcomeTimeout = 10 * time.Second

// Creates and returns a websocket connection, and session data
func (c *EventSubClient) NewWebSocketConn(ctx context.Context, URL string) (*websocket.Conn, SessionData, error) {

	var sd SessionData
	newConn, _, err := websocket.DefaultDialer.DialContext(ctx, URL, nil)
	if err != nil {
		return nil, sd, fmt.Errorf("Twitch: dial EventSub: %w", err)
	}
	// Twitch sends the welcome right away; don't wait forever for it.
	_ = newConn.SetReadDeadline(time.Now().Add(welcomeTimeout))

	var event EventSubMessage
	for {
		if err := newConn.ReadJSON(&event); err != nil {
			newConn.Close()
			return nil, sd, fmt.Errorf("newwebsocketconn read json: %w", err)
		}

		if event.Metadata.MessageType == "session_welcome" {
			sd.SessionID = event.Payload.Session.ID
			sd.KeepAliveTimeout = event.Payload.Session.KeepaliveTimeoutSeconds
			return newConn, sd, nil
		}
	}
}

// Creates and stores a websocket in the eventsub component
func (c *EventSubClient) dialNewWebSocketConnection(ctx context.Context, URL string) error {

	newConn, _, err := websocket.DefaultDialer.DialContext(ctx, URL, nil)
	if err != nil {
		return fmt.Errorf("Twitch: dial EventSub: %w", err)
	}
	_ = newConn.SetReadDeadline(time.Now().Add(welcomeTimeout))

	var event EventSubMessage
	for event.Metadata.MessageType != "session_welcome" {
		if event.Metadata.MessageType != "" {
			c.ConsumeEvent(ctx, event)
		}
		event = EventSubMessage{}
		if err := newConn.ReadJSON(&event); err != nil {
			newConn.Close()
			return fmt.Errorf("Twitch: wait for the EventSub welcome: %w", err)
		}
	}

	var sD = SessionData{
		SessionID:        event.Payload.Session.ID,
		KeepAliveTimeout: event.Payload.Session.KeepaliveTimeoutSeconds,
	}

	c.WS = newConn
	c.SessionData = sD

	return nil
}

// Used for twitch session_reconnect message
func (c *EventSubClient) reconnectWebSocket(ctx context.Context, reconnectmsg EventSubMessage) error {

	reconnectUrl := reconnectmsg.Payload.Session.ReconnectURL

	ws, sd, err := c.NewWebSocketConn(ctx, reconnectUrl)
	if err != nil {
		return fmt.Errorf("newwebsocketconn error: %w", err)
	}

	//Send a close frame to twitch to prevent more messages being sent here.
	_ = c.WS.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "reconnecting"), time.Now().Add(time.Second))

	var event EventSubMessage
	for {
		if err := c.WS.ReadJSON(&event); err != nil {

			if c.WS != nil {
				c.WS.Close()
				c.WS = nil
			}

			c.WS = ws
			c.SessionData = sd

			return nil
		}

		c.ConsumeEvent(ctx, event)
		event = EventSubMessage{}
	}
}

// Used when an error occurs in the reading of the websocket. Creates an entire new websocket.
func (c *EventSubClient) resetWebSocket(ctx context.Context, url string) bool {

	dialBackoff := 200 * time.Millisecond
	attempt := 1
	maxAttempts := 8

	for attempt <= maxAttempts {

		_ = c.WS.Close()

		if err := c.dialNewWebSocketConnection(ctx, url); err != nil {
			log.Printf("dial attempt %d failed: %q\n", attempt, err)
			attempt += 1

		} else if err := c.JoinAllChannels(ctx); err != nil {
			log.Printf("join attempt %d failed: %q\n", attempt, err)
			attempt = 1

		} else {
			log.Println("Websocket Reconnected Successfully")
			return true
		}

		select {
		case <-time.After(dialBackoff):
			dialBackoff *= 2
		case <-ctx.Done():
			log.Println("context canceled, aborting websocket reset.")
			return true
		}
	}

	return false

}
