// Package connections keeps the channels Lys is in: communities and their
// connections on each platform. Every change is one explicit transaction.
//
// Until the old tables are dropped, Twitch changes are also written to
// twitch_channels and twitch_settings, so rolling back to a release that
// reads them keeps every channel and prefix.
package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
)

// ErrNotFound means no connection matches.
var ErrNotFound = errors.New("no such channel")

// ErrAmbiguous means a name matches more than one connection.
var ErrAmbiguous = errors.New("ambiguous channel name")

// Connection is one community's presence on one platform.
type Connection struct {
	ID          int64
	CommunityID int64
	Platform    string
	ExternalID  string // the platform's ID for the channel
	Name        string
	Authorized  bool // the platform still honours the channel's grant
	// WasAuthorized: it has been authorized at some point; false for channels
	// only seen in feature data and never added.
	WasAuthorized bool
	Enabled       bool // the owner wants Lys there
	AdminBlocked  bool // Lys refuses to be there
	BlockReason   string
	Prefix        string
	ConnectedAt   time.Time
}

// Active reports whether Lys should be in the channel.
func (c Connection) Active() bool { return c.Authorized && c.Enabled && !c.AdminBlocked }

// Enrollment describes a channel joining Lys.
type Enrollment struct {
	Platform   string
	ExternalID string
	Name       string // display name
	Login      string // Twitch login; ignored elsewhere
}

type Store struct {
	pool *pgxpool.Pool
}

var _ core.Directory = (*Store)(nil)

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const selectConnection = `
	SELECT pc.id, pc.community_id, pc.platform, pc.external_id, pc.name, pc.authorized, pc.enabled,
	       pc.admin_blocked, COALESCE(pc.admin_block_reason, ''), COALESCE(cs.command_prefix, '!'), pc.connected_at,
	       pc.authorized OR EXISTS (SELECT 1 FROM lys.connection_events e WHERE e.connection_id = pc.id
	           AND (e.event_type IN ('reauthorized', 'authorization_revoked')
	                OR (e.event_type = 'enrolled' AND COALESCE(e.metadata->>'authorized', 'true') <> 'false')))
	FROM lys.platform_connections pc
	LEFT JOIN lys.connection_settings cs ON cs.connection_id = pc.id`

func scan(row pgx.Row) (Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.CommunityID, &c.Platform, &c.ExternalID, &c.Name, &c.Authorized, &c.Enabled,
		&c.AdminBlocked, &c.BlockReason, &c.Prefix, &c.ConnectedAt, &c.WasAuthorized)
	return c, err
}

// List returns every connection, active or not, by platform and name.
func (s *Store) List(ctx context.Context) ([]Connection, error) {
	rows, err := s.pool.Query(ctx, selectConnection+` ORDER BY pc.platform, lower(pc.name)`)
	if err != nil {
		return nil, fmt.Errorf("database: list channels: %w", err)
	}
	defer rows.Close()

	var out []Connection
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("database: list channels: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Channels lists the platform's active channels; it implements core.Directory.
func (s *Store) Channels(ctx context.Context, platform string) ([]core.Channel, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []core.Channel
	for _, c := range all {
		if c.Platform == platform && c.Active() {
			out = append(out, core.Channel{ID: c.ExternalID, Name: c.Name})
		}
	}
	return out, nil
}

// Find looks a connection up by its platform ID or, failing that, its name in
// any case. A name that several connections share is refused: use the ID.
func (s *Store) Find(ctx context.Context, platform, idOrName string) (Connection, error) {
	rows, err := s.pool.Query(ctx, selectConnection+`
		WHERE pc.platform = $1 AND (pc.external_id = $2 OR lower(pc.name) = lower($2))
		ORDER BY pc.external_id = $2 DESC, pc.id`, platform, idOrName)
	if err != nil {
		return Connection{}, fmt.Errorf("database: find channel %q: %w", idOrName, err)
	}
	defer rows.Close()
	var found []Connection
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return Connection{}, fmt.Errorf("database: find channel %q: %w", idOrName, err)
		}
		found = append(found, c)
	}
	if err := rows.Err(); err != nil {
		return Connection{}, fmt.Errorf("database: find channel %q: %w", idOrName, err)
	}

	switch {
	case len(found) == 0:
		return Connection{}, fmt.Errorf("%w: %s %q", ErrNotFound, platform, idOrName)
	case found[0].ExternalID == idOrName || len(found) == 1:
		return found[0], nil
	default:
		return Connection{}, fmt.Errorf("%w: %d %s channels are named %q; use the ID from lys channel list", ErrAmbiguous, len(found), platform, idOrName)
	}
}

