package sleeper

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lsariol/lys/internal/app/dependencies"
)

func FindLeagueLinkConflict(leagueID string, channelID string, ctx context.Context, deps *dependencies.Deps) (*SleeperLeagueLink, error) {

	const query string = `
		SELECT twitch_channel_id, sleeper_league_id
		FROM lys.sleeper_leagues
		WHERE twitch_channel_id = $1
			OR sleeper_league_id = $2
		LIMIT 1`

	var link SleeperLeagueLink
	err := deps.DB.Pool.QueryRow(ctx, query, channelID, leagueID).Scan(&link.TwitchChannelID, &link.SleeperLeagueID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &link, nil
}

func addLeagueLink(league *League, channelID string, ctx context.Context, deps *dependencies.Deps) (error, bool) {

	const query = `
	INSERT INTO lys.sleeper_leagues(
	twitch_channel_id,
	sleeper_league_id,
	league_name,
	season
	)
	VALUES ($1, $2, $3, $4)`

	_, err := deps.DB.Pool.Exec(ctx, query, channelID, league.LeagueID, league.Name, league.Season)
	if err != nil {
		log.Println(err)
		return err, false
	}
	return nil, true
}

func getLeagueByChannel(ctx context.Context, channelID string, deps *dependencies.Deps) (*SleeperLeague, error) {
	const query string = `
		SELECT 
		id,
		twitch_channel_id,
		sleeper_league_id,
		league_name,
		season
		FROM lys.sleeper_leagues
		WHERE twitch_channel_id = $1
		`

	var league SleeperLeague
	err := deps.DB.Pool.QueryRow(ctx, query, channelID).Scan(&league.ID, &league.TwitchChannelID, &league.SleeperLeagueID, &league.LeagueName, &league.Season)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &league, nil
}

func getConnectionByUserID(ctx context.Context, leagueID uuid.UUID, twitchUserID string, deps *dependencies.Deps) (*SleeperConnection, error) {

	const query = `
		SELECT
		league_id,
		twitch_user_id,
		COALESCE(twitch_username, ''),
		sleeper_user_id,
		sleeper_username,
		sleeper_roster_id
		FROM lys.sleeper_connections
		WHERE league_id = $1
			AND twitch_user_id = $2`

	var connection SleeperConnection
	err := deps.DB.Pool.QueryRow(ctx, query, leagueID, twitchUserID).Scan(
		&connection.LeagueID,
		&connection.TwitchUserID,
		&connection.TwitchUsername,
		&connection.SleeperUserID,
		&connection.SleeperUsername,
		&connection.SleeperRosterID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &connection, nil
}

func getConnectionByUsername(ctx context.Context, leagueID uuid.UUID, twitchUsername string, deps *dependencies.Deps) (*SleeperConnection, error) {

	twitchUsername = strings.TrimPrefix(strings.TrimSpace(twitchUsername), "@")
	if twitchUsername == "" {
		return nil, nil
	}

	const query = `
		SELECT
		league_id,
		twitch_user_id,
		COALESCE(twitch_username, ''),
		sleeper_user_id,
		sleeper_username,
		sleeper_roster_id
		FROM lys.sleeper_connections
		WHERE league_id = $1
			AND LOWER(twitch_username) = LOWER($2)`

	rows, err := deps.DB.Pool.Query(ctx, query, leagueID, twitchUsername)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connection *SleeperConnection

	for rows.Next() {
		if connection != nil {
			return nil, fmt.Errorf("multiple Sleeper connections match Twitch username %s", twitchUsername)
		}

		connection = &SleeperConnection{}
		err := rows.Scan(
			&connection.LeagueID,
			&connection.TwitchUserID,
			&connection.TwitchUsername,
			&connection.SleeperUserID,
			&connection.SleeperUsername,
			&connection.SleeperRosterID,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return connection, nil
}

func linkUser(ctx context.Context, leagueID uuid.UUID, twitchUserID string, twitchUsername string, sleeperUserID string, sleeperUsername string, sleeperRosterID int, deps *dependencies.Deps) error {

	const query = `
	INSERT INTO lys.sleeper_connections(
	league_id,
	twitch_user_id,
	twitch_username,
	sleeper_user_id,
	sleeper_username,
	sleeper_roster_id
	)
	VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := deps.DB.Pool.Exec(ctx, query, leagueID, twitchUserID, twitchUsername, sleeperUserID, sleeperUsername, sleeperRosterID)

	return err
}

func updateTwitchUsername(ctx context.Context, twitchUserID string, twitchUsername string, deps *dependencies.Deps) error {

	if twitchUserID == "" || twitchUsername == "" {
		return nil
	}

	const query = `
		UPDATE lys.sleeper_connections
		SET twitch_username = $2
		WHERE twitch_user_id = $1
			AND twitch_username IS DISTINCT FROM $2`

	_, err := deps.DB.Pool.Exec(ctx, query, twitchUserID, twitchUsername)

	return err
}

func removeLeagueLink(ctx context.Context, leagueID uuid.UUID, deps *dependencies.Deps) error {

	const userQuery = `
		DELETE FROM lys.sleeper_connections
		WHERE league_id = $1`

	const leagueQuery = `
		DELETE FROM lys.sleeper_leagues
		WHERE id = $1`

	tx, err := deps.DB.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, userQuery, leagueID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, leagueQuery, leagueID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func removeUser(ctx context.Context, leagueID uuid.UUID, twitchUserID string, deps *dependencies.Deps) (bool, error) {

	const query = `
		DELETE FROM lys.sleeper_connections
		WHERE league_id = $1
			AND twitch_user_id = $2`

	result, err := deps.DB.Pool.Exec(ctx, query, leagueID, twitchUserID)
	if err != nil {
		return false, err
	}

	return result.RowsAffected() > 0, nil
}

func registerUser(ctx context.Context, username, userID, lbUsername, feedURL, alertChannel string, deps *dependencies.Deps) error {
	query := `
		INSERT INTO lys.letterboxd_feed_subscriptions
			(username, user_id, letterboxd_username, feed_url, alert_channels)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := deps.DB.Pool.Exec(ctx, query, username, userID, lbUsername, feedURL, []string{alertChannel})
	if err != nil {
		return fmt.Errorf("addSubscription: %w", err)
	}

	return nil
}

func unregisterUser(ctx context.Context, userID string, deps *dependencies.Deps) (lbUsername string, found bool, err error) {
	query := `
		DELETE FROM lys.letterboxd_feed_subscriptions
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
