package sleeper

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

func standings(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return adapter.Response{Text: "Usage: !sleeper standings [page]"}, nil
	}

	page := 1
	if len(e.Args) == 2 {
		requestedPage, err := strconv.Atoi(e.Args[1])
		if err != nil || requestedPage < 1 {
			return adapter.Response{Text: "Usage: !sleeper standings [page]. Page must be a positive number."}, nil
		}
		page = requestedPage
	}

	league, err := getLeagueByChannel(ctx, e.ChannelID, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Internal error getting league standings."}, err
	}

	if league == nil {
		return adapter.Response{Text: "This channel is not associated with any league."}, nil
	}

	sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get standings from Sleeper."}, err
	}

	if sleeperRosters == nil || len(*sleeperRosters) == 0 {
		return adapter.Response{Text: "No rosters are available for this league yet."}, nil
	}

	const pageSize = 4
	totalPages := (len(*sleeperRosters) + pageSize - 1) / pageSize

	if page > totalPages {
		return adapter.Response{Text: fmt.Sprintf("Standings has %d page(s).", totalPages)}, nil
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

	rosters := sortStandings(*sleeperRosters)

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(rosters) {
		end = len(rosters)
	}

	var entries []string

	for i := start; i < end; i++ {
		roster := rosters[i]

		name := getTeamName(roster, users)
		name = strings.Join(strings.Fields(name), " ")

		nameRunes := []rune(name)
		if len(nameRunes) > 40 {
			name = string(nameRunes[:37]) + "..."
		}

		record := fmt.Sprintf("%d-%d", roster.Settings.Wins, roster.Settings.Losses)
		if roster.Settings.Ties > 0 {
			record = fmt.Sprintf("%s-%d", record, roster.Settings.Ties)
		}

		points := float64(roster.Settings.Fpts) + float64(roster.Settings.FptsDecimal)/100

		entries = append(entries, fmt.Sprintf("%d. %s %s (%.2f PF)",
			i+1, name, record, points))
	}

	return adapter.Response{Text: fmt.Sprintf("Standings by record/PF (%d/%d): %s",
		page, totalPages, strings.Join(entries, " | "))}, nil
}
