package sleeper

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

func whohas(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	if len(e.Args) < 2 {
		return commands.Reply{Text: "Usage: !sleeper whohas <player name or ID>"}, nil
	}

	search := strings.Join(strings.Fields(strings.Join(e.Args[1:], " ")), " ")
	if search == "" {
		return commands.Reply{Text: "Usage: !sleeper whohas <player name or ID>"}, nil
	}

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Internal error getting league information."}, err
	}
	if league == nil {
		return commands.Reply{Text: "This channel is not associated with any league."}, nil
	}

	players, err := getPlayerCatalog(ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Player names are loading or temporarily unavailable. Please try again shortly."}, nil
	}

	matches := findPlayers(search, players)
	if len(matches) == 0 {
		return commands.Reply{Text: "No player matched that name or ID."}, nil
	}

	if len(matches) > 1 {
		var names []string
		limit := len(matches)
		if limit > 5 {
			limit = 5
		}
		for _, playerID := range matches[:limit] {
			name := getPlayerName(playerID, players)
			names = append(names, formatRosterEntry("", fmt.Sprintf("%s [%s]", name, playerID)))
		}

		text := "Multiple players matched: " + strings.Join(names, " | ")
		if len(matches) > limit {
			text += fmt.Sprintf(" | +%d more", len(matches)-limit)
		}
		return commands.Reply{Text: text + ". Use a full name or player ID."}, nil
	}

	playerID := matches[0]
	name := formatRosterEntry("", getPlayerName(playerID, players))

	sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league rosters from Sleeper."}, err
	}
	if sleeperRosters == nil || len(*sleeperRosters) == 0 {
		return commands.Reply{Text: "No rosters are available for this league yet."}, nil
	}

	var owners []SleeperRoster
	for _, roster := range *sleeperRosters {
		if rosterHasPlayer(roster, playerID) {
			owners = append(owners, roster)
		}
	}
	if len(owners) == 0 {
		return commands.Reply{Text: fmt.Sprintf("%s is currently unrostered.", name)}, nil
	}
	if len(owners) > 1 {
		return commands.Reply{Text: fmt.Sprintf("Sleeper lists %s on multiple rosters, so ownership is unclear.", name)}, nil
	}

	sleeperUsers, err := getPlayersInLeague(league.SleeperLeagueID, ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league members from Sleeper."}, err
	}
	var users []SleeperLeagueUser
	if sleeperUsers != nil {
		users = *sleeperUsers
	}

	owner := owners[0]
	teamName := getMatchupTeamName(owner.RosterID, *sleeperRosters, users)
	for _, user := range users {
		if owner.OwnerID != "" && user.UserID == owner.OwnerID {
			displayName := formatRosterEntry("", user.DisplayName)
			if displayName != "" && !strings.EqualFold(displayName, teamName) {
				return commands.Reply{Text: fmt.Sprintf("%s belongs to %s / %s.", name, displayName, teamName)}, nil
			}
			break
		}
	}
	return commands.Reply{Text: fmt.Sprintf("%s is rostered on %s.", name, teamName)}, nil
}

func findPlayers(search string, players map[string]SleeperPlayer) []string {

	search = strings.ToLower(strings.Join(strings.Fields(search), " "))
	if search == "" {
		return nil
	}

	var exactMatches []string
	var partialMatches []string
	var idMatches []string
	for playerID := range players {
		if strings.EqualFold(playerID, search) {
			idMatches = append(idMatches, playerID)
		}
		name := strings.ToLower(strings.Join(strings.Fields(getPlayerName(playerID, players)), " "))
		if name == search {
			exactMatches = append(exactMatches, playerID)
		} else if strings.Contains(name, search) {
			partialMatches = append(partialMatches, playerID)
		}
	}

	matches := partialMatches
	if len(exactMatches) > 0 {
		matches = exactMatches
	}
	if len(idMatches) > 0 {
		matches = idMatches
	}
	sort.Slice(matches, func(i, j int) bool {
		firstName := strings.ToLower(getPlayerName(matches[i], players))
		secondName := strings.ToLower(getPlayerName(matches[j], players))
		if firstName != secondName {
			return firstName < secondName
		}
		return matches[i] < matches[j]
	})
	return matches
}

func rosterHasPlayer(roster SleeperRoster, playerID string) bool {

	for _, players := range [][]string{roster.Players, roster.Starters, roster.Reserve, roster.Taxi} {
		for _, id := range players {
			if id == playerID {
				return true
			}
		}
	}
	return false
}
