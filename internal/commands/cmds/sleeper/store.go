package sleeper

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// The leagues are keyed by community and their members by platform user.

func FindLeagueLinkConflict(leagueID string, communityID int64, ctx context.Context, sl *Sleeper) (*SleeperLeagueLink, error) {

	const query string = `
		SELECT community_id, sleeper_league_id
		FROM lys.sleeper_leagues
		WHERE community_id = $1
			OR sleeper_league_id = $2
		LIMIT 1`

	var link SleeperLeagueLink
	err := sl.db.QueryRow(ctx, query, communityID, leagueID).Scan(&link.CommunityID, &link.SleeperLeagueID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &link, nil
}

func addLeagueLink(league *api.League, communityID int64, ctx context.Context, sl *Sleeper) (error, bool) {

	const query = `
	INSERT INTO lys.sleeper_leagues(
	sleeper_league_id,
	league_name,
	season,
	community_id
	)
	VALUES ($1, $2, $3, $4)`

	_, err := sl.db.Exec(ctx, query, league.LeagueID, league.Name, league.Season, communityID)
	if err != nil {
		log.Println(err)
		return err, false
	}
	return nil, true
}

// getLeague returns the community's league, or nil if it has none.
func getLeague(ctx context.Context, communityID int64, sl *Sleeper) (*SleeperLeague, error) {
	if communityID == 0 {
		return nil, nil
	}

	const query string = `
		SELECT 
		id,
		community_id,
		sleeper_league_id,
		COALESCE(league_name, ''),
		COALESCE(season, '')
		FROM lys.sleeper_leagues
		WHERE community_id = $1
		ORDER BY created_at
		LIMIT 1
		`

	var league SleeperLeague
	err := sl.db.QueryRow(ctx, query, communityID).Scan(&league.ID, &league.CommunityID, &league.SleeperLeagueID, &league.LeagueName, &league.Season)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &league, nil
}

// selectMember reads a league member with their current chat name.
const selectMember = `
		SELECT
		c.league_id,
		c.platform_user_id,
		COALESCE(pu.display_name, c.twitch_username, ''),
		c.sleeper_user_id,
		c.sleeper_username,
		c.sleeper_roster_id
		FROM lys.sleeper_connections c
		JOIN lys.platform_users pu ON pu.id = c.platform_user_id`

func getConnectionByUserID(ctx context.Context, leagueID uuid.UUID, platformUserID int64, sl *Sleeper) (*SleeperConnection, error) {

	var connection SleeperConnection
	err := sl.db.QueryRow(ctx, selectMember+`
		WHERE c.league_id = $1
			AND c.platform_user_id = $2`, leagueID, platformUserID).Scan(
		&connection.LeagueID,
		&connection.PlatformUserID,
		&connection.DisplayName,
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

// getConnectionByUsername finds a member by their current chat name.
func getConnectionByUsername(ctx context.Context, leagueID uuid.UUID, name string, sl *Sleeper) (*SleeperConnection, error) {

	name = strings.TrimPrefix(strings.TrimSpace(name), "@")
	if name == "" {
		return nil, nil
	}

	rows, err := sl.db.Query(ctx, selectMember+`
		WHERE c.league_id = $1
			AND lower(pu.display_name) = lower($2)`, leagueID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connection *SleeperConnection

	for rows.Next() {
		if connection != nil {
			return nil, fmt.Errorf("multiple Sleeper members are called %q", name)
		}

		connection = &SleeperConnection{}
		err := rows.Scan(
			&connection.LeagueID,
			&connection.PlatformUserID,
			&connection.DisplayName,
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

func linkUser(ctx context.Context, leagueID uuid.UUID, platformUserID int64, sleeperUserID string, sleeperUsername string, sleeperRosterID int, sl *Sleeper) error {
	if platformUserID == 0 {
		return errors.New("sleeper: the member's identity couldn't be looked up, so they can't be linked")
	}

	const query = `
	INSERT INTO lys.sleeper_connections(
	league_id,
	sleeper_user_id,
	sleeper_username,
	sleeper_roster_id,
	platform_user_id
	)
	VALUES ($1, $2, $3, $4, $5)`

	_, err := sl.db.Exec(ctx, query, leagueID, sleeperUserID, sleeperUsername, sleeperRosterID, platformUserID)

	return err
}

func removeLeagueLink(ctx context.Context, leagueID uuid.UUID, sl *Sleeper) error {

	const userQuery = `
		DELETE FROM lys.sleeper_connections
		WHERE league_id = $1`

	const leagueQuery = `
		DELETE FROM lys.sleeper_leagues
		WHERE id = $1`

	tx, err := sl.db.Begin(ctx)
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

func removeUser(ctx context.Context, leagueID uuid.UUID, platformUserID int64, sl *Sleeper) (bool, error) {

	const query = `
		DELETE FROM lys.sleeper_connections
		WHERE league_id = $1
			AND platform_user_id = $2`

	result, err := sl.db.Exec(ctx, query, leagueID, platformUserID)
	if err != nil {
		return false, err
	}

	return result.RowsAffected() > 0, nil
}
