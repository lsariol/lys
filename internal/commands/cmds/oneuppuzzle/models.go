package oneuppuzzle

import "time"

type OneUpGame struct {
	StoredTime time.Time
	GameCode   string
	RawMessage string
	Details    OneUpGameDetails

	ConnectionID   int64 // 0 if the channel isn't known
	PlatformUserID int64
}

type OneUpGameDetails struct {
	PuzzleID    int `json:"puzzle_id"`
	TimeSeconds int `json:"time_seconds"`
}

// Stat profile for a specific user
type UserProfile struct {
	UserID         string
	UserName       string
	GamesCompleted *int
	Completions    Completions
}

// Stat profile for all users in a specific channel
type ChannelProfile struct {
	ChannelID      string
	ChannelName    string
	GamesCompleted *int
	Completions    Completions
	FastestUser    *string
	SlowestUser    *string
}

// Stat profile for a given gameID
type GameProfile struct {
	ChannelID      string
	ChannelName    string
	GameID         int
	GamesCompleted *int
	Completions    Completions
	FastestUser    *string
	SlowestUser    *string
}

// Stat profile for the entire bot across all channels.
type BotProfile struct {
	ChannelID      string
	ChannelName    string
	GamesCompleted int
	Completions    Completions
}

type Completions struct {
	FastestTime *int
	SlowestTime *int
	AverageTime *int
}
