package bot

import (
	"context"
	"fmt"
)

func (c *TwitchClient) Initilize(ctx context.Context) error {

	if err := c.Settings.LoadAllTwitchChannelSettings(ctx); err != nil {
		return fmt.Errorf("load channels: %w", err)
	}

	if err := c.Auth.Initilize(ctx); err != nil {
		return fmt.Errorf("initilize auth: %w", err)
	}

	if err := c.Chat.Initilize(); err != nil {
		return fmt.Errorf("initilize chat: %w", err)
	}

	if err := c.EventSub.Initilize(ctx); err != nil {
		return fmt.Errorf("initilize eventsub: %w", err)
	}

	return nil
}
