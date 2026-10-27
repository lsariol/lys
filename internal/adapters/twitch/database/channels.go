package twitchdb

import (
	"context"
	"fmt"
)

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
	SELECT tc.user_id, tc.username, COALESCE(ts.command_prefix, '!')
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
