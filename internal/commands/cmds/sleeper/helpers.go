package sleeper

import (
	"fmt"
	"sort"
	"strings"
)

func getCurrentWeek(league SleeperLeague, state SleeperNFLState) (int, string) {

	if league.Season != state.Season {
		return 0, "This league is not associated with the current NFL season."
	}
	if state.SeasonType != "regular" || state.Week < 1 {
		return 0, "There is no current NFL regular-season week to show."
	}

	return state.Week, ""
}

func getMatchupPoints(matchup SleeperMatchup) float64 {

	if matchup.CustomPoints != nil {
		return *matchup.CustomPoints
	}
	return matchup.Points
}

func groupMatchups(matchups []SleeperMatchup) [][]SleeperMatchup {

	sorted := append([]SleeperMatchup(nil), matchups...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].RosterID < sorted[j].RosterID
	})

	var groups [][]SleeperMatchup
	groupIndexes := make(map[int]int)
	for _, matchup := range sorted {
		if matchup.MatchupID == nil || *matchup.MatchupID <= 0 {
			groups = append(groups, []SleeperMatchup{matchup})
			continue
		}

		index, found := groupIndexes[*matchup.MatchupID]
		if found {
			groups[index] = append(groups[index], matchup)
			continue
		}

		groupIndexes[*matchup.MatchupID] = len(groups)
		groups = append(groups, []SleeperMatchup{matchup})
	}

	return groups
}

func getMatchupTeamName(rosterID int, rosters []SleeperRoster, users []SleeperLeagueUser) string {

	name := fmt.Sprintf("Team %d", rosterID)
	for _, roster := range rosters {
		if roster.RosterID == rosterID {
			name = getTeamName(roster, users)
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

func getTeamName(roster SleeperRoster, users []SleeperLeagueUser) string {

	for _, user := range users {
		if roster.OwnerID != "" && user.UserID == roster.OwnerID {
			if strings.TrimSpace(user.Metadata.TeamName) != "" {
				return user.Metadata.TeamName
			}

			if strings.TrimSpace(user.DisplayName) != "" {
				return user.DisplayName
			}
		}
	}

	return fmt.Sprintf("Team %d", roster.RosterID)
}

func getWinningPercentage(settings SleeperRosterSettings) float64 {

	games := settings.Wins + settings.Losses + settings.Ties
	if games == 0 {
		return 0
	}

	return (float64(settings.Wins) + float64(settings.Ties)/2) / float64(games)
}

func sortStandings(rosters []SleeperRoster) []SleeperRoster {

	sorted := append([]SleeperRoster(nil), rosters...)

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
