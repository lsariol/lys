package sleeper

import "github.com/google/uuid"

// SleeperConnection is a chat member linked to their Sleeper account.
type SleeperConnection struct {
	LeagueID        uuid.UUID
	PlatformUserID  int64
	DisplayName     string // the member's current chat name
	SleeperUserID   string
	SleeperUsername string
	SleeperRosterID int
}

// SleeperLeagueLink finds a community or league that's linked already.
type SleeperLeagueLink struct {
	CommunityID     int64
	SleeperLeagueID string
}

// SleeperLeague is a community's linked league.
type SleeperLeague struct {
	ID              uuid.UUID
	CommunityID     int64
	SleeperLeagueID string
	LeagueName      string
	Season          string // NFL season year, such as "2026"
}
