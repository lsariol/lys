package sleeper

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
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

// Names says who a roster is in chat, and players by name.
type Names struct {
	Teams    map[int]string    // team names by roster ID
	Mentions map[int]string    // linked viewers' chat names by roster ID
	Players  map[string]Player // the catalog; nil shows player IDs
}

// NamesFrom builds Names from a league's rosters and users.
func NamesFrom(rosters []Roster, users []LeagueUser, mentions map[int]string, players map[string]Player) Names {
	teams := make(map[int]string, len(rosters))
	for _, r := range rosters {
		teams[r.RosterID] = TeamName(r, users)
	}
	return Names{Teams: teams, Mentions: mentions, Players: players}
}

// Team is the roster's team name, at most 30 characters.
func (n Names) Team(rosterID int) string {
	name := strings.Join(strings.Fields(n.Teams[rosterID]), " ")
	if name == "" {
		return fmt.Sprintf("Team %d", rosterID)
	}
	if r := []rune(name); len(r) > 30 {
		name = string(r[:27]) + "..."
	}
	return name
}

// Who is the roster's @name when a viewer linked it, otherwise its team name.
func (n Names) Who(rosterID int) string {
	if name := n.Mentions[rosterID]; name != "" {
		return "@" + name
	}
	return n.Team(rosterID)
}

// TeamAndWho is the team name, followed by the @name when a viewer linked it.
func (n Names) TeamAndWho(rosterID int) string {
	if name := n.Mentions[rosterID]; name != "" {
		return n.Team(rosterID) + " @" + name
	}
	return n.Team(rosterID)
}

func (n Names) Player(playerID string) string { return PlayerName(playerID, n.Players) }

// Mentions is the chat name of each viewer linked to a team in the league, by roster ID.
func Mentions(ctx context.Context, db *pgxpool.Pool, leagueID string) (map[int]string, error) {
	rows, err := db.Query(ctx, `
		SELECT c.sleeper_roster_id, pu.display_name
		FROM lys.sleeper_connections c
		JOIN lys.sleeper_leagues l ON l.id = c.league_id
		JOIN lys.platform_users pu ON pu.id = c.platform_user_id
		WHERE l.sleeper_league_id = $1`, leagueID)
	if err != nil {
		return nil, fmt.Errorf("database: read the league's linked viewers: %w", err)
	}
	defer rows.Close()
	mentions := make(map[int]string)
	for rows.Next() {
		var roster int
		var name string
		if err := rows.Scan(&roster, &name); err != nil {
			return nil, fmt.Errorf("database: read the league's linked viewers: %w", err)
		}
		mentions[roster] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: read the league's linked viewers: %w", err)
	}
	return mentions, nil
}

// LoadNames reads a league's teams, linked viewers and the player catalog.
// What can't be loaded is logged, and shows as IDs instead.
func (c *Client) LoadNames(ctx context.Context, db *pgxpool.Pool, leagueID string) Names {
	var rosters []Roster
	if r, err := c.Rosters(ctx, leagueID); err != nil {
		log.Printf("[Sleeper] league %s rosters: %v", leagueID, err)
	} else {
		rosters = *r
	}
	var users []LeagueUser
	if u, err := c.LeagueUsers(ctx, leagueID); err != nil {
		log.Printf("[Sleeper] league %s users: %v", leagueID, err)
	} else {
		users = *u
	}
	mentions, err := Mentions(ctx, db, leagueID)
	if err != nil {
		log.Printf("[Sleeper] %v", err)
	}
	players, err := c.Players(ctx)
	if err != nil {
		log.Printf("[Sleeper] players: %v", err)
	}
	return NamesFrom(rosters, users, mentions, players)
}
