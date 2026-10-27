// Package core holds the platform-neutral types every adapter, command and
// source shares.
package core

import (
	"context"
	"time"
)

// Message is one chat message from any platform.
type Message struct {
	Platform string // e.g. "twitch"
	ID       string // the platform's message ID
	Channel  Channel
	User     User
	Text     string
	Time     time.Time
}

// Channel is where a message was sent.
type Channel struct {
	ID   string
	Name string
}

// User is who sent a message.
type User struct {
	ID    string
	Name  string // display name
	Roles Roles
}

// Roles is a set of the sender's roles in the channel.
type Roles uint8

const (
	RoleBroadcaster Roles = 1 << iota
	RoleModerator
	RoleVIP
	RoleSubscriber
)

// Has reports whether every role in want is set.
func (r Roles) Has(want Roles) bool { return r&want == want }

// Outbound is a message for an adapter to send.
type Outbound struct {
	Platform string
	Channel  Channel
	Text     string
}

// ChannelConfig holds per-channel settings, keyed by platform and channel ID.
type ChannelConfig interface {
	// Prefix returns the channel's command prefix ("!" unless set).
	Prefix(platform, channelID string) string
	SetPrefix(ctx context.Context, platform, channelID, prefix string) error
}

// DefaultPrefix is the command prefix for channels that haven't set one.
const DefaultPrefix = "!"
