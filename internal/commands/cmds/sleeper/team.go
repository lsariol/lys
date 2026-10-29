package sleeper

import (
	"context"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// team is "!ff team [@user]": the team's place, record, points and this week's opponent.
func team(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	if len(e.Args) > 2 {
		return usage(e, "team [@user]"), nil
	}
	name, self := targetUser(e, e.Args[1:])

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
	roster, reply, err := sl.member(ctx, e, league, name, self, rosters)
	if reply != nil {
		return *reply, err
	}
	n := sl.names(ctx, league, rosters, users, nil)

	place := 0
	for i, r := range sortStandings(rosters) {
		if r.RosterID == roster.RosterID {
			place = i + 1
		}
	}
	pf, pa := pointsFor(roster.Settings)
	text := say("%s: %s · %s (%s) · %.2f PF · %.2f PA", n.Who(roster.RosterID), n.Team(roster.RosterID), api.Ordinal(place),
		api.Record(roster.Settings.Wins, roster.Settings.Losses, roster.Settings.Ties), pf, pa).Text

	state, err := sl.client.NFLState(ctx)
	if err != nil {
		return commands.Reply{Text: text}, err
	}
	week, message := getCurrentWeek(*league, *state)
	if message != "" {
		return commands.Reply{Text: text}, nil
	}
	matchups, err := sl.client.Matchups(ctx, league.SleeperLeagueID, week)
	if err != nil {
		return commands.Reply{Text: text}, err
	}
	for _, g := range api.GroupMatchups(*matchups) {
		if len(g) == 2 && (g[0].RosterID == roster.RosterID || g[1].RosterID == roster.RosterID) {
			opponent := g[0]
			if opponent.RosterID == roster.RosterID {
				opponent = g[1]
			}
			text += " · Week " + itoa(week) + " vs " + n.Who(opponent.RosterID)
		}
	}
	return commands.Reply{Text: text}, nil
}
