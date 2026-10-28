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

// PGStore is the Store in lys.feature_requests. Requesters and channels show
// under their current names.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

func (s *PGStore) StoreRequest(ctx context.Context, featureRequest FeatureRequest) (int64, error) {

	query := `
	INSERT INTO lys.feature_requests (platform, body, connection_id, platform_user_id)
	VALUES ($1, $2, NULLIF($3, 0), $4)
	RETURNING id;
	`

	var id int64
	err := s.pool.QueryRow(ctx, query, featureRequest.Platform, featureRequest.Body,
		featureRequest.ConnectionID, featureRequest.PlatformUserID).Scan(&id)
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
		SELECT f.id, f.occurred_at, COALESCE(pu.external_id, ''), COALESCE(pu.display_name, ''),
		       COALESCE(pc.external_id, ''), COALESCE(pc.name, ''), f.platform, f.body, f.status
		FROM lys.feature_requests f
		LEFT JOIN lys.platform_users pu ON pu.id = f.platform_user_id
		LEFT JOIN lys.platform_connections pc ON pc.id = f.connection_id
		ORDER BY f.occurred_at DESC
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
		SELECT f.id, f.occurred_at, COALESCE(pu.external_id, ''), COALESCE(pu.display_name, ''),
		       COALESCE(pc.external_id, ''), COALESCE(pc.name, ''), f.platform, f.body, f.status
		FROM lys.feature_requests f
		LEFT JOIN lys.platform_users pu ON pu.id = f.platform_user_id
		LEFT JOIN lys.platform_connections pc ON pc.id = f.connection_id
		WHERE f.status = 'new'
		ORDER BY f.id DESC
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
