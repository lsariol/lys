package config

import (
	"fmt"
	"os"
	"strings"

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

	// The bot account, from bot_accounts (lys bot authorize records it).
	BotAccountID    string
	AccessTokenKey  string
	RefreshTokenKey string

	LogChat bool // LOG_CHAT=true: log every chat message, not only commands
}

func NewTwitchConfig() *TwitchConfig {
	return &TwitchConfig{AccessTokenKey: DefaultAccessTokenKey, RefreshTokenKey: DefaultRefreshTokenKey,
		LogChat: strings.EqualFold(os.Getenv("LOG_CHAT"), "true")}
}

func (c *Config) InitializeTwitchConfig(cove *coveclient.Client) error {

	clientID, err := cove.GetSecret("LYS_TWITCH_CLIENT_ID")
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_CLIENT_ID: %w", err)
	}

	clientSecret, err := cove.GetSecret("LYS_TWITCH_CLIENT_SECRET")
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_CLIENT_SECRET from Cove: %w", err)
	}

	c.Twitch.ClientID = clientID
	c.Twitch.ClientSecret = clientSecret

	return nil
}
