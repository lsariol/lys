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

	if err := c.Auth.RefreshUserAccessToken(ctx); err != nil {
		return fmt.Errorf("refresh user access tokens: %w", err)
	}

	// Not sure if this is needed anymore. I think this was fully unnecessary to being with
	// if err := c.Auth.RefreshAppAccessToken(ctx); err != nil {
	// 	return fmt.Errorf("refresh app access tokens: %w", err)
	// }

	if err := c.Chat.Initilize(); err != nil {
		return fmt.Errorf("initilize chat: %w", err)
	}

	if err := c.EventSub.Initilize(ctx); err != nil {
		return fmt.Errorf("initilize eventsub: %w", err)
	}

	return nil
}
