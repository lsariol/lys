package sleeper

import (
	"context"
	"fmt"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// scores is "!ff scores": every matchup this week.
func scores(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
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
	week, message := getCurrentWeek(*league, *state)
	if message != "" {
		return say("%s", message), nil
	}
	matchups, err := sl.client.Matchups(ctx, league.SleeperLeagueID, week)
	if err != nil {
		return unreachable(), err
	}
	if len(*matchups) == 0 {
		return say("No scores for week %d yet.", week), nil
	}
	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	n := sl.names(ctx, league, rosters, users, nil)

	var entries []string
	for _, g := range api.GroupMatchups(*matchups) {
		switch len(g) {
		case 1:
			entries = append(entries, fmt.Sprintf("%s %.2f (no opponent)", n.Who(g[0].RosterID), api.Points(g[0])))
		case 2:
			entries = append(entries, fmt.Sprintf("%s %.2f vs %s %.2f", n.Who(g[0].RosterID), api.Points(g[0]), n.Who(g[1].RosterID), api.Points(g[1])))
		default:
			return unreachable(), fmt.Errorf("Sleeper: week %d has a matchup of %d teams", week, len(g))
		}
	}
	return say("Week %d scores: %s", week, strings.Join(entries, " · ")), nil
}
