package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/lsariol/coveclient"
	"github.com/lsariol/lys/internal/config"
)

// Cove keys holding the bot account's user token pair.
const (
	keyUserAccessToken  = "LYS_TWITCH_ACCESS_TOKEN"
	keyUserRefreshToken = "LYS_TWITCH_REFRESH_TOKEN"
)

const tokenURL = "https://id.twitch.tv/oauth2/token"

// AuthClient holds the bot's Twitch tokens and keeps them fresh.
type AuthClient struct {
	Cove   *coveclient.Client
	HTTP   *http.Client
	Config *config.TwitchConfig
	Tokens *SafeTwitchTokens

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

	access, err := c.Cove.GetSecret(keyUserAccessToken)
	if err != nil {
		return fmt.Errorf("Cove: get %s: %w (store the bot account's user access token there)", keyUserAccessToken, err)
	}

	refresh, err := c.Cove.GetSecret(keyUserRefreshToken)
	if err != nil {
		return fmt.Errorf("Cove: get %s: %w (store the bot account's refresh token there)", keyUserRefreshToken, err)
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
		return fmt.Errorf("Twitch: the tokens in %s belong to %q (%s), not the bot account %s: store the bot account's tokens there", keyUserAccessToken, info.Login, info.UserId, c.Config.BotAccountID)
	}

	if err := c.RefreshAppAccessToken(ctx); err != nil {
		return fmt.Errorf("refresh app access token: %w", err)
	}

	return nil
}
