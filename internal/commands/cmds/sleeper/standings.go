package sleeper

import (
	"context"
	"fmt"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// standings is "!ff standings": every team by record, then points scored.
func standings(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	if len(rosters) == 0 {
		return say("The league has no teams yet."), nil
	}
	n := sl.names(ctx, league, rosters, users, nil)

	var entries []string
	for i, r := range sortStandings(rosters) {
		pf, _ := pointsFor(r.Settings)
		entries = append(entries, fmt.Sprintf("%d. %s %s, %.2f PF", i+1, n.TeamAndWho(r.RosterID),
			api.Record(r.Settings.Wins, r.Settings.Losses, r.Settings.Ties), pf))
	}
	return say("Standings: %s", strings.Join(entries, " · ")), nil
}
