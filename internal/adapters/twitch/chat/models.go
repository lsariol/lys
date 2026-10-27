package chat

import "time"

// Will send messages, handle message queue and so on

type SendChatBody struct {
	BroadcasterID string `json:"broadcaster_id"`
	SenderID      string `json:"sender_id"`
	Message       string `json:"message"`
}

type SendChatMessageResponse struct {
	Data []SendChatMessageData `json:"data"`
}

type SendChatMessageData struct {
	MessageID  string      `json:"message_id"`
	IsSent     bool        `json:"is_sent"`
	DropReason *DropReason `json:"drop_reason"`
}

type DropReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type HelixError struct {
	Error   string `json:"error"`
	Status  int    `json:"status"`
	Message string `json:"message"`

	RetryAfter time.Duration `json:"-"` // from Ratelimit-Reset on a 429
}
