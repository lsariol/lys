package oneuppuzzle

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// store holds OneUpPuzzle games in lys.game_events.
type store struct {
	pool *pgxpool.Pool
}

func (s *store) storeGame(ctx context.Context, game OneUpGame) error {

	query := `
	INSERT INTO lys.game_events (user_id, username, channel_id, channel_name, game_code, raw_message, details)
	VALUES ($1, $2, $3, $4, $5, $6, $7);
	`

	b, _ := json.Marshal(game.Details)

	_, err := s.pool.Exec(ctx, query, game.UserID, game.Username, game.ChannelID, game.ChannelName, game.GameCode, game.RawMessage, b)
	if err != nil {
		return fmt.Errorf("exec query: %w", err)
	}

	return nil
}

func (s *store) storeFlaggedGame(ctx context.Context, game OneUpGame) error {

	query := `
	INSERT INTO lys.game_events (user_id, username, channel_id, channel_name, game_code, raw_message, details, is_flagged)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`

	b, _ := json.Marshal(game.Details)

	_, err := s.pool.Exec(ctx, query, game.UserID, game.Username, game.ChannelID, game.ChannelName, game.GameCode, game.RawMessage, b, true)
	if err != nil {
		return fmt.Errorf("exec query: %w", err)
	}

	return nil
}

func (s *store) puzzleStatsByChannel(ctx context.Context, channelID string) (ChannelProfile, bool, error) {
	var channelProfile ChannelProfile

	query := `
	SELECT
		-- stats
		COUNT(*)                                 			AS total_games,
		MIN((details->>'time_seconds')::int)     			AS fastest_time,
		MAX((details->>'time_seconds')::int)     			AS slowest_time,
		CAST(AVG((details->>'time_seconds')::int) AS int)	AS avg_time,

		-- fastest username
		(
			SELECT username
			FROM lys.game_events
			WHERE channel_id = $1
			AND game_code  = 'oneuppuzzle'
			AND is_flagged = FALSE
			ORDER BY (details->>'time_seconds')::int ASC
			LIMIT 1
		) AS fastest_user,

		-- slowest username
		(
			SELECT username
			FROM lys.game_events
			WHERE channel_id = $1
			AND game_code  = 'oneuppuzzle'
			AND is_flagged = FALSE
			ORDER BY (details->>'time_seconds')::int DESC
			LIMIT 1
		) AS slowest_user

	FROM lys.game_events
	WHERE channel_id = $1
	AND game_code  = 'oneuppuzzle'
	AND is_flagged = FALSE;
	`
	err := s.pool.QueryRow(ctx, query, channelID).Scan(
		&channelProfile.GamesCompleted,
		&channelProfile.Completions.FastestTime,
		&channelProfile.Completions.SlowestTime,
		&channelProfile.Completions.AverageTime,
		&channelProfile.FastestUser,
		&channelProfile.SlowestUser,
	)

	if err != nil {
		return channelProfile, false, fmt.Errorf("query row: %w", err)
	}

	if channelProfile.Completions.FastestTime == nil {
		return channelProfile, false, nil
	}

	return channelProfile, true, nil
}

func (s *store) puzzleStatsByUser(ctx context.Context, username string) (UserProfile, bool, error) {
	var userProfile UserProfile

	query := `
	SELECT
		COUNT(*)                                 			AS total_games,
		MIN((details->>'time_seconds')::int)     			AS fastest,
		MAX((details->>'time_seconds')::int)     			AS slowest,
		CAST(AVG((details->>'time_seconds')::int) AS int)	AS avg_time
	FROM lys.game_events
	WHERE username  = $1
	AND game_code = 'oneuppuzzle'
	AND is_flagged = FALSE;
	`

	err := s.pool.QueryRow(ctx, query, username).Scan(
		&userProfile.GamesCompleted,
		&userProfile.Completions.FastestTime,
		&userProfile.Completions.SlowestTime,
		&userProfile.Completions.AverageTime,
	)

	userProfile.UserName = username

	if err != nil {
		return userProfile, false, fmt.Errorf("query row: %w", err)
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
		-- stats
		COUNT(*)                                 			AS total_games,
		MIN((details->>'time_seconds')::int)     			AS fastest_time,
		MAX((details->>'time_seconds')::int)     			AS slowest_time,
		CAST(AVG((details->>'time_seconds')::int) AS int)	AS avg_time,

		-- fastest username
		(
			SELECT username
			FROM lys.game_events
			WHERE (details->'puzzle_id')::INT = $1
			AND game_code  = 'oneuppuzzle'
			AND is_flagged = FALSE
			ORDER BY (details->>'time_seconds')::int ASC
			LIMIT 1
		) AS fastest_user,

		-- slowest username
		(
			SELECT username
			FROM lys.game_events
			WHERE (details->'puzzle_id')::INT = $1
			AND game_code  = 'oneuppuzzle'
			AND is_flagged = FALSE
			ORDER BY (details->>'time_seconds')::int DESC
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
		return gameProfile, false, fmt.Errorf("query row: %w", err)
	}

	if gameProfile.Completions.FastestTime == nil {
		return gameProfile, false, nil
	}

	return gameProfile, true, nil

}

func (s *store) isPuzzleRecorded(ctx context.Context, userID string, gameID int) (bool, error) {
	var recorded bool = false

	query := `
		SELECT EXISTS (
			SELECT 1
			FROM lys.game_events
			WHERE user_id = $1
			AND game_code = 'oneuppuzzle'
			AND
			(details->>'puzzle_id')::int = $2	
		);
	`

	err := s.pool.QueryRow(ctx, query, userID, gameID).Scan(&recorded)
	if err != nil {
		return false, fmt.Errorf("query row: %w", err)
	}

	return recorded, nil
}
