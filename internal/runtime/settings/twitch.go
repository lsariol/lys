package settings

import (
	"context"
	"fmt"

	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
)

func (s *Store) LoadAllTwitchChannelSettings(ctx context.Context) error {

	settings, err := s.db.GetPerChannelSettings(ctx)
	if err != nil {
		return fmt.Errorf("GetPerChannelSettings: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.twitch = settings

	return nil
}

func (s *Store) GetTwitchChannelSettings(userID string) (twitchdb.TwitchChannelSettings, bool) {

	s.mu.RLock()
	channelSettings, ok := s.twitch[userID]
	s.mu.RUnlock()
	return channelSettings, ok
}

func (s *Store) UpdateTwitchChannelPrefixSetting(ctx context.Context, channelID string, newPrefix string) error {

	err := s.db.UpdateChannelPrefix(ctx, channelID, newPrefix)
	if err != nil {
		return fmt.Errorf("failed to update prefix: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.twitch[channelID]
	cs.CommandPrefix = newPrefix
	s.twitch[channelID] = cs

	return nil
}
