package request

import (
	"time"
)

type RequestStatus string

const (
	StatusNew        RequestStatus = "new"
	StatusInProgress RequestStatus = "in_progress"
	StatusComplete   RequestStatus = "completed"
	StatusRejected   RequestStatus = "rejected"
)

type FeatureRequest struct {
	ID          int
	Timestamp   time.Time
	UserID      string
	Username    string
	ChannelID   string
	ChannelName string
	Platform    string
	Body        string
	Status      RequestStatus

	ConnectionID   int64
	PlatformUserID int64
}
