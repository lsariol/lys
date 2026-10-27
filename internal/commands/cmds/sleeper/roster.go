package sleeper

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

func roster(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	const usage = "Usage: !sleeper roster [@user] [page]"
	if len(e.Args) < 1 || len(e.Args) > 3 {
		return commands.Reply{Text: usage}, nil
	}

	username := e.User.Name
	page := 1
	pageArgument := ""
	if len(e.Args) >= 2 {
		if strings.HasPrefix(e.Args[1], "@") {
			username = strings.TrimPrefix(e.Args[1], "@")
			if username == "" {
				return commands.Reply{Text: usage}, nil
			}
			if len(e.Args) == 3 {
				pageArgument = e.Args[2]
			}
		} else {
			if len(e.Args) == 3 {
				return commands.Reply{Text: usage}, nil
			}
			pageArgument = e.Args[1]
		}
		if pageArgument != "" {
			requestedPage, err := strconv.Atoi(pageArgument)
			if err != nil || requestedPage < 1 {
				return commands.Reply{Text: usage + ". Page must be a positive number."}, nil
			}
			page = requestedPage
		}
	}

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Internal error getting league information."}, err
	}
	if league == nil {
		return commands.Reply{Text: "This channel is not associated with any league."}, nil
	}

	var connection *SleeperConnection
	if strings.EqualFold(username, e.User.Name) {
		connection, err = getConnectionByUserID(ctx, league.ID, e.Identity.PlatformUserID, sl)
	} else {
		connection, err = getConnectionByUsername(ctx, league.ID, username, sl)
	}
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Internal error finding the linked account."}, err
	}
	if connection == nil {
		return commands.Reply{Text: fmt.Sprintf("@%s is not linked to a Sleeper account in this league.", username)}, nil
	}

	sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league rosters from Sleeper."}, err
	}
	if sleeperRosters == nil || len(*sleeperRosters) == 0 {
		return commands.Reply{Text: "No rosters are available for this league yet."}, nil
	}

	var selectedRoster *SleeperRoster
	for i := range *sleeperRosters {
		if (*sleeperRosters)[i].RosterID == connection.SleeperRosterID {
			selectedRoster = &(*sleeperRosters)[i]
			break
		}
	}
	if selectedRoster == nil {
		return commands.Reply{Text: fmt.Sprintf("@%s's linked roster could not be found in this league.", username)}, nil
	}
	if selectedRoster.OwnerID == "" || selectedRoster.OwnerID != connection.SleeperUserID {
		return commands.Reply{Text: fmt.Sprintf("@%s's saved link no longer matches the roster owner. Please unlink and link the account again.", username)}, nil
	}

	sleeperLeague, err := GetLeague(ctx, league.SleeperLeagueID, sl.http)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league settings from Sleeper."}, err
	}
	if sleeperLeague == nil || sleeperLeague.LeagueID == "" {
		return commands.Reply{Text: "Sleeper did not return valid league settings."}, nil
	}

	players, err := getPlayerCatalog(ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Player names are loading or temporarily unavailable. Please try again shortly."}, nil
	}

	entries := getRosterEntries(*selectedRoster, sleeperLeague.RosterPositions, players)
	if len(entries) == 0 {
		return commands.Reply{Text: fmt.Sprintf("@%s's roster is empty.", username)}, nil
	}

	// Four bounded entries keep the response compact.
	const pageSize = 4
	totalPages := (len(entries) + pageSize - 1) / pageSize
	if page > totalPages {
		return commands.Reply{Text: fmt.Sprintf("Roster has %d page(s).", totalPages)}, nil
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(entries) {
		end = len(entries)
	}

	return commands.Reply{Text: fmt.Sprintf("@%s roster (%d/%d): %s",
		username, page, totalPages, strings.Join(entries[start:end], " | "))}, nil
}

func getRosterEntries(roster SleeperRoster, positions []string, players map[string]SleeperPlayer) []string {

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

	slotCount := len(roster.Starters)
	if len(starterPositions) > slotCount {
		slotCount = len(starterPositions)
	}
	for i := 0; i < slotCount; i++ {
		position := "Starter"
		if i < len(starterPositions) {
			position = starterPositions[i]
		}
		playerID := "0"
		if i < len(roster.Starters) {
			playerID = roster.Starters[i]
		}
		entries = append(entries, formatRosterEntry(position, getPlayerName(playerID, players)))
		if playerID != "" && playerID != "0" {
			seen[playerID] = true
		}
	}

	for _, playerID := range roster.Players {
		if playerID == "" || playerID == "0" || seen[playerID] || reserve[playerID] || taxi[playerID] {
			continue
		}
		entries = append(entries, formatRosterEntry("BN", getPlayerName(playerID, players)))
		seen[playerID] = true
	}
	for _, playerID := range roster.Reserve {
		if playerID == "" || playerID == "0" || seen[playerID] {
			continue
		}
		entries = append(entries, formatRosterEntry("IR", getPlayerName(playerID, players)))
		seen[playerID] = true
	}
	for _, playerID := range roster.Taxi {
		if playerID == "" || playerID == "0" || seen[playerID] {
			continue
		}
		entries = append(entries, formatRosterEntry("Taxi", getPlayerName(playerID, players)))
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