// Enroll adds a channel, or turns an existing one back on. created reports
// whether it's new.
func (s *Store) Enroll(ctx context.Context, e Enrollment) (c Connection, created bool, err error) {
	if e.Platform == "" || e.ExternalID == "" || e.Name == "" {
		return Connection{}, false, errors.New("connections: an enrollment needs a platform, an ID and a name")
	}

	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id int64
		var wasAuthorized, wasEnabled bool
		err := tx.QueryRow(ctx, `SELECT id, authorized, enabled FROM lys.platform_connections
			WHERE platform = $1 AND external_id = $2 FOR UPDATE`, e.Platform, e.ExternalID).Scan(&id, &wasAuthorized, &wasEnabled)

		switch {
		case errors.Is(err, pgx.ErrNoRows):
			created = true
			var community int64
			if err := tx.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ($1) RETURNING id`, e.Name).Scan(&community); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO lys.platform_connections (community_id, platform, external_id, name)
				VALUES ($1, $2, $3, $4) RETURNING id`, community, e.Platform, e.ExternalID, e.Name).Scan(&id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO lys.connection_settings (connection_id) VALUES ($1)`, id); err != nil {
				return err
			}
			if err := event(ctx, tx, id, "enrolled", nil); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if _, err := tx.Exec(ctx, `UPDATE lys.platform_connections
				SET name = $2, authorized = true, enabled = true, connected_at = now() WHERE id = $1`, id, e.Name); err != nil {
				return err
			}
			if !wasAuthorized {
				if err := event(ctx, tx, id, "reauthorized", nil); err != nil {
					return err
				}
			}
			if !wasEnabled {
				if err := event(ctx, tx, id, "enabled", nil); err != nil {
					return err
				}
			}
		}

		if e.Platform == "twitch" {
			if err := enrollTwitch(ctx, tx, id, e); err != nil {
				return err
			}
		}
		if err := notify(ctx, tx, e.Platform); err != nil {
			return err
		}

		c, err = scan(tx.QueryRow(ctx, selectConnection+` WHERE pc.id = $1`, id))
		return err
	})
	if err != nil {
		return Connection{}, false, fmt.Errorf("database: enroll %s channel %q: %w", e.Platform, e.Name, err)
	}
	return c, created, nil
}

func enrollTwitch(ctx context.Context, tx pgx.Tx, id int64, e Enrollment) error {
	login := e.Login
	if login == "" {
		login = strings.ToLower(e.Name)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO lys.twitch_connection_details (connection_id, broadcaster_login) VALUES ($1, $2)
		ON CONFLICT (connection_id) DO UPDATE
			SET broadcaster_login = EXCLUDED.broadcaster_login,
			    times_joined = lys.twitch_connection_details.times_joined + 1`, id, login); err != nil {
		return err
	}
	// Twitch names are unique at any one time, so another ID holding this name
	// in the rollback copy is stale (a renamed channel): free the name for this one.
	if _, err := tx.Exec(ctx, `UPDATE lys.twitch_channels SET username = username || '~' || user_id
		WHERE username = $2 AND user_id <> $1`, e.ExternalID, e.Name); err != nil {
		return err
	}
	// Rollback copy. Its triggers create the twitch_settings row and log the grant.
	_, err := tx.Exec(ctx, `INSERT INTO lys.twitch_channels (user_id, username, access_token, refresh_token)
		VALUES ($1, $2, '', '')
		ON CONFLICT (user_id) DO UPDATE
			SET username = EXCLUDED.username, active = TRUE,
			    times_joined = lys.twitch_channels.times_joined + 1, last_joined = now()`, e.ExternalID, e.Name)
	return err
}

