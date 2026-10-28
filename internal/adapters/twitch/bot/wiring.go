package bot

import (
	"context"
	"fmt"
	"log"

	"github.com/lsariol/lys/internal/botaccounts"
)

// Initialize gets the bot's tokens and connects to EventSub.
func (c *TwitchClient) Initialize(ctx context.Context) error {

	if err := c.Auth.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize auth: %w", err)
	}

	bot := c.Auth.Bot
	if err := c.Bots.SaveDefault(ctx, botaccounts.Account{
		Platform:        Platform,
		ExternalID:      bot.UserId,
		Login:           bot.Login,
		AccessTokenKey:  c.Config.AccessTokenKey,
		RefreshTokenKey: c.Config.RefreshTokenKey,
		Scopes:          bot.Scopes,
	}); err != nil {
		return err
	}

	if err := c.Chat.LoadRoles(ctx); err != nil {
		log.Printf("[Twitch] %v; sending as if the bot weren't a moderator anywhere until it speaks", err)
	}

	if err := c.EventSub.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize eventsub: %w", err)
	}

	return nil
}
