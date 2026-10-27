package sleeper

import "github.com/google/uuid"

type SleeperPlayer struct {
	FullName  string `json:"full_name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
	Team      string `json:"team"`
}

// Read in from the API NFL state
type SleeperNFLState struct {
	Week       int    `json:"week"`
	Season     string `json:"season"`
	SeasonType string `json:"season_type"`
}

// Read in from the API weekly matchups
type SleeperMatchup struct {
	RosterID     int      `json:"roster_id"`
	MatchupID    *int     `json:"matchup_id"`
	Points       float64  `json:"points"`
	CustomPoints *float64 `json:"custom_points"`
	Starters     []string `json:"starters"`
	Players      []string `json:"players"`
}

// Stored Twitch to Sleeper connection
type SleeperConnection struct {
	LeagueID        uuid.UUID
	PlatformUserID  int64
	DisplayName     string // the member's current chat name
	SleeperUserID   string
	SleeperUsername string
	SleeperRosterID int
}

// used in the database to find potential double linked channels/leagues
type SleeperLeagueLink struct {
	CommunityID     int64
	SleeperLeagueID string
}

// How the
type SleeperLeague struct {
	ID              uuid.UUID
	CommunityID     int64
	SleeperLeagueID string
	LeagueName      string
	Season          string // NFL season year, such as "2026"
}

// Read in from the API league data
type League struct {
	Settings        SleeperLeagueSettings `json:"settings"`
	RosterPositions []string              `json:"roster_positions"`
	LeagueID        string                `json:"league_id"`
	Name            string                `json:"name"`
	Season          string                `json:"season"` // NFL season year, such as "2026"
	Sport           string                `json:"sport"`
	Status          string                `json:"status"`
	TotalRosters    int                   `json:"total_rosters"`
}

// Read in from the sleeper API to get all users
type SleeperLeagueSettings struct {
	PlayoffWeekStart int  `json:"playoff_week_start"`
	PlayoffTeams     int  `json:"playoff_teams"`
	PlayoffRoundType *int `json:"playoff_round_type"`
}

type SleeperPlayoffSource struct {
	Winner *int `json:"w"`
	Loser  *int `json:"l"`
}

type SleeperPlayoffMatchup struct {
	Round     int                   `json:"r"`
	MatchID   int                   `json:"m"`
	Team1     *int                  `json:"t1"`
	Team2     *int                  `json:"t2"`
	Winner    *int                  `json:"w"`
	Loser     *int                  `json:"l"`
	Place     *int                  `json:"p"`
	Team1From *SleeperPlayoffSource `json:"t1_from"`
	Team2From *SleeperPlayoffSource `json:"t2_from"`
}

type SleeperLeagueUser struct {
	Avatar      *string             `json:"avatar"`
	DisplayName string              `json:"display_name"`
	IsBot       bool                `json:"is_bot"`
	IsOwner     *bool               `json:"is_owner"`
	LeagueID    string              `json:"league_id"`
	Metadata    SleeperUserMetadata `json:"metadata"`
	Settings    any                 `json:"settings"`
	UserID      string              `json:"user_id"`
}

type SleeperUserMetadata struct {
	AllowPN   string `json:"allow_pn"`
	Avatar    string `json:"avatar"`
	Archived  string `json:"archived"`
	MentionPN string `json:"mention_pn"`
	TeamName  string `json:"team_name"`
}

// Read in from the sleeper API to get all rosters
type SleeperRoster struct {
	Taxi     []string              `json:"taxi"`
	Starters []string              `json:"starters"`
	Settings SleeperRosterSettings `json:"settings"`
	RosterID int                   `json:"roster_id"`
	Reserve  []string              `json:"reserve"`
	Players  []string              `json:"players"`
	OwnerID  string                `json:"owner_id"`
	LeagueID string                `json:"league_id"`
}

type SleeperRosterSettings struct {
	Wins               int `json:"wins"`
	WaiverPosition     int `json:"waiver_position"`
	WaiverBudgetUsed   int `json:"waiver_budget_used"`
	TotalMoves         int `json:"total_moves"`
	Ties               int `json:"ties"`
	Losses             int `json:"losses"`
	FptsDecimal        int `json:"fpts_decimal"`
	FptsAgainstDecimal int `json:"fpts_against_decimal"`
	FptsAgainst        int `json:"fpts_against"`
	Fpts               int `json:"fpts"`
}
