// Package core holds the platform-neutral types every adapter, command and
// source shares.
package core

import (
	"context"
	"errors"
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

// Identity is who a message's sender is to Lys.
type Identity struct {
	PlatformUserID int64 // their platform_users row; 0 if it couldn't be looked up
	AccountID      int64 // their Lys account; 0 until they link one
}

// Identities finds, or records, the Lys identity of a platform user.
type Identities interface {
	Resolve(ctx context.Context, platform string, u User) (Identity, error)
}

// Outbound is a message for an adapter to send.
type Outbound struct {
	Platform string
	Channel  Channel
	Text     string
}

// Connection is a channel as Lys knows it: its connection and community.
type Connection struct {
	ID          int64 // platform_connections row
	CommunityID int64
}

// ChannelConfig holds per-channel settings, keyed by platform and channel ID.
type ChannelConfig interface {
	// Prefix returns the channel's command prefix ("!" unless set).
	Prefix(platform, channelID string) string
	SetPrefix(ctx context.Context, platform, channelID, prefix string) error
	// Connection returns the channel's IDs; false if Lys doesn't know it.
	Connection(platform, channelID string) (Connection, bool)
}

// Directory lists the channels Lys should be in on a platform.
type Directory interface {
	Channels(ctx context.Context, platform string) ([]Channel, error)
}

// Revoker records that Lys may no longer be in a channel; it then drops out
// of the Directory.
type Revoker interface {
	Revoke(ctx context.Context, platform, channelID, reason string) error
}

// ChannelInfo is what a platform says about one of its channels.
type ChannelInfo struct {
	ID    string
	Name  string // display name
	Login string // the handle, where the platform has one
}

// ErrNoSuchChannel means the platform has no channel with that ID.
var ErrNoSuchChannel = errors.New("no such channel on the platform")

// ChannelLookup is an adapter that can confirm a channel exists, by its ID.
type ChannelLookup interface {
	Platform() string
	// LookupChannel returns an error wrapping ErrNoSuchChannel if there's none.
	LookupChannel(ctx context.Context, id string) (ChannelInfo, error)
}

// DefaultPrefix is the command prefix for channels that haven't set one.
const DefaultPrefix = "!"
