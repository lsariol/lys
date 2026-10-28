package sleeper

import (
	"fmt"
	"strings"
)

// TeamName is the roster's team name, its owner's name, or "Team <id>".
func TeamName(roster Roster, users []LeagueUser) string {
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

// PlayerName is the player's name ("KC D/ST" for a defense), or "Player <id>"
// when the catalog doesn't have them.
func PlayerName(playerID string, players map[string]Player) string {
	if playerID == "" || playerID == "0" {
		return "Empty"
	}
	player, found := players[playerID]
	if !found {
		return fmt.Sprintf("Player %s", playerID)
	}
	if player.Position == "DEF" {
		if player.Team != "" {
			return player.Team + " D/ST"
		}
		return playerID + " D/ST"
	}

	name := strings.TrimSpace(player.FullName)
	if name == "" {
		name = strings.TrimSpace(player.FirstName + " " + player.LastName)
	}
	if name == "" {
		return fmt.Sprintf("Player %s", playerID)
	}
	return name
}
