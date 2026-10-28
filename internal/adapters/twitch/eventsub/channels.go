package eventsub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/lsariol/lys/internal/core"
)

// joinAtOnce is how many channels are subscribed to at the same time. One at a
// time, 300 channels took 45 s; Twitch wants them within 10 s of the welcome.
const joinAtOnce = 8

// JoinAllChannels subscribes to chat in every channel in c.Channels, for the
// current websocket session.
func (c *EventSubClient) JoinAllChannels(ctx context.Context) error {
	if err := c.Auth.EnsureUserToken(ctx); err != nil {
		return fmt.Errorf("refresh user access token: %w", err)
	}

	c.mu.Lock()
	var channels []core.Channel
	for id, ch := range c.Channels {
		channels = append(channels, core.Channel{ID: id, Name: ch.Username})
	}
	c.mu.Unlock()

	c.join(ctx, channels)
	return nil
}

// Sync joins the channels the directory now lists and leaves the ones it no
// longer does, without reconnecting. Before the first connection it does nothing.
func (c *EventSubClient) Sync(ctx context.Context) error {
	if !c.Connected() {
		return nil
	}
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	want, err := c.Directory.Channels(ctx, "twitch")
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}

	c.mu.Lock()
	join, leave := diffChannels(c.Channels, want)
	leaving := make([]Channel, 0, len(leave))
	for _, id := range leave {
		leaving = append(leaving, c.Channels[id])
	}
	c.mu.Unlock()
	if len(join) == 0 && len(leave) == 0 {
		return nil
	}
	if err := c.Auth.EnsureUserToken(ctx); err != nil {
		return fmt.Errorf("refresh user access token: %w", err)
	}

	for _, ch := range leaving {
		if ch.SubscriptionID != "" {
			if err := c.unsubscribe(ctx, ch.SubscriptionID); err != nil {
				log.Printf("[EventSub] leave %q (%s): %v", ch.Username, ch.UserID, err)
				continue
			}
		}
		c.mu.Lock()
		delete(c.Channels, ch.UserID)
		c.mu.Unlock()
		log.Printf("[EventSub] left %s", ch.Username)
	}
	c.join(ctx, join)
	return nil
}

// join subscribes to the channels, joinAtOnce at a time, and records each one
// that worked. The lock is held only to record, never across a request.
func (c *EventSubClient) join(ctx context.Context, channels []core.Channel) {
	slots := make(chan struct{}, joinAtOnce)
	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			sub, err := c.subscribe(ctx, ch.ID)
			if errors.Is(err, errUnauthorized) {
				// The token may have just run out: refresh it and try once more.
				if err = c.Auth.RefreshUserAccessToken(ctx); err == nil {
					sub, err = c.subscribe(ctx, ch.ID)
				}
			}
			if err != nil {
				log.Printf("[EventSub] join %q (%s): %v", ch.Name, ch.ID, err)
				return
			}
			c.mu.Lock()
			c.Channels[ch.ID] = Channel{UserID: ch.ID, Username: ch.Name, SubscriptionID: sub}
			c.mu.Unlock()
			log.Printf("[EventSub] joined %s", ch.Name)
		}()
	}
	wg.Wait()
}

// diffChannels returns the channels to join (in want, not joined) and the IDs
// to leave (joined, not in want).
func diffChannels(joined map[string]Channel, want []core.Channel) (join []core.Channel, leave []string) {
	wanted := make(map[string]bool, len(want))
	for _, w := range want {
		wanted[w.ID] = true
		if _, ok := joined[w.ID]; !ok {
			join = append(join, w)
		}
	}
	for id := range joined {
		if !wanted[id] {
			leave = append(leave, id)
		}
	}
	return join, leave
}

// subscribe asks Twitch for a broadcaster's chat on this session and returns
// the subscription's ID.
func (c *EventSubClient) subscribe(ctx context.Context, broadcasterID string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"type":    "channel.chat.message",
		"version": "1",
		"condition": map[string]string{
			"broadcaster_user_id": broadcasterID,
			"user_id":             c.Config.BotAccountID,
		},
		"transport": map[string]string{
			"method":     "websocket",
			"session_id": c.SessionData.SessionID,
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, EventSubAPIBaseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Auth.Tokens.GetUserAccessToken()) // MUST be a user token for WebSocket subs
	req.Header.Set("Client-Id", c.Config.ClientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("Twitch: subscribe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return "", subscribeError(resp)
	}
	var data EventSubJoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("Twitch: subscribe: %w", err)
	}
	if len(data.Data) == 0 {
		return "", fmt.Errorf("Twitch: subscribe: the response has no subscription")
	}
	return data.Data[0].ID, nil
}

// unsubscribe deletes a chat subscription; one that's already gone counts as done.
func (c *EventSubClient) unsubscribe(ctx context.Context, subscriptionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, EventSubAPIBaseURL+"?id="+subscriptionID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Auth.Tokens.GetUserAccessToken())
	req.Header.Set("Client-Id", c.Config.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Twitch: unsubscribe: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("Twitch: unsubscribe: status %s", resp.Status)
	}
	return nil
}

// errUnauthorized is a subscription Twitch refused for the bot's token.
var errUnauthorized = errors.New("Twitch: subscribe: status 401, the bot's token was refused")

// subscribeError turns a refused subscription into an error with Twitch's
// reason and, where it's clear, how to fix it.
func subscribeError(resp *http.Response) error {
	var body struct {
		Message string `json:"message"`
	}
	reason := "Twitch gave no reason"
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&body); err != nil {
		reason = "unreadable answer: " + err.Error()
	} else if body.Message != "" {
		reason = body.Message
	}
	switch resp.StatusCode {
	case http.StatusBadRequest:
		return fmt.Errorf("Twitch: subscribe: status 400: %s (check the channel's Twitch ID: lys channel list)", reason)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s (if it keeps happening, run lys bot authorize)", errUnauthorized, reason)
	case http.StatusForbidden:
		return fmt.Errorf("Twitch: subscribe: status 403: %s (the bot needs user:read:chat, and the channel must allow it)", reason)
	}
	return fmt.Errorf("Twitch: subscribe: status %d: %s", resp.StatusCode, reason)
}
