package sleeper

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// matchup is "!ff matchup [@user] [week]": the score of a team's matchup,
// this week's unless a week is given.
func matchup(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	const form = "matchup [@user] [week]"
	if len(e.Args) > 3 {
		return usage(e, form), nil
	}
	name, self := targetUser(e, e.Args[1:])
	week := 0
	for _, a := range e.Args[1:] {
		if strings.HasPrefix(a, "@") {
			continue
		}
		w, err := strconv.Atoi(a)
		if err != nil || w < 1 || w > 18 {
			return usage(e, form+" (a week from 1 to 18)"), nil
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
	current, message := getCurrentWeek(*league, *state)
	switch {
	case week == 0 && message != "":
		return say("%s Ask for a week: %sff %s.", message, e.Prefix, form), nil
	case week == 0:
		week = current
	case message == "" && week > current:
		return say("Week %d hasn't started yet.", week), nil
	}

	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	roster, reply, err := sl.member(ctx, e, league, name, self, rosters)
	if reply != nil {
		return *reply, err
	}
	matchups, err := sl.client.Matchups(ctx, league.SleeperLeagueID, week)
	if err != nil {
		return unreachable(), err
	}
	n := sl.names(ctx, league, rosters, users, nil)

	for _, g := range api.GroupMatchups(*matchups) {
		mine := -1
		for i := range g {
			if g[i].RosterID == roster.RosterID {
				mine = i
			}
		}
		switch {
		case mine < 0:
			continue
		case len(g) == 1:
			return say("Week %d: %s %.2f, with no opponent.", week, n.Who(roster.RosterID), api.Points(g[0])), nil
		case len(g) == 2:
			other := g[1-mine]
			return say("Week %d: %s %.2f vs %s %.2f", week, n.Who(roster.RosterID), api.Points(g[mine]), n.Who(other.RosterID), api.Points(other)), nil
		default:
			return unreachable(), fmt.Errorf("Sleeper: week %d has a matchup of %d teams", week, len(g))
		}
	}
	return say("%s has no matchup in week %d.", n.Who(roster.RosterID), week), nil
}

func itoa(n int) string { return strconv.Itoa(n) }
