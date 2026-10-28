package config

import (
	"fmt"

	"github.com/lsariol/coveclient"
)

type CommandsConfig struct {
	ChatGPTKey string
}

func NewCommandsConfig() *CommandsConfig {
	return &CommandsConfig{}
}

func (c *Config) InitializeCommandsConfig(cove *coveclient.Client) error {

	chatGPTKey, err := cove.GetSecret("LYS_OPENAI_API_KEY")
	if err != nil {
		return fmt.Errorf("get secret LYS_OPENAI_API_KEY: %w", err)
	}

	c.Commands.ChatGPTKey = chatGPTKey

	return nil
}
