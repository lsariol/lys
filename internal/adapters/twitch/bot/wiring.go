package bot

import (
	"context"
	"fmt"
)

// Initilize gets the bot's tokens and connects to EventSub.
func (c *TwitchClient) Initilize(ctx context.Context) error {

	if err := c.Auth.Initilize(ctx); err != nil {
		return fmt.Errorf("initilize auth: %w", err)
	}

	if err := c.EventSub.Initilize(ctx); err != nil {
		return fmt.Errorf("initilize eventsub: %w", err)
	}

	return nil
}
