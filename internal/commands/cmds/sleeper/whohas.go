package sleeper

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// whohas is "!ff whohas <player>": which team has the player.
func whohas(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	search := strings.Join(strings.Fields(strings.Join(e.Args[1:], " ")), " ")
	if search == "" {
		return usage(e, "whohas <player name or ID>"), nil
	}

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	players, err := sl.client.Players(ctx)
	if err != nil {
		return say("Player names are still loading, try again in a moment."), err
	}

	matches := findPlayers(search, players)
	switch {
	case len(matches) == 0:
		return say("No player matches %q.", search), nil
	case len(matches) > 1:
		var names []string
		for _, id := range matches[:min(len(matches), 5)] {
			names = append(names, formatRosterEntry("", fmt.Sprintf("%s [%s]", api.PlayerName(id, players), id)))
		}
		text := "Several players match: " + strings.Join(names, " · ")
		if len(matches) > 5 {
			text += fmt.Sprintf(" · and %d more", len(matches)-5)
		}
		return say("%s. Use a full name or the ID.", text), nil
	}

	playerID := matches[0]
	name := formatRosterEntry("", api.PlayerName(playerID, players))
	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	var owners []api.Roster
	for _, r := range rosters {
		if rosterHasPlayer(r, playerID) {
			owners = append(owners, r)
		}
	}
	switch len(owners) {
	case 0:
		return say("%s is a free agent.", name), nil
	case 1:
		n := sl.names(ctx, league, rosters, users, nil)
		return say("%s is on %s.", name, n.TeamAndWho(owners[0].RosterID)), nil
	}
	return say("Sleeper lists %s on more than one team, so it's unclear who has him.", name), nil
}

func findPlayers(search string, players map[string]api.Player) []string {
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
		name := strings.ToLower(strings.Join(strings.Fields(api.PlayerName(playerID, players)), " "))
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
		firstName := strings.ToLower(api.PlayerName(matches[i], players))
		secondName := strings.ToLower(api.PlayerName(matches[j], players))
		if firstName != secondName {
			return firstName < secondName
		}
		return matches[i] < matches[j]
	})
	return matches
}

func rosterHasPlayer(roster api.Roster, playerID string) bool {
	for _, players := range [][]string{roster.Players, roster.Starters, roster.Reserve, roster.Taxi} {
		for _, id := range players {
			if id == playerID {
				return true
			}
		}
	}
	return false
}
