package eventsub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func (c *EventSubClient) JoinAllChannels(ctx context.Context) error {

	if err := c.Auth.RefreshUserAccessToken(ctx); err != nil {
		return fmt.Errorf("refresh user access token: %w", err)
	}

	for _, channel := range c.Channels {

		body := map[string]any{
			"type":    "channel.chat.message",
			"version": "1",
			"condition": map[string]string{
				"broadcaster_user_id": channel.UserID,
				"user_id":             c.Config.BotAccountID,
			},
			"transport": map[string]string{
				"method":     "websocket",
				"session_id": c.SessionData.SessionID,
			},
		}

		buf, _ := json.Marshal(body)
		req, err := http.NewRequest("POST", EventSubAPIBaseURL, bytes.NewReader(buf))
		if err != nil {
			return fmt.Errorf("join: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.Auth.Tokens.GetUserAccessToken()) // MUST be a user token for WebSocket subs
		req.Header.Set("Client-Id", c.Config.ClientID)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 202 {
			log.Printf("join all channels: status code %d\n", resp.StatusCode)
			log.Println(resp.Status)
			log.Printf("unable to join ID: %s, Name: %s\n", channel.UserID, channel.Username)
			continue
		}
		var respData EventSubJoinResponse
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			return err
		}

		ch := c.Channels[channel.UserID]
		ch.SubscriptionID = respData.Data[0].ID
		c.Channels[channel.UserID] = ch
		log.Printf("[EventSub] joined %s", ch.Username)
	}

	return nil
}

func (c *EventSubClient) Leave(ctx context.Context, targets []string) error {

	for _, channelID := range targets {

		subscriptionID := c.Channels[channelID].SubscriptionID
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, EventSubAPIBaseURL+"?id="+subscriptionID, nil)
		if err != nil {
			return fmt.Errorf("unsubscribe: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.Auth.Tokens.GetUserAccessToken()) // MUST be a user token for WebSocket subs
		req.Header.Set("Client-Id", c.Config.ClientID)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusNoContent:
			fmt.Println("Successfully delete")

			//db.Purge(user)

		case http.StatusBadRequest:
			fmt.Println("400: Bad Delete")

		case http.StatusUnauthorized:
			fmt.Println("401: Unauthorized")

		case http.StatusNotFound:
			fmt.Println("404: Subscription Not found")

		}
	}

	return nil
}
