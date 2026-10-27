package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// usersURL is Helix's Get Users endpoint.
const usersURL = "https://api.twitch.tv/helix/users"

// ErrNoUser means Twitch has no user with that login or ID.
var ErrNoUser = errors.New("no such Twitch user")

// TwitchUser is a Twitch account as Helix describes it.
type TwitchUser struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
}

// LookupUser finds a Twitch user by login, with the app token.
func (c *AuthClient) LookupUser(ctx context.Context, login string) (TwitchUser, error) {
	u, err := c.lookupUser(ctx, url.Values{"login": {login}}, true)
	if errors.Is(err, ErrNoUser) {
		return TwitchUser{}, fmt.Errorf("Twitch: there's no user called %q: %w", login, err)
	}
	if err != nil {
		return TwitchUser{}, fmt.Errorf("Twitch: look up %q: %w", login, err)
	}
	return u, nil
}

// LookupUserByID finds a Twitch user by ID, with the app token.
func (c *AuthClient) LookupUserByID(ctx context.Context, id string) (TwitchUser, error) {
	u, err := c.lookupUser(ctx, url.Values{"id": {id}}, true)
	if errors.Is(err, ErrNoUser) {
		return TwitchUser{}, fmt.Errorf("Twitch: there's no user with ID %q: %w", id, err)
	}
	if err != nil {
		return TwitchUser{}, fmt.Errorf("Twitch: look up user %s: %w", id, err)
	}
	return u, nil
}

// lookupUser asks Get Users for one user. On a 401 it gets a new app token
// and, if retry is set, asks once more.
func (c *AuthClient) lookupUser(ctx context.Context, query url.Values, retry bool) (TwitchUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usersURL+"?"+query.Encode(), nil)
	if err != nil {
		return TwitchUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Tokens.GetAppAccessToken())
	req.Header.Set("Client-Id", c.Config.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return TwitchUser{}, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized && retry:
		if err := c.RefreshAppAccessToken(ctx); err != nil {
			return TwitchUser{}, err
		}
		return c.lookupUser(ctx, query, false)
	case resp.StatusCode == http.StatusBadRequest:
		return TwitchUser{}, ErrNoUser // a malformed login or ID
	case resp.StatusCode != http.StatusOK:
		return TwitchUser{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	var body struct {
		Data []TwitchUser `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return TwitchUser{}, fmt.Errorf("decode response: %w", err)
	}
	if len(body.Data) == 0 {
		return TwitchUser{}, ErrNoUser
	}
	return body.Data[0], nil
}
