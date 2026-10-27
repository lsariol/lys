package letterboxd

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Subscription is one viewer's Letterboxd feed, posted to one channel.
type Subscription struct {
	PlatformUserID int64
	ConnectionID   int64
	FeedURL        string
	LetterboxdName string

	// For the old letterboxd_feed_subscriptions row.
	Username     string
	UserID       string
	AlertChannel string // "platform:channelID"
}

// Store keeps who's watching which Letterboxd account.
type Store interface {
	AlreadyWatching(ctx context.Context, platformUserID int64) (bool, error)
	AddSubscription(ctx context.Context, s Subscription) error
	// RemoveSubscription returns the Letterboxd username it removed, or found=false.
	RemoveSubscription(ctx context.Context, platformUserID int64, userID string) (lbUsername string, found bool, err error)
}

// PGStore is the Store in lys.feed_subscriptions. Until LetterboxdWatcher is
// retired and the old table dropped, it also keeps letterboxd_feed_subscriptions
// current, in the same transaction.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

func (s *PGStore) AlreadyWatching(ctx context.Context, platformUserID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM lys.feed_subscriptions WHERE kind = 'letterboxd' AND platform_user_id = $1)`

	var exists bool
	if err := s.pool.QueryRow(ctx, query, platformUserID).Scan(&exists); err != nil {
		return false, fmt.Errorf("database: check Letterboxd subscription: %w", err)
	}
	return exists, nil
}

func (s *PGStore) AddSubscription(ctx context.Context, sub Subscription) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO lys.feed_subscriptions (kind, feed_url, feed_name, connection_id, platform_user_id)
			VALUES ('letterboxd', $1, $2, $3, $4)`,
			sub.FeedURL, sub.LetterboxdName, sub.ConnectionID, sub.PlatformUserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO lys.letterboxd_feed_subscriptions (username, user_id, letterboxd_username, feed_url, alert_channels)
			VALUES ($1, $2, $3, $4, $5)`,
			sub.Username, sub.UserID, sub.LetterboxdName, sub.FeedURL, []string{sub.AlertChannel})
		return err
	})
	if err != nil {
		return fmt.Errorf("database: add Letterboxd subscription: %w", err)
	}
	return nil
}

func (s *PGStore) RemoveSubscription(ctx context.Context, platformUserID int64, userID string) (lbUsername string, found bool, err error) {
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			DELETE FROM lys.feed_subscriptions
			WHERE kind = 'letterboxd' AND platform_user_id = $1
			RETURNING feed_name`, platformUserID).Scan(&lbUsername)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		default:
			found = true
		}
		_, err = tx.Exec(ctx, `DELETE FROM lys.letterboxd_feed_subscriptions WHERE user_id = $1`, userID)
		return err
	})
	if err != nil {
		return "", false, fmt.Errorf("database: remove Letterboxd subscription: %w", err)
	}
	return lbUsername, found, nil
}