// SetBlocked blocks or unblocks a channel; reason is kept while it's blocked.
func (s *Store) SetBlocked(ctx context.Context, id int64, blocked bool, reason string) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var platform, externalID string
		err := tx.QueryRow(ctx, `UPDATE lys.platform_connections
			SET admin_blocked = $2, admin_block_reason = CASE WHEN $2 THEN NULLIF($3, '') END
			WHERE id = $1 RETURNING platform, external_id`, id, blocked, reason).Scan(&platform, &externalID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		kind := "admin_unblocked"
		if blocked {
			kind = "admin_blocked"
		}
		if err := event(ctx, tx, id, kind, map[string]string{"reason": reason}); err != nil {
			return err
		}

		if platform == "twitch" {
			// Rollback copy; its trigger logs the ban or unban.
			if _, err := tx.Exec(ctx, `UPDATE lys.twitch_channels SET banned = $2 WHERE user_id = $1`, externalID, blocked); err != nil {
				return err
			}
		}
		return notify(ctx, tx, platform)
	})
	if err != nil {
		return fmt.Errorf("database: block channel %d: %w", id, err)
	}
	return nil
}

// Disable records that the channel's owner turned Lys off. Lys leaves; the
// channel's settings and history stay, and Enroll turns it back on.
func (s *Store) Disable(ctx context.Context, platform, externalID string) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id int64
		var wasEnabled bool
		err := tx.QueryRow(ctx, `SELECT id, enabled FROM lys.platform_connections
			WHERE platform = $1 AND external_id = $2 FOR UPDATE`, platform, externalID).Scan(&id, &wasEnabled)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s %q", ErrNotFound, platform, externalID)
		}
		if err != nil || !wasEnabled {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE lys.platform_connections SET enabled = false WHERE id = $1`, id); err != nil {
			return err
		}
		if err := event(ctx, tx, id, "disabled", nil); err != nil {
			return err
		}
		if platform == "twitch" {
			// Rollback copy; its trigger logs the revoke.
			if _, err := tx.Exec(ctx, `UPDATE lys.twitch_channels SET active = false WHERE user_id = $1`, externalID); err != nil {
				return err
			}
		}
		return notify(ctx, tx, platform)
	})
	if err != nil {
		return fmt.Errorf("database: disable %s channel %q: %w", platform, externalID, err)
	}
	return nil
}

// Revoke records that Lys may no longer be in a channel (its grant was taken
// back, the bot was banned there, or the channel is gone). Lys leaves it;
// Enroll turns it back on. It implements core.Revoker.
func (s *Store) Revoke(ctx context.Context, platform, externalID, reason string) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, `UPDATE lys.platform_connections SET authorized = false
			WHERE platform = $1 AND external_id = $2 AND authorized RETURNING id`, platform, externalID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // unknown, or already revoked
		}
		if err != nil {
			return err
		}
		if err := event(ctx, tx, id, "authorization_revoked", map[string]string{"reason": reason}); err != nil {
			return err
		}
		if platform == "twitch" {
			// Rollback copy; its trigger logs the revoke.
			if _, err := tx.Exec(ctx, `UPDATE lys.twitch_channels SET active = false WHERE user_id = $1`, externalID); err != nil {
				return err
			}
		}
		return notify(ctx, tx, platform)
	})
	if err != nil {
		return fmt.Errorf("database: revoke %s channel %q: %w", platform, externalID, err)
	}
	return nil
}

// SetPrefix changes a channel's command prefix.
func (s *Store) SetPrefix(ctx context.Context, platform, externalID, prefix string) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE lys.connection_settings SET command_prefix = $3
			WHERE connection_id = (SELECT id FROM lys.platform_connections WHERE platform = $1 AND external_id = $2)`,
			platform, externalID, prefix)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: %s %q", ErrNotFound, platform, externalID)
		}
		if platform == "twitch" {
			// Rollback copy.
			if _, err := tx.Exec(ctx, `UPDATE lys.twitch_settings SET command_prefix = $2 WHERE user_id = $1`, externalID, prefix); err != nil {
				return err
			}
		}
		return notify(ctx, tx, platform)
	})
	if err != nil {
		return fmt.Errorf("database: set the prefix of %s channel %q: %w", platform, externalID, err)
	}
	return nil
}

func event(ctx context.Context, tx pgx.Tx, connectionID int64, kind string, metadata map[string]string) error {
	if metadata == nil {
		metadata = map[string]string{}
	}
	_, err := tx.Exec(ctx, `INSERT INTO lys.connection_events (connection_id, event_type, metadata) VALUES ($1, $2, $3)`,
		connectionID, kind, metadata)
	return err
}
