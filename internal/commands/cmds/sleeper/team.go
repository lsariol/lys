package sleeper

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

func team(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return adapter.Response{Text: "Usage: !sleeper team [@user]"}, nil
	}

	username := e.Username
	if len(e.Args) == 2 {
		username = strings.TrimPrefix(strings.TrimSpace(e.Args[1]), "@")
		if username == "" {
			return adapter.Response{Text: "Usage: !sleeper team [@user]"}, nil
		}
	}

	league, err := getLeagueByChannel(ctx, e.ChannelID, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Internal error getting league information."}, err
	}

	if league == nil {
		return adapter.Response{Text: "This channel is not associated with any league."}, nil
	}

	var connection *SleeperConnection

	if len(e.Args) == 1 || strings.EqualFold(username, e.Username) {
		connection, err = getConnectionByUserID(ctx, league.ID, e.UserID, deps)
	} else {
		connection, err = getConnectionByUsername(ctx, league.ID, username, deps)
	}

	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Internal error finding the linked account."}, err
	}

	if connection == nil {
		return adapter.Response{Text: fmt.Sprintf("@%s is not linked to a Sleeper account in this league.",
			username)}, nil
	}

	sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get league rosters from Sleeper."}, err
	}

	if sleeperRosters == nil || len(*sleeperRosters) == 0 {
		return adapter.Response{Text: "No rosters are available for this league yet."}, nil
	}

	rosters := sortStandings(*sleeperRosters)

	var roster *SleeperRoster
	position := 0

	for i := range rosters {
		if rosters[i].RosterID == connection.SleeperRosterID {
			roster = &rosters[i]
			position = i + 1
			break
		}
	}

	if roster == nil {
		return adapter.Response{Text: fmt.Sprintf("@%s's linked roster could not be found in this league.",
			username)}, nil
	}

	if roster.OwnerID == "" || roster.OwnerID != connection.SleeperUserID {
		return adapter.Response{Text: fmt.Sprintf("@%s's saved link no longer matches the roster owner. Please unlink and link the account again.",
			username)}, nil
	}

	sleeperUsers, err := getPlayersInLeague(league.SleeperLeagueID, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get league members from Sleeper."}, err
	}

	var users []SleeperLeagueUser
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

	state, err := getNFLState(ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: text + " | Current matchup unavailable."}, nil
	}

	week, message := getCurrentWeek(*league, *state)
	if message != "" {
		return adapter.Response{Text: text + " | " + message}, nil
	}

	sleeperMatchups, err := getMatchups(league.SleeperLeagueID, week, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: text + " | Current matchup unavailable."}, nil
	}

	if sleeperMatchups == nil || len(*sleeperMatchups) == 0 {
		return adapter.Response{Text: fmt.Sprintf("%s | Week %d: matchup not available yet.",
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
			return adapter.Response{Text: fmt.Sprintf("%s | Week %d: no opponent listed.",
				text, week)}, nil

		case 2:
			for _, matchup := range group {
				if matchup.RosterID != roster.RosterID {
					opponent := getMatchupTeamName(matchup.RosterID, rosters, users)

					return adapter.Response{Text: fmt.Sprintf("%s | Week %d: vs %s",
						text, week, opponent)}, nil
				}
			}
		}

		return adapter.Response{Text: fmt.Sprintf("%s | Week %d: matchup data is unexpected.",
			text, week)}, nil
	}

	return adapter.Response{Text: fmt.Sprintf("%s | Week %d: matchup not available yet.",
		text, week)}, nil
}
