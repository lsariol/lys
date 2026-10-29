package sleeper

import (
	"context"
	"math"
	"strconv"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// highscore is "!ff highscore [week]": the week's top score, last week's by
// default (this week's so far when asked for it).
func highscore(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	if len(e.Args) > 2 {
		return usage(e, "highscore [week]"), nil
	}
	week := 0
	if len(e.Args) == 2 {
		w, err := strconv.Atoi(e.Args[1])
		if err != nil || w < 1 || w > 18 {
			return usage(e, "highscore [week] (a week from 1 to 18)"), nil
		}
		week = w
	}

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	state, err := sl.client.NFLState(ctx)
	if err != nil {
		return unreachable(), err
	}
	leagueSeason, err := strconv.Atoi(league.Season)
	if err != nil {
		return broken(), err
	}
	currentSeason, err := strconv.Atoi(state.Season)
	if err != nil {
		return unreachable(), err
	}
	if leagueSeason > currentSeason {
		return say("This league's season hasn't started yet."), nil
	}

	current, message := getCurrentWeek(*league, *state)
	if week == 0 {
		if message != "" {
			return usage(e, "highscore <week>"), nil
		}
		if current <= 1 {
			return say("No week has finished yet."), nil
		}
		week = current - 1
	}

	soFar := false
	if leagueSeason == currentSeason {
		switch state.SeasonType {
		case "pre":
			return say("The regular season hasn't started yet."), nil
		case "regular":
			if week > current {
				return say("Week %d hasn't started yet.", week), nil
			}
			soFar = week == current
		}
	}

	matchups, err := sl.client.Matchups(ctx, league.SleeperLeagueID, week)
	if err != nil {
		return unreachable(), err
	}
	if len(*matchups) == 0 {
		return say("No scores for week %d.", week), nil
	}

	var leaders []int
	var highest int64
	for _, m := range *matchups {
		// Compare at the two decimals chat shows.
		points := int64(math.Round(api.Points(m) * 100))
		switch {
		case len(leaders) == 0 || points > highest:
			highest, leaders = points, []int{m.RosterID}
		case points == highest:
			leaders = append(leaders, m.RosterID)
		}
	}

	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	n := sl.names(ctx, league, rosters, users, nil)
	var names []string
	for _, id := range leaders {
		names = append(names, n.Who(id))
	}
	label := "high score"
	if soFar {
		label = "high score so far"
	}
	return say("Week %d %s: %s, %.2f points.", week, label, api.List(names), float64(highest)/100), nil
}
