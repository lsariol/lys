package letterboxd

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store keeps who's watching which Letterboxd account.
type Store interface {
	AlreadyWatching(ctx context.Context, userID string) (bool, error)
	AddSubscription(ctx context.Context, username, userID, lbUsername, feedURL, alertChannel string) error
	// RemoveSubscription returns the Letterboxd username it removed, or found=false.
	RemoveSubscription(ctx context.Context, userID string) (lbUsername string, found bool, err error)
}

// PGStore is the Store in lys.letterboxd_feed_subscriptions.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

func (s *PGStore) AlreadyWatching(ctx context.Context, userID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM lys.letterboxd_feed_subscriptions WHERE user_id = $1)`

	var exists bool
	err := s.pool.QueryRow(ctx, query, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("alreadyWatching: %w", err)
	}

	return exists, nil
}

func (s *PGStore) AddSubscription(ctx context.Context, username, userID, lbUsername, feedURL, alertChannel string) error {
	query := `
		INSERT INTO lys.letterboxd_feed_subscriptions
			(username, user_id, letterboxd_username, feed_url, alert_channels)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := s.pool.Exec(ctx, query, username, userID, lbUsername, feedURL, []string{alertChannel})
	if err != nil {
		return fmt.Errorf("addSubscription: %w", err)
	}

	return nil
}

func (s *PGStore) RemoveSubscription(ctx context.Context, userID string) (lbUsername string, found bool, err error) {
	query := `
		DELETE FROM lys.letterboxd_feed_subscriptions
		WHERE user_id = $1
		RETURNING letterboxd_username
	`

	err = s.pool.QueryRow(ctx, query, userID).Scan(&lbUsername)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("removeSubscription: %w", err)
	}

	return lbUsername, true, nil
}
