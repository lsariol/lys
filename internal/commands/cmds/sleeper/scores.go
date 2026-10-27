package sleeper

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

func scores(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return commands.Reply{Text: "Usage: !sleeper scores [page]"}, nil
	}

	page := 1
	if len(e.Args) == 2 {
		requestedPage, err := strconv.Atoi(e.Args[1])
		if err != nil || requestedPage < 1 {
			return commands.Reply{Text: "Usage: !sleeper scores [page]. Page must be a positive number."}, nil
		}
		page = requestedPage
	}

	league, err := getLeagueByChannel(ctx, e.Channel.ID, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Internal error getting league scores."}, err
	}
	if league == nil {
		return commands.Reply{Text: "This channel is not associated with any league."}, nil
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
		return commands.Reply{Text: fmt.Sprintf("No matchup scores are available for Week %d yet.", week)}, nil
	}

	groups := groupMatchups(*sleeperMatchups)
	const pageSize = 3
	totalPages := (len(groups) + pageSize - 1) / pageSize
	if page > totalPages {
		return commands.Reply{Text: fmt.Sprintf("Scores has %d page(s).", totalPages)}, nil
	}

	sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league rosters from Sleeper."}, err
	}

	sleeperUsers, err := getPlayersInLeague(league.SleeperLeagueID, ctx, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league members from Sleeper."}, err
	}

	var rosters []SleeperRoster
	if sleeperRosters != nil {
		rosters = *sleeperRosters
	}
	var users []SleeperLeagueUser
	if sleeperUsers != nil {
		users = *sleeperUsers
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(groups) {
		end = len(groups)
	}

	var entries []string
	for _, group := range groups[start:end] {
		first := group[0]
		firstName := getMatchupTeamName(first.RosterID, rosters, users)

		switch len(group) {
		case 1:
			entries = append(entries, fmt.Sprintf("%s %.2f (no opponent listed)",
				firstName, getMatchupPoints(first)))
		case 2:
			second := group[1]
			secondName := getMatchupTeamName(second.RosterID, rosters, users)
			entries = append(entries, fmt.Sprintf("%s %.2f vs %s %.2f",
				firstName, getMatchupPoints(first), secondName, getMatchupPoints(second)))
		default:
			return commands.Reply{Text: "Sleeper returned an unsupported matchup with more than two teams."}, nil
		}
	}

	return commands.Reply{Text: fmt.Sprintf("Week %d scores (%d/%d): %s",
		week, page, totalPages, strings.Join(entries, " | "))}, nil
}
