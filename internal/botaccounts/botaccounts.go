// Package botaccounts keeps the accounts Lys speaks as on each platform. The
// token values live in Cove; this stores the Cove key names.
package botaccounts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Account is one bot identity on one platform.
type Account struct {
	Platform        string
	ExternalID      string
	Login           string
	AccessTokenKey  string // Cove key holding the user access token
	RefreshTokenKey string // Cove key holding the refresh token
	Scopes          []string
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Default returns the platform's default bot account; found is false if it has none.
func (s *Store) Default(ctx context.Context, platform string) (a Account, found bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT platform, external_id, login::text, access_token_key, COALESCE(refresh_token_key, ''), scopes
		FROM lys.bot_accounts WHERE platform = $1 AND is_default`, platform).
		Scan(&a.Platform, &a.ExternalID, &a.Login, &a.AccessTokenKey, &a.RefreshTokenKey, &a.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("database: read the default %s bot account: %w", platform, err)
	}
	return a, true, nil
}

// SaveDefault records the account as just authorized and makes it its platform's default.
func (s *Store) SaveDefault(ctx context.Context, a Account) error {
	if a.Scopes == nil {
		a.Scopes = []string{}
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE lys.bot_accounts SET is_default = false
			WHERE platform = $1 AND external_id <> $2 AND is_default`, a.Platform, a.ExternalID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO lys.bot_accounts (platform, external_id, login, access_token_key, refresh_token_key, scopes, is_default, last_authorized_at)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, true, now())
			ON CONFLICT (platform, external_id) DO UPDATE
				SET login = EXCLUDED.login, access_token_key = EXCLUDED.access_token_key,
				    refresh_token_key = EXCLUDED.refresh_token_key, scopes = EXCLUDED.scopes,
				    is_default = true, last_authorized_at = now()`,
			a.Platform, a.ExternalID, a.Login, a.AccessTokenKey, a.RefreshTokenKey, a.Scopes)
		return err
	})
	if err != nil {
		return fmt.Errorf("database: save the %s bot account %q: %w", a.Platform, a.Login, err)
	}
	return nil
}
