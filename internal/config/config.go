package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/lsariol/coveclient"
)

type Config struct {
	Twitch   *TwitchConfig
	Database *DatabaseConfig
	Commands *CommandsConfig
}

// Holds never changing variables ONLY

func New() *Config {
	return &Config{
		Twitch:   NewTwitchConfig(),
		Database: NewDatabaseConfig(),
		Commands: NewCommandsConfig(),
	}
}

func (c *Config) Initialize(cove *coveclient.Client) error {

	env := strings.ToUpper(os.Getenv("APP_ENV"))

	err := c.InitializeDatabaseConfig(cove, env)
	if err != nil {
		return fmt.Errorf("InitializeDatabaseConfig: %w", err)
	}

	err = c.InitializeTwitchConfig(cove)
	if err != nil {
		return fmt.Errorf("InitializeTwitchConfig: %w", err)
	}

	err = c.InitializeCommandsConfig(cove)
	if err != nil {
		return fmt.Errorf("InitializeCommandsConfig: %w", err)
	}

	return nil
}
