package sleeper

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

func highscore(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return adapter.Response{Text: "Usage: !sleeper highscore [week]"}, nil
	}

	week := 0
	if len(e.Args) == 2 {
		requestedWeek, err := strconv.Atoi(e.Args[1])
		if err != nil || requestedWeek < 1 || requestedWeek > 18 {
			return adapter.Response{Text: "Week must be a number from 1 to 18."}, nil
		}
		week = requestedWeek
	}

	league, err := getLeagueByChannel(ctx, e.ChannelID, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Internal error getting league information."}, err
	}

	if league == nil {
		return adapter.Response{Text: "This channel is not associated with any league."}, nil
	}

	state, err := getNFLState(ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get the current season from Sleeper."}, err
	}

	leagueSeason, err := strconv.Atoi(league.Season)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "This league has an invalid saved season."}, err
	}

	currentSeason, err := strconv.Atoi(state.Season)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Sleeper returned an invalid NFL season."}, err
	}

	if leagueSeason > currentSeason {
		return adapter.Response{Text: "Scores are not available for this league's season yet."}, nil
	}

	currentWeek, message := getCurrentWeek(*league, *state)

	if week == 0 {
		if message != "" {
			return adapter.Response{Text: "Please specify a week: !sleeper highscore <week>."}, nil
		}

		if currentWeek <= 1 {
			return adapter.Response{Text: "No completed week is available yet."}, nil
		}

		week = currentWeek - 1
	}

	soFar := false

	if leagueSeason == currentSeason {
		if state.SeasonType == "pre" {
			return adapter.Response{Text: "Regular-season scores are not available yet."}, nil
		}

		if state.SeasonType == "regular" {
			if currentWeek < 1 {
				return adapter.Response{Text: "The current NFL week is not available yet."}, nil
			}

			if week > currentWeek {
				return adapter.Response{Text: "That week has not started yet."}, nil
			}

			soFar = week == currentWeek
		} else if state.SeasonType != "post" {
			return adapter.Response{Text: "Unable to determine which weeks are available for this season."}, nil
		}
	}

	sleeperMatchups, err := getMatchups(league.SleeperLeagueID, week, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get matchup scores from Sleeper."}, err
	}

	if sleeperMatchups == nil || len(*sleeperMatchups) == 0 {
		return adapter.Response{Text: fmt.Sprintf("No scores are available for Week %d.",
			week)}, nil
	}

	var leaders []int
	var highestPoints int64
	seen := make(map[int]bool)

	for _, matchup := range *sleeperMatchups {
		// Compare scores at the same two-decimal precision used in chat.
		points := int64(math.Round(getMatchupPoints(matchup) * 100))

		if len(leaders) == 0 || points > highestPoints {
			highestPoints = points
			leaders = []int{matchup.RosterID}
			seen = map[int]bool{matchup.RosterID: true}
			continue
		}

		if points == highestPoints && !seen[matchup.RosterID] {
			leaders = append(leaders, matchup.RosterID)
			seen[matchup.RosterID] = true
		}
	}

	sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get league rosters from Sleeper."}, err
	}

	sleeperUsers, err := getPlayersInLeague(league.SleeperLeagueID, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get league members from Sleeper."}, err
	}

	var rosters []SleeperRoster
	if sleeperRosters != nil {
		rosters = *sleeperRosters
	}

	var users []SleeperLeagueUser
	if sleeperUsers != nil {
		users = *sleeperUsers
	}

	var names []string

	for _, rosterID := range leaders {
		names = append(names, getMatchupTeamName(rosterID, rosters, users))
	}

	label := "High Score"
	if soFar {
		label = "High Score so far"
	}

	return adapter.Response{Text: fmt.Sprintf("Week %d %s: %s - %.2f points.",
		week, label, strings.Join(names, ", "), float64(highestPoints)/100)}, nil
}
