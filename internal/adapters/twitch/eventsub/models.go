package eventsub

import (
	"time"
)

type SessionData struct {
	SessionID        string
	KeepAliveTimeout int
}

// Join response
type EventSubJoinResponse struct {
	Data         []EventsubSubscription `json:"data"`
	Total        int                    `json:"total"`
	MaxTotalCost int                    `json:"max_total_cost"`
	TotalCost    int                    `json:"total_cost"`
}

type EventsubSubscription struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Type      string            `json:"type"`
	Version   string            `json:"version"`
	Condition EventsubCondition `json:"condition"`
	CreatedAt time.Time         `json:"created_at"`
	Transport EventsubTransport `json:"transport"`
	Cost      int               `json:"cost"`
}

type EventsubCondition struct {
	BroadcasterUserID string `json:"broadcaster_user_id"`
	UserID            string `json:"user_id"`
}

type EventsubTransport struct {
	Method      string    `json:"method"`
	SessionID   string    `json:"session_id"`
	ConnectedAt time.Time `json:"connected_at"`
}
