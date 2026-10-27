package oneuppuzzle

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// store holds OneUpPuzzle games in lys.game_events, keyed by connection and
// platform user. The old user/channel columns are still written for rollback.
type store struct {
	pool *pgxpool.Pool
}

func (s *store) storeGame(ctx context.Context, game OneUpGame, flagged bool) error {

	query := `
	INSERT INTO lys.game_events
		(user_id, username, channel_id, channel_name, game_code, raw_message, details, is_flagged, connection_id, platform_user_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, 0), NULLIF($10, 0));
	`

	b, err := json.Marshal(game.Details)
	if err != nil {
		return fmt.Errorf("encode details: %w", err)
	}

	_, err = s.pool.Exec(ctx, query, game.UserID, game.Username, game.ChannelID, game.ChannelName, game.GameCode, game.RawMessage, b, flagged,
		game.ConnectionID, game.PlatformUserID)
	if err != nil {
		return fmt.Errorf("database: store game: %w", err)
	}

	return nil
}

// currentName is a game's player under their current display name.
const currentName = `COALESCE(pu.display_name, g.username::text)`

func (s *store) puzzleStatsByChannel(ctx context.Context, connectionID int64) (ChannelProfile, bool, error) {
	var channelProfile ChannelProfile

	query := `
	SELECT
		COUNT(*)                                 			AS total_games,
		MIN((details->>'time_seconds')::int)     			AS fastest_time,
		MAX((details->>'time_seconds')::int)     			AS slowest_time,
		CAST(AVG((details->>'time_seconds')::int) AS int)	AS avg_time,
		(
			SELECT ` + currentName + `
			FROM lys.game_events g LEFT JOIN lys.platform_users pu ON pu.id = g.platform_user_id
			WHERE g.connection_id = $1 AND g.game_code = 'oneuppuzzle' AND g.is_flagged = FALSE
			ORDER BY (g.details->>'time_seconds')::int ASC
			LIMIT 1
		) AS fastest_user,
		(
			SELECT ` + currentName + `
			FROM lys.game_events g LEFT JOIN lys.platform_users pu ON pu.id = g.platform_user_id
			WHERE g.connection_id = $1 AND g.game_code = 'oneuppuzzle' AND g.is_flagged = FALSE
			ORDER BY (g.details->>'time_seconds')::int DESC
			LIMIT 1
		) AS slowest_user
	FROM lys.game_events
	WHERE connection_id = $1
	AND game_code  = 'oneuppuzzle'
	AND is_flagged = FALSE;
	`
	err := s.pool.QueryRow(ctx, query, connectionID).Scan(
		&channelProfile.GamesCompleted,
		&channelProfile.Completions.FastestTime,
		&channelProfile.Completions.SlowestTime,
		&channelProfile.Completions.AverageTime,
		&channelProfile.FastestUser,
		&channelProfile.SlowestUser,
	)

	if err != nil {
		return channelProfile, false, fmt.Errorf("database: channel stats: %w", err)
	}

	if channelProfile.Completions.FastestTime == nil {
		return channelProfile, false, nil
	}

	return channelProfile, true, nil
}

// puzzleStatsByUser finds the player by their current display name on the platform.
func (s *store) puzzleStatsByUser(ctx context.Context, platform, name string) (UserProfile, bool, error) {
	var userProfile UserProfile

	query := `
	SELECT
		COUNT(*)                                 			AS total_games,
		MIN((details->>'time_seconds')::int)     			AS fastest,
		MAX((details->>'time_seconds')::int)     			AS slowest,
		CAST(AVG((details->>'time_seconds')::int) AS int)	AS avg_time
	FROM lys.game_events
	WHERE platform_user_id IN (
		SELECT id FROM lys.platform_users WHERE platform = $1 AND lower(display_name) = lower($2)
	)
	AND game_code = 'oneuppuzzle'
	AND is_flagged = FALSE;
	`

	err := s.pool.QueryRow(ctx, query, platform, name).Scan(
		&userProfile.GamesCompleted,
		&userProfile.Completions.FastestTime,
		&userProfile.Completions.SlowestTime,
		&userProfile.Completions.AverageTime,
	)

	userProfile.UserName = name

	if err != nil {
		return userProfile, false, fmt.Errorf("database: user stats: %w", err)
	}

	if userProfile.Completions.FastestTime == nil {
		return userProfile, false, nil
	}

	return userProfile, true, nil
}

func (s *store) puzzleStatsByPuzzleID(ctx context.Context, gameID int) (GameProfile, bool, error) {
	var gameProfile GameProfile

	query := `
	SELECT
		COUNT(*)                                 			AS total_games,
		MIN((details->>'time_seconds')::int)     			AS fastest_time,
		MAX((details->>'time_seconds')::int)     			AS slowest_time,
		CAST(AVG((details->>'time_seconds')::int) AS int)	AS avg_time,
		(
			SELECT ` + currentName + `
			FROM lys.game_events g LEFT JOIN lys.platform_users pu ON pu.id = g.platform_user_id
			WHERE (g.details->'puzzle_id')::INT = $1 AND g.game_code = 'oneuppuzzle' AND g.is_flagged = FALSE
			ORDER BY (g.details->>'time_seconds')::int ASC
			LIMIT 1
		) AS fastest_user,
		(
			SELECT ` + currentName + `
			FROM lys.game_events g LEFT JOIN lys.platform_users pu ON pu.id = g.platform_user_id
			WHERE (g.details->'puzzle_id')::INT = $1 AND g.game_code = 'oneuppuzzle' AND g.is_flagged = FALSE
			ORDER BY (g.details->>'time_seconds')::int DESC
			LIMIT 1
		) AS slowest_user
	FROM lys.game_events
	WHERE (details->'puzzle_id')::INT = $1
	AND game_code  = 'oneuppuzzle'
	AND is_flagged = FALSE;
	`

	err := s.pool.QueryRow(ctx, query, gameID).Scan(
		&gameProfile.GamesCompleted,
		&gameProfile.Completions.FastestTime,
		&gameProfile.Completions.SlowestTime,
		&gameProfile.Completions.AverageTime,
		&gameProfile.FastestUser,
		&gameProfile.SlowestUser,
	)

	gameProfile.GameID = gameID

	if err != nil {
		return gameProfile, false, fmt.Errorf("database: puzzle stats: %w", err)
	}

	if gameProfile.Completions.FastestTime == nil {
		return gameProfile, false, nil
	}

	return gameProfile, true, nil
}

func (s *store) isPuzzleRecorded(ctx context.Context, platformUserID int64, gameID int) (bool, error) {
	var recorded bool

	query := `
		SELECT EXISTS (
			SELECT 1
			FROM lys.game_events
			WHERE platform_user_id = $1
			AND game_code = 'oneuppuzzle'
			AND (details->>'puzzle_id')::int = $2
		);
	`

	err := s.pool.QueryRow(ctx, query, platformUserID, gameID).Scan(&recorded)
	if err != nil {
		return false, fmt.Errorf("database: check recorded game: %w", err)
	}

	return recorded, nil
}
