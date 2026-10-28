package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// refreshBefore is how long before the user token runs out it's renewed.
const refreshBefore = 10 * time.Minute

// EnsureUserToken refreshes the bot's user token if it runs out within
// refreshBefore (or its expiry isn't known), and otherwise does nothing.
func (c *AuthClient) EnsureUserToken(ctx context.Context) error {
	c.refreshMu.Lock()
	expires := c.userExpires
	c.refreshMu.Unlock()
	if !expires.IsZero() && time.Until(expires) > refreshBefore {
		return nil
	}
	return c.RefreshUserAccessToken(ctx)
}

// RefreshUserAccessToken trades the bot's refresh token for a new pair and
// saves it to Cove. Use EnsureUserToken unless Twitch just refused the token.
func (c *AuthClient) RefreshUserAccessToken(ctx context.Context) error {

	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	data := url.Values{}
	data.Set("client_id", c.Config.ClientID)
	data.Set("client_secret", c.Config.ClientSecret)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.Tokens.GetUserRefreshToken())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("Twitch: refresh user token: new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Twitch: refresh user token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var fail RefreshFail
		if err := json.NewDecoder(resp.Body).Decode(&fail); err != nil {
			return fmt.Errorf("Twitch: refresh user token: status %d", resp.StatusCode)
		}
		return fmt.Errorf("Twitch: refresh user token: status %d: %s (if the refresh token is invalid, re-authorize the bot account and store its tokens in Cove)", resp.StatusCode, fail.Message)
	}

	var success RefreshSuccess
	if err := json.NewDecoder(resp.Body).Decode(&success); err != nil {
		return fmt.Errorf("Twitch: refresh user token: decode response: %w", err)
	}
	if success.AccessToken == "" || success.RefreshToken == "" {
		return fmt.Errorf("Twitch: refresh user token: response is missing a token")
	}

	c.Tokens.SetUserAccessTokens(success.AccessToken, success.RefreshToken)
	c.userExpires = time.Time{}
	if success.ExpiresIn > 0 {
		c.userExpires = time.Now().Add(time.Duration(success.ExpiresIn) * time.Second)
	}

	return c.storeUserTokens(success.AccessToken, success.RefreshToken)
}
