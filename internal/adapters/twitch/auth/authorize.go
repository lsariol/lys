package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// BotScopes are what the bot account grants Lys: reading and sending chat as a bot.
var BotScopes = []string{"user:bot", "user:read:chat", "user:write:chat"}

// AuthorizeURL is where the bot account grants Lys its scopes. Twitch sends the
// browser back to redirectURI with a code and state.
func AuthorizeURL(clientID, redirectURI, state string, scopes []string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("state", state)
	q.Set("force_verify", "true")
	return "https://id.twitch.tv/oauth2/authorize?" + q.Encode()
}

// ParseRedirect takes what the user pasted (the whole address Twitch sent the
// browser to, or just the code) and returns the code, checking state when the
// address carries one.
func ParseRedirect(pasted, wantState string) (string, error) {
	pasted = strings.TrimSpace(pasted)
	if pasted == "" {
		return "", errors.New("nothing was pasted")
	}
	if !strings.Contains(pasted, "code=") && !strings.Contains(pasted, "error=") {
		return pasted, nil
	}

	u, err := url.Parse(pasted)
	if err != nil {
		return "", fmt.Errorf("that isn't an address: %w", err)
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("Twitch: %s: %s", e, q.Get("error_description"))
	}
	if s := q.Get("state"); s != "" && s != wantState {
		return "", errors.New("the address is from another authorization attempt (state doesn't match): start again")
	}
	code := q.Get("code")
	if code == "" {
		return "", errors.New("the address has no code")
	}
	return code, nil
}

// ExchangeCode trades an authorization code for the account's token pair.
func (c *AuthClient) ExchangeCode(ctx context.Context, code, redirectURI string) (access, refresh string, err error) {
	data := url.Values{}
	data.Set("client_id", c.Config.ClientID)
	data.Set("client_secret", c.Config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("Twitch: exchange code: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("Twitch: exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var fail RefreshFail
		_ = json.NewDecoder(resp.Body).Decode(&fail)
		return "", "", fmt.Errorf("Twitch: exchange code: status %d: %s (codes work once and expire quickly; start again, and check the redirect address matches the Twitch app's)", resp.StatusCode, fail.Message)
	}

	var success RefreshSuccess
	if err := json.NewDecoder(resp.Body).Decode(&success); err != nil {
		return "", "", fmt.Errorf("Twitch: exchange code: decode response: %w", err)
	}
	if success.AccessToken == "" || success.RefreshToken == "" {
		return "", "", errors.New("Twitch: exchange code: the response is missing a token")
	}
	return success.AccessToken, success.RefreshToken, nil
}
