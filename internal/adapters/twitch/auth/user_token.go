package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *AuthClient) RefreshUserAccessToken(ctx context.Context) error {

	data := url.Values{}
	data.Set("client_id", c.Config.ClientID)
	data.Set("client_secret", c.Config.ClientSecret)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.Config.TokenPair.RefreshToken)

	req, err := http.NewRequest("POST", "https://id.twitch.tv/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("http new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	var fail RefreshFail
	var success RefreshSuccess

	if resp.StatusCode != http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&fail); err != nil {
			return fmt.Errorf("json decode: %w", err)
		}
		return fmt.Errorf("RefreshUserAccessToken: StatusCode not OK. Code %d: Message %s: Error: %s: %w", fail.Status, fail.Message, fail.Error, err)
	}

	if err := json.NewDecoder(resp.Body).Decode(&success); err != nil {
		return fmt.Errorf("decode: StatusCode OK: Decode Error: %w", err)
	}

	c.Tokens.SetUserAccessTokens(success.AccessToken, success.RefreshToken)

	c.updateTokens()
	//c.DB.StoreUserAccessTokens(ctx, c.Tokens.GetUserAccessToken(), c.Tokens.GetUserRefreshToken())

	return nil

}
