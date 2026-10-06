package letterboxd

import (
	"context"
	"fmt"

	"github.com/lsariol/lys/internal/app/dependencies"
)

func alreadyWatching(ctx context.Context, userID string, deps *dependencies.Deps) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM botsuite.letterboxd_feed_subscriptions WHERE user_id = $1)`

	var exists bool
	err := deps.DB.Pool.QueryRow(ctx, query, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("alreadyWatching: %w", err)
	}

	return exists, nil
}

func addSubscription(ctx context.Context, username, userID, lbUsername, feedURL, alertChannel string, deps *dependencies.Deps) error {
	query := `
		INSERT INTO botsuite.letterboxd_feed_subscriptions
			(username, user_id, letterboxd_username, feed_url, alert_channels)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := deps.DB.Pool.Exec(ctx, query, username, userID, lbUsername, feedURL, []string{alertChannel})
	if err != nil {
		return fmt.Errorf("addSubscription: %w", err)
	}

	return nil
}

func removeSubscription(ctx context.Context, userID string, deps *dependencies.Deps) (lbUsername string, found bool, err error) {
	query := `
		DELETE FROM botsuite.letterboxd_feed_subscriptions
		WHERE user_id = $1
		RETURNING letterboxd_username
	`

	err = deps.DB.Pool.QueryRow(ctx, query, userID).Scan(&lbUsername)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", false, nil
		}
		return "", false, fmt.Errorf("removeSubscription: %w", err)
	}

	return lbUsername, true, nil
}
