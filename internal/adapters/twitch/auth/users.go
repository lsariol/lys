package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// TwitchUser is a Twitch account as Helix describes it.
type TwitchUser struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
}

// LookupUser finds a Twitch user by login, with the app token.
func (c *AuthClient) LookupUser(ctx context.Context, login string) (TwitchUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users?login="+url.QueryEscape(login), nil)
	if err != nil {
		return TwitchUser{}, fmt.Errorf("Twitch: look up %q: %w", login, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Tokens.GetAppAccessToken())
	req.Header.Set("Client-Id", c.Config.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return TwitchUser{}, fmt.Errorf("Twitch: look up %q: %w", login, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TwitchUser{}, fmt.Errorf("Twitch: look up %q: status %d", login, resp.StatusCode)
	}

	var body struct {
		Data []TwitchUser `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return TwitchUser{}, fmt.Errorf("Twitch: look up %q: decode response: %w", login, err)
	}
	if len(body.Data) == 0 {
		return TwitchUser{}, fmt.Errorf("Twitch: there's no user called %q", login)
	}
	return body.Data[0], nil
}
