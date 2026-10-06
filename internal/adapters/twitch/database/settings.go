package twitchdb

import (
	"context"
	"fmt"
)

func (s *Store) UpdateChannelPrefix(ctx context.Context, channelID string, newPrefix string) error {

	const query = `
	UPDATE lys.twitch_settings
	SET command_prefix = $2
	WHERE user_id = $1;
	`

	_, err := s.pool.Exec(ctx, query, channelID, newPrefix)
	if err != nil {
		return fmt.Errorf("update prefix: %w", err)
	}

	return nil
}
