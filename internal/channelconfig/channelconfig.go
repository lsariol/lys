// Package channelconfig keeps each channel's settings and IDs in memory,
// backed by the connections tables.
package channelconfig

import (
	"context"
	"sync"

	"github.com/lsariol/lys/internal/connections"
	"github.com/lsariol/lys/internal/core"
)

// Backend reads and writes the stored settings.
type Backend interface {
	List(ctx context.Context) ([]connections.Connection, error)
	SetPrefix(ctx context.Context, platform, externalID, prefix string) error
}

// Store implements core.ChannelConfig.
type Store struct {
	backend Backend

	mu       sync.RWMutex
	channels map[key]channel
}

var _ core.ChannelConfig = (*Store)(nil)

type key struct{ platform, id string }

type channel struct {
	prefix string
	ids    core.Connection
}

func New(backend Backend) *Store {
	return &Store{backend: backend, channels: make(map[key]channel)}
}

// Load reads every channel's settings.
func (s *Store) Load(ctx context.Context) error {
	all, err := s.backend.List(ctx)
	if err != nil {
		return err
	}
	channels := make(map[key]channel, len(all))
	for _, c := range all {
		channels[key{c.Platform, c.ExternalID}] = channel{prefix: c.Prefix, ids: core.Connection{ID: c.ID, CommunityID: c.CommunityID}}
	}
	s.mu.Lock()
	s.channels = channels
	s.mu.Unlock()
	return nil
}

func (s *Store) Prefix(platform, channelID string) string {
	s.mu.RLock()
	p := s.channels[key{platform, channelID}].prefix
	s.mu.RUnlock()
	if p == "" {
		return core.DefaultPrefix
	}
	return p
}

func (s *Store) Connection(platform, channelID string) (core.Connection, bool) {
	s.mu.RLock()
	c, ok := s.channels[key{platform, channelID}]
	s.mu.RUnlock()
	return c.ids, ok
}

func (s *Store) SetPrefix(ctx context.Context, platform, channelID, prefix string) error {
	if err := s.backend.SetPrefix(ctx, platform, channelID, prefix); err != nil {
		return err
	}
	k := key{platform, channelID}
	s.mu.Lock()
	c := s.channels[k]
	c.prefix = prefix
	s.channels[k] = c
	s.mu.Unlock()
	return nil
}
