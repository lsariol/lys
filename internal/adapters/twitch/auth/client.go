package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/lsariol/coveclient"
	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/config"
)

type AuthClient struct {
	DB     *twitchdb.Store
	Cove   *coveclient.Client
	HTTP   *http.Client
	Config *config.TwitchConfig
	Tokens *SafeTwitchTokens
}

func New(db *twitchdb.Store, cc *coveclient.Client, cfg *config.TwitchConfig, http *http.Client) *AuthClient {
	return &AuthClient{
		DB:     db,
		Cove:   cc,
		Config: cfg,
		HTTP:   http,
		Tokens: NewSafeTwitchTokens(),
	}
}

func (c *AuthClient) Initilize(ctx context.Context) error {

	userAccessTokens, err := c.DB.GetUserAccessTokens(ctx)
	if err != nil {
		return fmt.Errorf("GetUserAccessTokens: %w", err)
	}
	c.Tokens.SetUserAccessTokens(userAccessTokens.UserAccessToken, userAccessTokens.UserRefreshToken)

	appAccessToken, err := c.GetAppAccessTokens(ctx)
	if err != nil {
		return fmt.Errorf("GetAppAccessTokens: %w", err)
	}
	c.Tokens.SetAppAccessTokens(appAccessToken.TwitchAppAccessToken)

	return nil
}

func (c *AuthClient) Shutdown(ctx context.Context) error {

	if err := c.DB.StoreUserAccessTokens(ctx, c.Tokens.GetUserAccessToken(), c.Tokens.GetUserRefreshToken()); err != nil {
		return fmt.Errorf("store tokens: %w", err)
	}

	return nil
}
