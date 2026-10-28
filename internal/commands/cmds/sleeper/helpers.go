package sleeper

import (
	"fmt"
	"sort"
	"strings"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

func getCurrentWeek(league SleeperLeague, state api.NFLState) (int, string) {

	if league.Season != state.Season {
		return 0, "This league is not associated with the current NFL season."
	}
	if state.SeasonType != "regular" || state.Week < 1 {
		return 0, "There is no current NFL regular-season week to show."
	}

	return state.Week, ""
}

func getMatchupPoints(matchup api.Matchup) float64 {

	if matchup.CustomPoints != nil {
		return *matchup.CustomPoints
	}
	return matchup.Points
}

func groupMatchups(matchups []api.Matchup) [][]api.Matchup {

	sorted := append([]api.Matchup(nil), matchups...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].RosterID < sorted[j].RosterID
	})

	var groups [][]api.Matchup
	groupIndexes := make(map[int]int)
	for _, matchup := range sorted {
		if matchup.MatchupID == nil || *matchup.MatchupID <= 0 {
			groups = append(groups, []api.Matchup{matchup})
			continue
		}

		index, found := groupIndexes[*matchup.MatchupID]
		if found {
			groups[index] = append(groups[index], matchup)
			continue
		}

		groupIndexes[*matchup.MatchupID] = len(groups)
		groups = append(groups, []api.Matchup{matchup})
	}

	return groups
}

func getMatchupTeamName(rosterID int, rosters []api.Roster, users []api.LeagueUser) string {

	name := fmt.Sprintf("Team %d", rosterID)
	for _, roster := range rosters {
		if roster.RosterID == rosterID {
			name = api.TeamName(roster, users)
			break
		}
	}

	name = strings.Join(strings.Fields(name), " ")
	nameRunes := []rune(name)
	if len(nameRunes) > 30 {
		name = string(nameRunes[:27]) + "..."
	}
	return name
}

func getWinningPercentage(settings api.RosterSettings) float64 {

	games := settings.Wins + settings.Losses + settings.Ties
	if games == 0 {
		return 0
	}

	return (float64(settings.Wins) + float64(settings.Ties)/2) / float64(games)
}

func sortStandings(rosters []api.Roster) []api.Roster {

	sorted := append([]api.Roster(nil), rosters...)

	sort.Slice(sorted, func(i, j int) bool {
		first := sorted[i].Settings
		second := sorted[j].Settings

		firstPercentage := getWinningPercentage(first)
		secondPercentage := getWinningPercentage(second)

		if firstPercentage != secondPercentage {
			return firstPercentage > secondPercentage
		}

		firstPoints := int64(first.Fpts)*100 + int64(first.FptsDecimal)
		secondPoints := int64(second.Fpts)*100 + int64(second.FptsDecimal)

		if firstPoints != secondPoints {
			return firstPoints > secondPoints
		}

		return sorted[i].RosterID < sorted[j].RosterID
	})

	return sorted
}
