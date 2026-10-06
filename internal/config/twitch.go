package config

import (
	"fmt"

	"github.com/lsariol/coveclient"
)

type TwitchConfig struct {
	ClientID       string
	ClientSecret   string
	BotAccountID   string
	BotAccountName string

	TokenPair TokenPair
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
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

	botName, err := cove.GetSecret("LYS_TWITCH_BOT_NAME")
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_BOT_NAME: %w", err)
	}

	accessToken, err := cove.GetSecret("LYS_TWITCH_ACCESS_TOKEN")
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_ACCESS_TOKEN: %w", err)
	}

	refreshToken, err := cove.GetSecret("LYS_TWITCH_REFRESH_TOKEN")
	if err != nil {
		return fmt.Errorf("get secret LYS_TWITCH_REFRESH_TOKEN: %w", err)
	}

	c.Twitch.ClientID = clientID
	c.Twitch.ClientSecret = clientSecret
	c.Twitch.BotAccountID = botID
	c.Twitch.BotAccountName = botName

	tokenPair := TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	c.Twitch.TokenPair = tokenPair

	return nil
}

func (c *Config) UpdateTokenPair() error {

	return nil
}
