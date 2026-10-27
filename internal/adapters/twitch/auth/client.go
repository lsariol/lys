package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/lsariol/coveclient"
	"github.com/lsariol/lys/internal/config"
)

const tokenURL = "https://id.twitch.tv/oauth2/token"

// AuthClient holds the bot's Twitch tokens and keeps them fresh.
type AuthClient struct {
	Cove   *coveclient.Client
	HTTP   *http.Client
	Config *config.TwitchConfig
	Tokens *SafeTwitchTokens

	// Bot is what Twitch said about the bot's token at startup.
	Bot *VerificationSuccess

	refreshMu sync.Mutex
}

func New(cc *coveclient.Client, cfg *config.TwitchConfig, http *http.Client) *AuthClient {
	return &AuthClient{
		Cove:   cc,
		Config: cfg,
		HTTP:   http,
		Tokens: NewSafeTwitchTokens(),
	}
}

// Initilize loads the bot's user tokens from Cove, refreshes them, checks they
// belong to the bot account, and fetches an app token.
func (c *AuthClient) Initilize(ctx context.Context) error {

	if c.Config.BotAccountID == "" {
		return fmt.Errorf("Twitch: no bot account: run `lys bot authorize` (or store LYS_TWITCH_BOT_ID in Cove)")
	}

	access, err := c.Cove.GetSecret(c.Config.AccessTokenKey)
	if err != nil {
		return fmt.Errorf("Cove: get %s: %w (store the bot account's user access token there, or run `lys bot authorize`)", c.Config.AccessTokenKey, err)
	}

	refresh, err := c.Cove.GetSecret(c.Config.RefreshTokenKey)
	if err != nil {
		return fmt.Errorf("Cove: get %s: %w (store the bot account's refresh token there, or run `lys bot authorize`)", c.Config.RefreshTokenKey, err)
	}

	c.Tokens.SetUserAccessTokens(access, refresh)

	if err := c.RefreshUserAccessToken(ctx); err != nil {
		return fmt.Errorf("refresh user access token: %w", err)
	}

	info, err := ValidateToken(c.Tokens.GetUserAccessToken(), c.HTTP)
	if err != nil {
		return fmt.Errorf("Twitch: validate user access token: %w", err)
	}
	if info.UserId != c.Config.BotAccountID {
		return fmt.Errorf("Twitch: the tokens in %s belong to %q (%s), not the bot account %s: store the bot account's tokens there", c.Config.AccessTokenKey, info.Login, info.UserId, c.Config.BotAccountID)
	}
	c.Bot = info

	if err := c.RefreshAppAccessToken(ctx); err != nil {
		return fmt.Errorf("refresh app access token: %w", err)
	}

	return nil
}
