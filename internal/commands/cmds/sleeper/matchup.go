package sleeper

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

func matchup(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return commands.Reply{Text: "Usage: !sleeper matchup [@user]"}, nil
	}

	username := e.User.Name
	if len(e.Args) == 2 {
		username = strings.TrimPrefix(strings.TrimSpace(e.Args[1]), "@")
		if username == "" {
			return commands.Reply{Text: "Usage: !sleeper matchup [@user]"}, nil
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

	sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league rosters from Sleeper."}, err
	}

	if sleeperRosters == nil || len(*sleeperRosters) == 0 {
		return commands.Reply{Text: "No rosters are available for this league yet."}, nil
	}

	rosters := *sleeperRosters

	var roster *SleeperRoster

	for i := range rosters {
		if rosters[i].RosterID == connection.SleeperRosterID {
			roster = &rosters[i]
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

	state, err := getNFLState(ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get the current week from Sleeper."}, err
	}

	week, message := getCurrentWeek(*league, *state)
	if message != "" {
		return commands.Reply{Text: message}, nil
	}

	sleeperMatchups, err := getMatchups(league.SleeperLeagueID, week, ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get matchup scores from Sleeper."}, err
	}

	if sleeperMatchups == nil || len(*sleeperMatchups) == 0 {
		return commands.Reply{Text: fmt.Sprintf("No matchup scores are available for Week %d yet.",
			week)}, nil
	}

	var userMatchup *SleeperMatchup
	var opponentMatchup *SleeperMatchup

	groups := groupMatchups(*sleeperMatchups)

	for _, group := range groups {
		for i := range group {
			if group[i].RosterID == roster.RosterID {
				userMatchup = &group[i]
				break
			}
		}

		if userMatchup == nil {
			continue
		}

		if len(group) > 2 {
			return commands.Reply{Text: "Sleeper returned an unsupported matchup with more than two teams."}, nil
		}

		for i := range group {
			if group[i].RosterID != roster.RosterID {
				opponentMatchup = &group[i]
				break
			}
		}

		if len(group) == 2 && opponentMatchup == nil {
			return commands.Reply{Text: "Sleeper returned unexpected matchup data."}, nil
		}

		break
	}

	if userMatchup == nil {
		return commands.Reply{Text: fmt.Sprintf("No matchup is available for @%s in Week %d yet.",
			username, week)}, nil
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

	name := getMatchupTeamName(roster.RosterID, rosters, users)
	points := getMatchupPoints(*userMatchup)

	if opponentMatchup == nil {
		return commands.Reply{Text: fmt.Sprintf("Week %d: %s %.2f | No opponent listed.",
			week, name, points)}, nil
	}

	opponentName := getMatchupTeamName(opponentMatchup.RosterID, rosters, users)
	opponentPoints := getMatchupPoints(*opponentMatchup)

	return commands.Reply{Text: fmt.Sprintf("Week %d: %s %.2f vs %s %.2f",
		week, name, points, opponentName, opponentPoints)}, nil
}
