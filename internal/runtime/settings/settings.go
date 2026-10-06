package settings

import (
	"sync"

	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
)

type Store struct {
	mu     sync.RWMutex
	twitch map[string]twitchdb.TwitchChannelSettings
	db     *twitchdb.Store
}

func NewSettings(db *twitchdb.Store) *Store {
	return &Store{
		twitch: make(map[string]twitchdb.TwitchChannelSettings),
		db:     db,
	}
}
