package twitchdb

import (
	"context"
	"fmt"
)

// func (s *Store) GetTokens(ctx context.Context) (*TwitchTokens, error) {
// 	query := `
// 	SELECT access_token, refresh_token
// 	FROM lys.twitch_channels
// 	WHERE user_id = '965482552' AND username = 'botmoba';
// 	`

// 	var tokens TwitchTokens
// 	err := s.pool.QueryRow(ctx, query).Scan(&tokens.UserAccessToken, &tokens.UserRefreshToken)
// 	if err != nil {
// 		return nil, fmt.Errorf("query row: %w", err)
// 	}

// 	return &tokens, nil
// }

// func (s *Store) StoreTokens(ctx context.Context, user string, refresh string) error {
// 	query := `
// 	UPDATE lys.twitch_channels
// 	SET
// 		access_token = $1,
// 		refresh_token = $2
// 	WHERE
// 		user_id = '965482552' AND username = 'botmoba'
// 		AND (
// 			access_token IS DISTINCT FROM $1
// 			OR
// 			refresh_token IS DISTINCT FROM $2
// 			);
// 	`

// 	_, err := s.pool.Exec(ctx, query, user, refresh)
// 	if err != nil {
// 		return fmt.Errorf("pool exec: %w", err)
// 	}

// 	return nil
// }

// twitchDB
func (s *Store) GetUserAccessTokens(ctx context.Context) (*TwitchUserAccessTokens, error) {
	query := `
	SELECT access_token, refresh_token 
	FROM lys.twitch_channels
	WHERE user_id = '965482552' AND username = 'botmoba';
	`

	var tokens TwitchUserAccessTokens
	err := s.pool.QueryRow(ctx, query).Scan(&tokens.UserAccessToken, &tokens.UserRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("query row: %w", err)
	}

	return &tokens, nil
}

func (s *Store) StoreUserAccessTokens(ctx context.Context, user string, refresh string) error {
	query := `
	UPDATE lys.twitch_channels
	SET 
		access_token = $1,
		refresh_token = $2
	WHERE 
		user_id = '965482552' AND username = 'botmoba'
		AND (
			access_token IS DISTINCT FROM $1
			OR
			refresh_token IS DISTINCT FROM $2
			);
	`

	_, err := s.pool.Exec(ctx, query, user, refresh)
	if err != nil {
		return fmt.Errorf("pool exec: %w", err)
	}

	return nil
}
