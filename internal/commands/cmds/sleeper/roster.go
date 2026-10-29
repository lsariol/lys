package sleeper

import (
	"context"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// roster is "!ff roster [@user]": starters by position, then the bench,
// injured reserve and taxi squad. A long roster takes two messages.
func roster(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	if len(e.Args) > 2 {
		return usage(e, "roster [@user]"), nil
	}
	name, self := targetUser(e, e.Args[1:])

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	selected, reply, err := sl.member(ctx, e, league, name, self, rosters)
	if reply != nil {
		return *reply, err
	}
	settings, err := sl.client.League(ctx, league.SleeperLeagueID)
	if err != nil {
		return unreachable(), err
	}
	players, err := sl.client.Players(ctx)
	if err != nil {
		return say("Player names are still loading, try again in a moment."), err
	}
	n := sl.names(ctx, league, rosters, users, players)

	entries := getRosterEntries(*selected, settings.RosterPositions, players)
	if len(entries) == 0 {
		return say("%s's roster is empty.", n.Who(selected.RosterID)), nil
	}
	return say("%s's roster: %s", n.Who(selected.RosterID), strings.Join(entries, " · ")), nil
}

func getRosterEntries(roster api.Roster, positions []string, players map[string]api.Player) []string {
	var entries []string
	var starterPositions []string
	for _, position := range positions {
		switch position {
		case "BN", "IR", "TAXI":
			continue
		default:
			starterPositions = append(starterPositions, position)
		}
	}

	seen := make(map[string]bool)
	reserve := make(map[string]bool)
	taxi := make(map[string]bool)
	for _, playerID := range roster.Reserve {
		reserve[playerID] = true
	}
	for _, playerID := range roster.Taxi {
		taxi[playerID] = true
	}

	slotCount := max(len(roster.Starters), len(starterPositions))
	for i := 0; i < slotCount; i++ {
		position := "Starter"
		if i < len(starterPositions) {
			position = starterPositions[i]
		}
		playerID := "0"
		if i < len(roster.Starters) {
			playerID = roster.Starters[i]
		}
		entries = append(entries, formatRosterEntry(position, api.PlayerName(playerID, players)))
		if playerID != "" && playerID != "0" {
			seen[playerID] = true
		}
	}

	for _, playerID := range roster.Players {
		if playerID == "" || playerID == "0" || seen[playerID] || reserve[playerID] || taxi[playerID] {
			continue
		}
		entries = append(entries, formatRosterEntry("BN", api.PlayerName(playerID, players)))
		seen[playerID] = true
	}
	for _, playerID := range roster.Reserve {
		if playerID == "" || playerID == "0" || seen[playerID] {
			continue
		}
		entries = append(entries, formatRosterEntry("IR", api.PlayerName(playerID, players)))
		seen[playerID] = true
	}
	for _, playerID := range roster.Taxi {
		if playerID == "" || playerID == "0" || seen[playerID] {
			continue
		}
		entries = append(entries, formatRosterEntry("Taxi", api.PlayerName(playerID, players)))
		seen[playerID] = true
	}
	return entries
}

func formatRosterEntry(position string, name string) string {
	entry := strings.Join(strings.Fields(position+" "+name), " ")
	// Bound bytes as well as preserving complete UTF-8 characters.
	for len(entry) > 80 {
		runes := []rune(entry)
		entry = string(runes[:len(runes)-1])
	}
	return entry
}
