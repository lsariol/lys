package twitchdb

import (
	"context"
	"fmt"
)

func (s *Store) AddChannel(ctx context.Context, ch AddChannelParams) error {
	query := `
	INSERT INTO lys.twitch_channels (user_id, username, access_token, refresh_token)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (user_id) DO UPDATE
		SET username = EXCLUDED.username,
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			active = TRUE,
			times_joined = lys.twitch_channels.times_joined + 1,
			last_joined = NOW();`

	_, err := s.pool.Exec(ctx, query, ch.UserID, ch.Username, ch.AccessToken, ch.RefreshToken)
	if err == nil {
		err = s.LogEvent(ctx, ch, "grant")
	}

	return err
}

func (s *Store) RemoveChannel(ctx context.Context, channelID string) error {
	return nil
}

func (s *Store) BanChannel(ctx context.Context, channelID string) error {
	query := `
	UPDATE lys.twitch_channels
	SET banned = TRUE
	WHERE user_id = $1;
`
	_, err := s.pool.Exec(ctx, query, channelID)
	if err != nil {
		return fmt.Errorf("failed to ban user %s: %w", channelID, err)
	}

	return nil
}

func (s *Store) UnbanChannel(ctx context.Context, channelID string) error {
	query := `
	UPDATE lys.twitch_channels
	SET banned = FALSE
	WHERE user_id = $1;
`
	_, err := s.pool.Exec(ctx, query, channelID)
	if err != nil {
		return fmt.Errorf("failed to unban user %s: %w", channelID, err)
	}

	return nil
}

func (s *Store) GetAllChannels(ctx context.Context) (map[string]TwitchChannel, error) {

	query := `
	SELECT user_id, username
	FROM lys.twitch_channels 
	WHERE 
		active = TRUE
		AND
		banned = FALSE;
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query getallchannels: %w", err)
	}
	defer rows.Close()

	channels := make(map[string]TwitchChannel)

	for rows.Next() {
		var channel TwitchChannel
		if err := rows.Scan(&channel.UserID, &channel.Username); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		channels[channel.UserID] = channel
	}

	return channels, nil
}

func (s *Store) GetPerChannelSettings(ctx context.Context) (map[string]TwitchChannelSettings, error) {

	query := `
	SELECT tc.user_ID, tc.username, ts.command_prefix
	FROM lys.twitch_channels AS tc
	LEFT JOIN lys.twitch_settings AS ts
	ON tc.user_id = ts.user_id
	WHERE 
		tc.active = TRUE
		AND
		tc.banned = false;
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query getallchannels: %w", err)
	}
	defer rows.Close()

	perChannelSettings := make(map[string]TwitchChannelSettings)

	for rows.Next() {
		var setting TwitchChannelSettings
		if err := rows.Scan(&setting.UserID, &setting.Username, &setting.CommandPrefix); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		perChannelSettings[setting.UserID] = setting
	}

	return perChannelSettings, nil
}

// Action can either be 'grant', 'revoke', 'ban', 'unban'
func (s *Store) LogEvent(ctx context.Context, ch AddChannelParams, action string) error {
	query := `
	INSERT INTO lys.twitch_channel_events (user_id, username, action)
	VALUES ($1, $2, $3)
	`
	_, err := s.pool.Exec(ctx, query, ch.UserID, ch.Username, action)
	if err != nil {
		err = fmt.Errorf("error logging event: %w", err)
	}
	return err
}
