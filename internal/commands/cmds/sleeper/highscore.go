package sleeper

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

func highscore(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return commands.Reply{Text: "Usage: !sleeper highscore [week]"}, nil
	}

	week := 0
	if len(e.Args) == 2 {
		requestedWeek, err := strconv.Atoi(e.Args[1])
		if err != nil || requestedWeek < 1 || requestedWeek > 18 {
			return commands.Reply{Text: "Week must be a number from 1 to 18."}, nil
		}
		week = requestedWeek
	}

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Internal error getting league information."}, err
	}

	if league == nil {
		return commands.Reply{Text: "This channel is not associated with any league."}, nil
	}

	state, err := sl.client.NFLState(ctx)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get the current season from Sleeper."}, err
	}

	leagueSeason, err := strconv.Atoi(league.Season)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "This league has an invalid saved season."}, err
	}

	currentSeason, err := strconv.Atoi(state.Season)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Sleeper returned an invalid NFL season."}, err
	}

	if leagueSeason > currentSeason {
		return commands.Reply{Text: "Scores are not available for this league's season yet."}, nil
	}

	currentWeek, message := getCurrentWeek(*league, *state)

	if week == 0 {
		if message != "" {
			return commands.Reply{Text: "Please specify a week: !sleeper highscore <week>."}, nil
		}

		if currentWeek <= 1 {
			return commands.Reply{Text: "No completed week is available yet."}, nil
		}

		week = currentWeek - 1
	}

	soFar := false

	if leagueSeason == currentSeason {
		if state.SeasonType == "pre" {
			return commands.Reply{Text: "Regular-season scores are not available yet."}, nil
		}

		if state.SeasonType == "regular" {
			if currentWeek < 1 {
				return commands.Reply{Text: "The current NFL week is not available yet."}, nil
			}

			if week > currentWeek {
				return commands.Reply{Text: "That week has not started yet."}, nil
			}

			soFar = week == currentWeek
		} else if state.SeasonType != "post" {
			return commands.Reply{Text: "Unable to determine which weeks are available for this season."}, nil
		}
	}

	sleeperMatchups, err := sl.client.Matchups(ctx, league.SleeperLeagueID, week)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get matchup scores from Sleeper."}, err
	}

	if sleeperMatchups == nil || len(*sleeperMatchups) == 0 {
		return commands.Reply{Text: fmt.Sprintf("No scores are available for Week %d.",
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

	sleeperRosters, err := sl.client.Rosters(ctx, league.SleeperLeagueID)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league rosters from Sleeper."}, err
	}

	sleeperUsers, err := sl.client.LeagueUsers(ctx, league.SleeperLeagueID)
	if err != nil {
		log.Print(err)
		return commands.Reply{Text: "Unable to get league members from Sleeper."}, err
	}

	var rosters []api.Roster
	if sleeperRosters != nil {
		rosters = *sleeperRosters
	}

	var users []api.LeagueUser
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

	return commands.Reply{Text: fmt.Sprintf("Week %d %s: %s - %.2f points.",
		week, label, strings.Join(names, ", "), float64(highestPoints)/100)}, nil
}
