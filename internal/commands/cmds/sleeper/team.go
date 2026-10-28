package sleeper

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

func team(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return commands.Reply{Text: "Usage: !sleeper team [@user]"}, nil
	}

	username := e.User.Name
	if len(e.Args) == 2 {
		username = strings.TrimPrefix(strings.TrimSpace(e.Args[1]), "@")
		if username == "" {
			return commands.Reply{Text: "Usage: !sleeper team [@user]"}, nil
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

	if len(e.Args) == 1 || strings.EqualFold(username, e.User.Name) {
		connection, err = getConnectionByUserID(ctx, league.ID, e.Identity.PlatformUserID, sl)
	} else {
		connection, err = getConnectionByUsername(ctx, league.ID, username, sl)
	}

	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Internal error finding the linked account."}, err
	}

	if connection == nil {
		return commands.Reply{Text: fmt.Sprintf("@%s is not linked to a Sleeper account in this league.",
			username)}, nil
	}

	sleeperRosters, err := sl.client.Rosters(ctx, league.SleeperLeagueID)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league rosters from Sleeper."}, err
	}

	if sleeperRosters == nil || len(*sleeperRosters) == 0 {
		return commands.Reply{Text: "No rosters are available for this league yet."}, nil
	}

	rosters := sortStandings(*sleeperRosters)

	var roster *api.Roster
	position := 0

	for i := range rosters {
		if rosters[i].RosterID == connection.SleeperRosterID {
			roster = &rosters[i]
			position = i + 1
			break
		}
	}

	if roster == nil {
		return commands.Reply{Text: fmt.Sprintf("@%s's linked roster could not be found in this league.",
			username)}, nil
	}

	if roster.OwnerID == "" || roster.OwnerID != connection.SleeperUserID {
		return commands.Reply{Text: fmt.Sprintf("@%s's saved link no longer matches the roster owner. Please unlink and link the account again.",
			username)}, nil
	}

	sleeperUsers, err := sl.client.LeagueUsers(ctx, league.SleeperLeagueID)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league members from Sleeper."}, err
	}

	var users []api.LeagueUser
	if sleeperUsers != nil {
		users = *sleeperUsers
	}

	name := getMatchupTeamName(roster.RosterID, rosters, users)

	record := fmt.Sprintf("%d-%d", roster.Settings.Wins, roster.Settings.Losses)
	if roster.Settings.Ties > 0 {
		record = fmt.Sprintf("%s-%d", record, roster.Settings.Ties)
	}

	pointsFor := float64(roster.Settings.Fpts) +
		float64(roster.Settings.FptsDecimal)/100

	pointsAgainst := float64(roster.Settings.FptsAgainst) +
		float64(roster.Settings.FptsAgainstDecimal)/100

	text := fmt.Sprintf("@%s - %s | #%d by record/PF | %s | %.2f PF | %.2f PA",
		username, name, position, record, pointsFor, pointsAgainst)

	state, err := sl.client.NFLState(ctx)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: text + " | Current matchup unavailable."}, nil
	}

	week, message := getCurrentWeek(*league, *state)
	if message != "" {
		return commands.Reply{Text: text + " | " + message}, nil
	}

	sleeperMatchups, err := sl.client.Matchups(ctx, league.SleeperLeagueID, week)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: text + " | Current matchup unavailable."}, nil
	}

	if sleeperMatchups == nil || len(*sleeperMatchups) == 0 {
		return commands.Reply{Text: fmt.Sprintf("%s | Week %d: matchup not available yet.",
			text, week)}, nil
	}

	groups := groupMatchups(*sleeperMatchups)

	for _, group := range groups {
		found := false

		for _, matchup := range group {
			if matchup.RosterID == roster.RosterID {
				found = true
				break
			}
		}

		if !found {
			continue
		}

		switch len(group) {
		case 1:
			return commands.Reply{Text: fmt.Sprintf("%s | Week %d: no opponent listed.",
				text, week)}, nil

		case 2:
			for _, matchup := range group {
				if matchup.RosterID != roster.RosterID {
					opponent := getMatchupTeamName(matchup.RosterID, rosters, users)

					return commands.Reply{Text: fmt.Sprintf("%s | Week %d: vs %s",
						text, week, opponent)}, nil
				}
			}
		}

		return commands.Reply{Text: fmt.Sprintf("%s | Week %d: matchup data is unexpected.",
			text, week)}, nil
	}

	return commands.Reply{Text: fmt.Sprintf("%s | Week %d: matchup not available yet.",
		text, week)}, nil
}
