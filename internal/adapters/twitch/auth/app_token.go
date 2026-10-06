package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
)

func (c *AuthClient) RefreshAppAccessToken(ctx context.Context) error {

	vals := url.Values{}
	vals.Set("client_id", c.Config.ClientID)
	vals.Set("client_secret", c.Config.ClientSecret)
	vals.Set("grant_type", "client_credentials")
	urlVals := vals.Encode()

	req, err := http.NewRequest("POST", "https://id.twitch.tv/oauth2/token", strings.NewReader(urlVals))
	if err != nil {
		return fmt.Errorf("RefreshAppAccessToken: error with request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RefreshAppAccessToken: response unexpected status: %d: %w", resp.StatusCode, err)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("RefreshAppAccessToken: error with decode: %w", err)
	}

	appAccessToken, ok := data["access_token"].(string)

	if !ok {
		return fmt.Errorf("RefreshAppAccessToken: Access token not found in response")
	}

	c.Tokens.SetAppAccessTokens(appAccessToken)
	err = c.StoreAppAccessTokens(ctx, appAccessToken)
	if err != nil {
		return fmt.Errorf("store app access tokens: %w", err)
	}

	return nil
}

func (c *AuthClient) GetAppAccessTokens(ctx context.Context) (twitchdb.TwitchAppAccessToken, error) {

	var appAccessToken twitchdb.TwitchAppAccessToken

	token, err := c.Cove.GetSecret("LYS_TWITCH_ACCESS_TOKEN")
	if err != nil {
		return appAccessToken, fmt.Errorf("cove getSecret: %w", err)
	}

	appAccessToken.TwitchAppAccessToken = token

	return appAccessToken, nil
}

func (c *AuthClient) StoreAppAccessTokens(ctx context.Context, appAccessToken string) error {

	err := c.Cove.UpdateSecret("LYS_TWITCH_ACCESS_TOKEN", appAccessToken)
	if err != nil {
		return fmt.Errorf("cove updateSecret: %w", err)
	}

	return nil
}
