package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const sendChatURL = "https://api.twitch.tv/helix/chat/messages"

// SendChat posts one message as the bot, with the app token. A non-200 answer
// comes back as a HelixError.
func (c *ChatClient) SendChat(ctx context.Context, broadcasterID, text string) (*SendChatMessageResponse, *HelixError, error) {

	payload, err := json.Marshal(SendChatBody{
		BroadcasterID: broadcasterID,
		SenderID:      c.Config.BotAccountID,
		Message:       text,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("encode body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendChatURL, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Auth.Tokens.GetAppAccessToken())
	req.Header.Set("Client-Id", c.Config.ClientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}

	c.stats.record(resp.Header, resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		helixErr := HelixError{Status: resp.StatusCode}
		if err := json.Unmarshal(respBody, &helixErr); err != nil {
			return nil, nil, fmt.Errorf("status %d: unmarshal helix error: %w", resp.StatusCode, err)
		}
		if reset, err := strconv.ParseInt(resp.Header.Get("Ratelimit-Reset"), 10, 64); err == nil {
			helixErr.RetryAfter = time.Until(time.Unix(reset, 0))
		}
		return nil, &helixErr, nil
	}

	var data SendChatMessageResponse
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, nil, fmt.Errorf("unmarshal successful response: %w", err)
	}

	return &data, nil, nil
}
