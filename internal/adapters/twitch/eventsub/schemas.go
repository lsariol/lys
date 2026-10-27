package eventsub

import "encoding/json"

type EventSubMessage struct {
	Metadata MetaData `json:"metadata"`
	Payload  Payload  `json:"payload"`
}

type MetaData struct {
	MessageId           string `json:"message_id"`
	MessageType         string `json:"message_type"`
	MessageTimestamp    string `json:"message_timestamp"`
	SubscriptionType    string `json:"subscription_type"`
	SubscriptionVersion string `json:"subscription_version"`
}

type Payload struct {
	Subscription Subscription `json:"subscription"`
	Event        Event        `json:"event"`
	Session      Session      `json:"session"`
}

type Subscription struct {
	Id         string    `json:"id"`
	Status     string    `json:"status"`
	Type       string    `json:"type"`
	Version    string    `json:"version"`
	Condition  Condition `json:"condition"`
	Transport  Transport `json:"transport"`
	Created_at string    `json:"created_at"`
	Cost       int       `json:"cost"`
}

type Condition struct {
	BroadcasterUserId string `json:"broadcaster_user_id"`
	UserId            string `json:"user_id"`
}

type Transport struct {
	Method    string `json:"method"`
	SessionId string `json:"session_id"`
}

type Event struct {
	BroadcasterUserID           string           `json:"broadcaster_user_id"`
	BroadcasterUserLogin        string           `json:"broadcaster_user_login"`
	BroadcasterUserName         string           `json:"broadcaster_user_name"`
	SourceBroadcasterUserID     *string          `json:"source_broadcaster_user_id"`
	SourceBroadcasterUserLogin  *string          `json:"source_broadcaster_user_login"`
	SourceBroadcasterUserName   *string          `json:"source_broadcaster_user_name"`
	ChatterUserID               string           `json:"chatter_user_id"`
	ChatterUserLogin            string           `json:"chatter_user_login"`
	ChatterUserName             string           `json:"chatter_user_name"`
	MessageID                   string           `json:"message_id"`
	SourceMessageID             *string          `json:"source_message_id"`
	IsSourceOnly                *bool            `json:"is_source_only"`
	Message                     Message          `json:"message"`
	Color                       string           `json:"color"`
	Badges                      []ChatBadge      `json:"badges"`
	SourceBadges                *[]ChatBadge     `json:"source_badges"` // null in sample
	MessageType                 string           `json:"message_type"`
	Cheer                       *json.RawMessage `json:"cheer"` // unknown shape → raw
	Reply                       *json.RawMessage `json:"reply"` // unknown shape → raw
	ChannelPointsCustomRewardID *string          `json:"channel_points_custom_reward_id"`
	ChannelPointsAnimationID    *string          `json:"channel_points_animation_id"`
}

type Message struct {
	Text      string     `json:"text"`
	Fragments []Fragment `json:"fragments"`
}

type Fragment struct {
	Type      string           `json:"type"`
	Text      string           `json:"text"`
	Cheermote *json.RawMessage `json:"cheermote"` // null or object
	Emote     *json.RawMessage `json:"emote"`     // null or object
	Mention   *json.RawMessage `json:"mention"`   // null or object
}

type ChatBadge struct {
	SetID string `json:"set_id"`
	ID    string `json:"id"`
	Info  string `json:"info"`
}

type Session struct {
	ID                      string `json:"id"`
	Status                  string `json:"status"`
	KeepaliveTimeoutSeconds int    `json:"keepalive_timeout_seconds"`
	ReconnectURL            string `json:"reconnect_url"`
	ConnectedAt             string `json:"connected_at"`
}
