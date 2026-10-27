package request

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store keeps feature requests. Status is one of 'new', 'in_progress',
// 'completed' or 'rejected'.
type Store interface {
	StoreRequest(ctx context.Context, fr FeatureRequest) (int64, error)
	UpdateStatus(ctx context.Context, newStatus string, requestID int64) error
	RecentRequests(ctx context.Context, limit int) ([]FeatureRequest, error)
	NewRequests(ctx context.Context, limit int) ([]FeatureRequest, error)
}

// PGStore is the Store in lys.feature_requests.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

func (s *PGStore) StoreRequest(ctx context.Context, featureRequest FeatureRequest) (int64, error) {

	query := `
	INSERT INTO lys.feature_requests (user_id, username, channel_id, channel_name, platform, body)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id;
	`

	var id int64
	err := s.pool.QueryRow(ctx, query, featureRequest.UserID, featureRequest.Username, featureRequest.ChannelID, featureRequest.ChannelName, featureRequest.Platform, featureRequest.Body).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("exec query: %w", err)
	}

	return id, nil
}

func (s *PGStore) UpdateStatus(ctx context.Context, newStatus string, requestID int64) error {
	query := `
	UPDATE lys.feature_requests
	SET status = $1
	WHERE id = $2;
	`

	tag, err := s.pool.Exec(ctx, query, newStatus, requestID)
	if err != nil {
		return fmt.Errorf("updateStatus: executing query: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updateStatus: no request found with id %d", requestID)
	}

	return nil
}

func (s *PGStore) RecentRequests(ctx context.Context, limit int) ([]FeatureRequest, error) {
	query := `
		SELECT id, occurred_at, user_id, username, channel_id, channel_name, platform, body, status
		FROM lys.feature_requests
		ORDER BY occurred_at DESC
		LIMIT $1;
	`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent requests: %w", err)
	}
	return scanRequests(rows)
}

func (s *PGStore) NewRequests(ctx context.Context, limit int) ([]FeatureRequest, error) {

	query := `
		SELECT id, occurred_at, user_id, username, channel_id, channel_name, platform, body, status
		FROM lys.feature_requests
		WHERE status = 'new'
		ORDER BY id DESC
		LIMIT $1;
	`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query new requests: %w", err)
	}
	return scanRequests(rows)
}

func scanRequests(rows pgx.Rows) ([]FeatureRequest, error) {
	defer rows.Close()

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
	return featureRequests, rows.Err()
}
