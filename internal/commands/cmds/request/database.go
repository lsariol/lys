package request

import (
	"context"
	"fmt"

	"github.com/lsariol/lys/internal/app/dependencies"
)

// status types can only be 'new', 'in_progress', 'completed', 'rejected'

func storeRequest(ctx context.Context, featureRequest FeatureRequest, deps *dependencies.Deps) (int64, error) {

	query := `
	INSERT INTO lys.feature_requests (user_id, username, channel_id, channel_name, platform, body)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id;
	`

	var id int64
	err := deps.DB.Pool.QueryRow(ctx, query, featureRequest.UserID, featureRequest.Username, featureRequest.ChannelID, featureRequest.ChannelName, featureRequest.Platform, featureRequest.Body).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("exec query: %w", err)
	}

	return id, nil
}

func updateStatus(ctx context.Context, newStatus string, requestID int64, deps *dependencies.Deps) error {
	query := `
	UPDATE lys.feature_requests
	SET status = $1
	WHERE id = $2;
	`

	tag, err := deps.DB.Pool.Exec(ctx, query, newStatus, requestID)
	if err != nil {
		return fmt.Errorf("updateStatus: executing query: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updateStatus: no request found with id %d", requestID)
	}

	return nil
}

func getRecentRequests(ctx context.Context, limit int, deps *dependencies.Deps) ([]FeatureRequest, error) {
	query := `
		SELECT id, occurred_at, user_id, username, channel_id, channel_name, platform, body, status
		FROM lys.feature_requests
		ORDER BY occurred_at DESC
		LIMIT $1;
	`

	rows, err := deps.DB.Pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query getnewrequests: %w", err)
	}

	featureRequests := []FeatureRequest{}
	for rows.Next() {
		var featureRequest FeatureRequest
		if err := rows.Scan(
			&featureRequest.ID,
			&featureRequest.Timestamp,
			&featureRequest.UserID,
			&featureRequest.Username,
			&featureRequest.ChannelID,
			&featureRequest.ChannelName,
			&featureRequest.Platform,
			&featureRequest.Body,
			&featureRequest.Status,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		featureRequests = append(featureRequests, featureRequest)
	}
	return featureRequests, nil
}

func getNewRequests(ctx context.Context, limit int, deps *dependencies.Deps) ([]FeatureRequest, error) {

	query := `
		SELECT id, occurred_at, user_id, username, channel_id, channel_name, platform, body, status
		FROM lys.feature_requests
		WHERE status = 'new'
		ORDER BY id DESC
		LIMIT $1;
	`

	rows, err := deps.DB.Pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query getnewrequests: %w", err)
	}

	featureRequests := []FeatureRequest{}
	for rows.Next() {
		var featureRequest FeatureRequest
		if err := rows.Scan(
			&featureRequest.ID,
			&featureRequest.Timestamp,
			&featureRequest.UserID,
			&featureRequest.Username,
			&featureRequest.ChannelID,
			&featureRequest.ChannelName,
			&featureRequest.Platform,
			&featureRequest.Body,
			&featureRequest.Status,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		featureRequests = append(featureRequests, featureRequest)

	}

	return featureRequests, nil
}

func getRecentRequestsByChannel(ctx context.Context, channelID string, limit int, deps *dependencies.Deps) ([]FeatureRequest, error) {

	query := `
		SELECT * FROM lys.feature_requests
		WHERE channel_id = $1
		ORDER BY occurred_at DESC
		LIMIT $2;
	`

	rows, err := deps.DB.Pool.Query(ctx, query, channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("query getnewrequests: %w", err)
	}

	featureRequests := []FeatureRequest{}
	for rows.Next() {
		var fR FeatureRequest
		if err := rows.Scan(fR); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		featureRequests = append(featureRequests, fR)

	}

	return featureRequests, nil
}

func getNewRequestsByChannel(ctx context.Context, channelID string, limit int, deps *dependencies.Deps) ([]FeatureRequest, error) {

	query := `
		SELECT * FROM lys.feature_requests
		WHERE status = 'new'
		AND
		channel_id = $1
		ORDER BY id DESC
		LIMIT $2;
	`

	rows, err := deps.DB.Pool.Query(ctx, query, channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("query getnewrequests: %w", err)
	}

	featureRequests := []FeatureRequest{}
	for rows.Next() {
		var fR FeatureRequest
		if err := rows.Scan(fR); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		featureRequests = append(featureRequests, fR)

	}

	return featureRequests, nil
}
