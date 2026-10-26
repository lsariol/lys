package config

import (
	"fmt"

	"github.com/lsariol/coveclient"
)

type TwitchConfig struct {
	ClientID     string
	ClientSecret string
	BotAccountID string
}

func NewTwitchConfig() *TwitchConfig {
	return &TwitchConfig{}
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
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_BOT_ID: %w", err)
	}

	c.Twitch.ClientID = clientID
	c.Twitch.ClientSecret = clientSecret
	c.Twitch.BotAccountID = botID

	return nil
}
