package eventsub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/lsariol/lys/internal/core"
)

// JoinAllChannels subscribes to chat in every channel in c.Channels, for the
// current websocket session.
func (c *EventSubClient) JoinAllChannels(ctx context.Context) error {

	if err := c.Auth.RefreshUserAccessToken(ctx); err != nil {
		return fmt.Errorf("refresh user access token: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.Channels {
		sub, err := c.subscribe(ctx, id)
		if err != nil {
			log.Printf("[EventSub] join %q (%s): %v", ch.Username, id, err)
			continue
		}
		ch.SubscriptionID = sub
		c.Channels[id] = ch
		log.Printf("[EventSub] joined %s", ch.Username)
	}
	return nil
}

// Sync joins the channels the directory now lists and leaves the ones it no
// longer does, without reconnecting. Before the first connection it does nothing.
func (c *EventSubClient) Sync(ctx context.Context) error {
	if !c.Connected() {
		return nil
	}
	want, err := c.Directory.Channels(ctx, "twitch")
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	join, leave := diffChannels(c.Channels, want)
	if len(join) == 0 && len(leave) == 0 {
		return nil
	}
	if err := c.Auth.RefreshUserAccessToken(ctx); err != nil {
		return fmt.Errorf("refresh user access token: %w", err)
	}

	for _, id := range leave {
		ch := c.Channels[id]
		if ch.SubscriptionID != "" {
			if err := c.unsubscribe(ctx, ch.SubscriptionID); err != nil {
				log.Printf("[EventSub] leave %q (%s): %v", ch.Username, id, err)
				continue
			}
		}
		delete(c.Channels, id)
		log.Printf("[EventSub] left %s", ch.Username)
	}
	for _, ch := range join {
		sub, err := c.subscribe(ctx, ch.ID)
		if err != nil {
			log.Printf("[EventSub] join %q (%s): %v", ch.Name, ch.ID, err)
			continue
		}
		c.Channels[ch.ID] = Channel{UserID: ch.ID, Username: ch.Name, SubscriptionID: sub}
		log.Printf("[EventSub] joined %s", ch.Name)
	}
	return nil
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
	hint := ""
	switch resp.StatusCode {
	case http.StatusBadRequest:
		hint = " (check the channel's Twitch ID: lys channel list)"
	case http.StatusUnauthorized:
		hint = " (the bot's token isn't valid: run lys bot authorize)"
	case http.StatusForbidden:
		hint = " (the bot needs user:read:chat, and the channel must allow it)"
	}
	return fmt.Errorf("Twitch: subscribe: status %d: %s%s", resp.StatusCode, reason, hint)
}
