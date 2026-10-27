// Package identity records the people Lys sees on each platform and finds
// their Lys identity.
package identity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
)

// refreshAfter is how long a looked-up identity is trusted before Lys checks
// the database again (to see a new display name or a newly linked account).
const refreshAfter = 10 * time.Minute

// maxCached caps the cache; when it's full it starts over.
const maxCached = 10000

// Store implements core.Identities on lys.platform_users.
type Store struct {
	pool *pgxpool.Pool

	mu    sync.Mutex
	cache map[key]entry
}

var _ core.Identities = (*Store)(nil)

type key struct{ platform, id string }

type entry struct {
	identity core.Identity
	name     string
	checked  time.Time
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, cache: make(map[key]entry)}
}

// Resolve returns the user's identity, adding them to platform_users the first
// time and keeping their display name current.
func (s *Store) Resolve(ctx context.Context, platform string, u core.User) (core.Identity, error) {
	if u.ID == "" {
		return core.Identity{}, errors.New("identity: the message has no user ID")
	}
	k := key{platform, u.ID}

	s.mu.Lock()
	e, ok := s.cache[k]
	s.mu.Unlock()
	if ok && e.name == u.Name && time.Since(e.checked) < refreshAfter {
		return e.identity, nil
	}

	const query = `
	INSERT INTO lys.platform_users (platform, external_id, display_name)
	VALUES ($1, $2, NULLIF($3, ''))
	ON CONFLICT (platform, external_id) DO UPDATE
		SET display_name = COALESCE(EXCLUDED.display_name, lys.platform_users.display_name),
		    last_seen_at = now()
	RETURNING id, COALESCE(account_id, 0)`

	var id core.Identity
	if err := s.pool.QueryRow(ctx, query, platform, u.ID, u.Name).Scan(&id.PlatformUserID, &id.AccountID); err != nil {
		return core.Identity{}, fmt.Errorf("database: record platform user %s %q: %w", platform, u.ID, err)
	}

	s.mu.Lock()
	if len(s.cache) >= maxCached {
		clear(s.cache)
	}
	s.cache[k] = entry{identity: id, name: u.Name, checked: time.Now()}
	s.mu.Unlock()
	return id, nil
}
