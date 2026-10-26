package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// RefreshAppAccessToken fetches a new app token; it lives in memory only.
func (c *AuthClient) RefreshAppAccessToken(ctx context.Context) error {

	vals := url.Values{}
	vals.Set("client_id", c.Config.ClientID)
	vals.Set("client_secret", c.Config.ClientSecret)
	vals.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return fmt.Errorf("Twitch: get app token: new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Twitch: get app token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Twitch: get app token: status %d (check LYS_TWITCH_CLIENT_ID and LYS_TWITCH_CLIENT_SECRET in Cove)", resp.StatusCode)
	}

	var data struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("Twitch: get app token: decode response: %w", err)
	}
	if data.AccessToken == "" {
		return fmt.Errorf("Twitch: get app token: response is missing the token")
	}

	c.Tokens.SetAppAccessTokens(data.AccessToken)

	return nil
}
