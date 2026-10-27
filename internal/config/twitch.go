package config

import (
	"errors"
	"fmt"

	"github.com/lsariol/coveclient"
)

// Default Cove keys for the bot account's user token pair.
const (
	DefaultAccessTokenKey  = "LYS_TWITCH_ACCESS_TOKEN"
	DefaultRefreshTokenKey = "LYS_TWITCH_REFRESH_TOKEN"
)

type TwitchConfig struct {
	ClientID     string
	ClientSecret string

	// The bot account: from bot_accounts once it has a row; until then
	// BotAccountID comes from LYS_TWITCH_BOT_ID in Cove.
	BotAccountID    string
	AccessTokenKey  string
	RefreshTokenKey string
}

func NewTwitchConfig() *TwitchConfig {
	return &TwitchConfig{AccessTokenKey: DefaultAccessTokenKey, RefreshTokenKey: DefaultRefreshTokenKey}
}

func (c *Config) InitilizeTwitchConfig(cove *coveclient.Client) error {

	clientID, err := cove.GetSecret("LYS_TWITCH_CLIENT_ID")
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_CLIENT_ID: %w", err)
	}

	clientSecret, err := cove.GetSecret("LYS_TWITCH_CLIENT_SECRET")
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_CLIENT_SECRET: %w", err)
	}

	botID, err := cove.GetSecret("LYS_TWITCH_BOT_ID")
	if err != nil && !errors.Is(err, coveclient.ErrNotFound) {
		return fmt.Errorf("get secret LYS_TWITCH_BOT_ID: %w", err)
	}

	c.Twitch.ClientID = clientID
	c.Twitch.ClientSecret = clientSecret
	c.Twitch.BotAccountID = botID

	return nil
}
