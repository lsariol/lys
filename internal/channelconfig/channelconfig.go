// Package channelconfig keeps each channel's settings in memory, backed by
// the connections tables.
package channelconfig

import (
	"context"
	"sync"

	"github.com/lsariol/lys/internal/core"
)

// Backend reads and writes the stored settings.
type Backend interface {
	Prefixes(ctx context.Context) (map[[2]string]string, error)
	SetPrefix(ctx context.Context, platform, externalID, prefix string) error
}

// Store implements core.ChannelConfig.
type Store struct {
	backend Backend

	mu       sync.RWMutex
	prefixes map[[2]string]string // platform, channel ID -> prefix
}

var _ core.ChannelConfig = (*Store)(nil)

func New(backend Backend) *Store {
	return &Store{backend: backend, prefixes: make(map[[2]string]string)}
}

// Load reads every channel's settings.
func (s *Store) Load(ctx context.Context) error {
	prefixes, err := s.backend.Prefixes(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.prefixes = prefixes
	s.mu.Unlock()
	return nil
}

func (s *Store) Prefix(platform, channelID string) string {
	s.mu.RLock()
	p := s.prefixes[[2]string{platform, channelID}]
	s.mu.RUnlock()
	if p == "" {
		return core.DefaultPrefix
	}
	return p
}

func (s *Store) SetPrefix(ctx context.Context, platform, channelID, prefix string) error {
	if err := s.backend.SetPrefix(ctx, platform, channelID, prefix); err != nil {
		return err
	}
	s.mu.Lock()
	s.prefixes[[2]string{platform, channelID}] = prefix
	s.mu.Unlock()
	return nil
}
