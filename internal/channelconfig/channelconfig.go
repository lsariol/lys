// Package channelconfig keeps each channel's settings in memory, backed by
// the database. For now only Twitch channels have stored settings.
package channelconfig

import (
	"context"
	"fmt"
	"sync"

	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/core"
)

// Store implements core.ChannelConfig.
type Store struct {
	twitch *twitchdb.Store

	mu       sync.RWMutex
	prefixes map[string]string // twitch channel ID -> prefix
}

var _ core.ChannelConfig = (*Store)(nil)

func New(twitch *twitchdb.Store) *Store {
	return &Store{twitch: twitch, prefixes: make(map[string]string)}
}

// Load reads every active channel's settings.
func (s *Store) Load(ctx context.Context) error {
	settings, err := s.twitch.GetPerChannelSettings(ctx)
	if err != nil {
		return fmt.Errorf("database: load channel settings: %w", err)
	}

	prefixes := make(map[string]string, len(settings))
	for id, cs := range settings {
		prefixes[id] = cs.CommandPrefix
	}

	s.mu.Lock()
	s.prefixes = prefixes
	s.mu.Unlock()
	return nil
}

func (s *Store) Prefix(platform, channelID string) string {
	if platform != "twitch" {
		return core.DefaultPrefix
	}
	s.mu.RLock()
	p := s.prefixes[channelID]
	s.mu.RUnlock()
	if p == "" {
		return core.DefaultPrefix
	}
	return p
}

func (s *Store) SetPrefix(ctx context.Context, platform, channelID, prefix string) error {
	if platform != "twitch" {
		return fmt.Errorf("channelconfig: %s channels can't store settings yet", platform)
	}
	if err := s.twitch.UpdateChannelPrefix(ctx, channelID, prefix); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	s.mu.Lock()
	s.prefixes[channelID] = prefix
	s.mu.Unlock()
	return nil
}
