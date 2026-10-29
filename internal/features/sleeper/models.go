package sleeper

type Player struct {
	FullName  string `json:"full_name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
	Team      string `json:"team"`
}

// NFLState is the current NFL season and week.
type NFLState struct {
	Week       int    `json:"week"`
	Season     string `json:"season"`
	SeasonType string `json:"season_type"`
}

// Matchup is one roster's side of a week's matchup.
type Matchup struct {
	RosterID     int      `json:"roster_id"`
	MatchupID    *int     `json:"matchup_id"`
	Points       float64  `json:"points"`
	CustomPoints *float64 `json:"custom_points"`
	Starters     []string `json:"starters"`
	Players      []string `json:"players"`

	StartersPoints []float64          `json:"starters_points"` // in the order of Starters
	PlayersPoints  map[string]float64 `json:"players_points"`  // by player ID
}

type League struct {
	Settings        LeagueSettings `json:"settings"`
	RosterPositions []string       `json:"roster_positions"`
	LeagueID        string         `json:"league_id"`
	Name            string         `json:"name"`
	Season          string         `json:"season"` // NFL season year, such as "2026"
	Sport           string         `json:"sport"`
	Status          string         `json:"status"`
	TotalRosters    int            `json:"total_rosters"`
}

type LeagueSettings struct {
	PlayoffWeekStart int  `json:"playoff_week_start"`
	PlayoffTeams     int  `json:"playoff_teams"`
	PlayoffRoundType *int `json:"playoff_round_type"`
}

// PlayoffSource says which earlier match a playoff team comes from.
type PlayoffSource struct {
	Winner *int `json:"w"`
	Loser  *int `json:"l"`
}

type PlayoffMatchup struct {
	Round     int            `json:"r"`
	MatchID   int            `json:"m"`
	Team1     *int           `json:"t1"`
	Team2     *int           `json:"t2"`
	Winner    *int           `json:"w"`
	Loser     *int           `json:"l"`
	Place     *int           `json:"p"`
	Team1From *PlayoffSource `json:"t1_from"`
	Team2From *PlayoffSource `json:"t2_from"`
}

// LeagueUser is a Sleeper account in a league.
type LeagueUser struct {
	Avatar      *string      `json:"avatar"`
	DisplayName string       `json:"display_name"`
	IsBot       bool         `json:"is_bot"`
	IsOwner     *bool        `json:"is_owner"`
	LeagueID    string       `json:"league_id"`
	Metadata    UserMetadata `json:"metadata"`
	Settings    any          `json:"settings"`
	UserID      string       `json:"user_id"`
}

type UserMetadata struct {
	AllowPN   string `json:"allow_pn"`
	Avatar    string `json:"avatar"`
	Archived  string `json:"archived"`
	MentionPN string `json:"mention_pn"`
	TeamName  string `json:"team_name"`
}

type Roster struct {
	Taxi     []string       `json:"taxi"`
	Starters []string       `json:"starters"`
	Settings RosterSettings `json:"settings"`
	RosterID int            `json:"roster_id"`
	Reserve  []string       `json:"reserve"`
	Players  []string       `json:"players"`
	OwnerID  string         `json:"owner_id"`
	LeagueID string         `json:"league_id"`
}

type RosterSettings struct {
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

// Transaction is a trade, waiver claim or free-agent move.
type Transaction struct {
	TransactionID string          `json:"transaction_id"`
	Type          string          `json:"type"`   // trade, waiver, free_agent, commissioner
	Status        string          `json:"status"` // complete, failed, ...
	StatusUpdated int64           `json:"status_updated"`
	RosterIDs     []int           `json:"roster_ids"`
	Adds          map[string]int  `json:"adds"`  // player ID -> roster ID
	Drops         map[string]int  `json:"drops"` // player ID -> roster ID
	DraftPicks    []TradedPick    `json:"draft_picks"`
	WaiverBudget  []BudgetPayment `json:"waiver_budget"`
	Settings      *struct {
		WaiverBid *int `json:"waiver_bid"`
	} `json:"settings"`
}

// TradedPick is a draft pick that changes hands in a trade.
type TradedPick struct {
	Season  string `json:"season"`
	Round   int    `json:"round"`
	OwnerID int    `json:"owner_id"` // the roster that gets it
}

// BudgetPayment is FAAB budget sent in a trade.
type BudgetPayment struct {
	Sender   int `json:"sender"`
	Receiver int `json:"receiver"`
	Amount   int `json:"amount"`
}

// Draft is one of a league's drafts.
type Draft struct {
	DraftID    string `json:"draft_id"`
	Season     string `json:"season"`
	Status     string `json:"status"`      // pre_draft, drafting, paused, complete
	StartTime  *int64 `json:"start_time"`  // Unix milliseconds; unset until it's scheduled
	LastPicked *int64 `json:"last_picked"` // Unix milliseconds
}
