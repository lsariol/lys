package sleeper

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// say is a Sleeper reply; every one starts with 🏈.
func say(format string, args ...any) commands.Reply {
	return commands.Reply{Text: "🏈 " + fmt.Sprintf(format, args...)}
}

// usage answers wrong arguments with the subcommand's form, such as "team [@user]".
func usage(e commands.Request, form string) commands.Reply {
	return say("Usage: %sff %s", e.Prefix, form)
}

func noLeague(e commands.Request) commands.Reply {
	return say("This channel has no Sleeper league linked. The channel's owner can link one with %sff link league <league URL>.", e.Prefix)
}

// unreachable answers when Sleeper fails or gives an answer Lys can't use.
func unreachable() commands.Reply { return say("Couldn't reach Sleeper, try again in a moment.") }

// broken answers when Lys's database fails.
func broken() commands.Reply { return say("Something went wrong, try again in a moment.") }

// names gives the league's rosters their chat names; linked viewers that
// can't be read are logged and show as team names.
func (sl *Sleeper) names(ctx context.Context, league *SleeperLeague, rosters []api.Roster, users []api.LeagueUser, players map[string]api.Player) api.Names {
	mentions, err := api.Mentions(ctx, sl.db, league.SleeperLeagueID)
	if err != nil {
		log.Printf("[Sleeper] %v", err)
	}
	return api.NamesFrom(rosters, users, mentions, players)
}

// leagueData is a league's rosters and users from Sleeper.
func (sl *Sleeper) leagueData(ctx context.Context, league *SleeperLeague) ([]api.Roster, []api.LeagueUser, error) {
	rosters, err := sl.client.Rosters(ctx, league.SleeperLeagueID)
	if err != nil {
		return nil, nil, err
	}
	users, err := sl.client.LeagueUsers(ctx, league.SleeperLeagueID)
	if err != nil {
		return nil, nil, err
	}
	return *rosters, *users, nil
}

// targetUser is the viewer a subcommand is about: the @user in args, or the sender.
func targetUser(e commands.Request, args []string) (name string, self bool) {
	for _, a := range args {
		if strings.HasPrefix(a, "@") && len(a) > 1 {
			name := strings.TrimPrefix(a, "@")
			return name, strings.EqualFold(name, e.User.Name)
		}
	}
	return e.User.Name, true
}

// member finds the linked viewer's roster. When it can't, reply says why.
func (sl *Sleeper) member(ctx context.Context, e commands.Request, league *SleeperLeague, name string, self bool, rosters []api.Roster) (roster *api.Roster, reply *commands.Reply, err error) {
	var c *SleeperConnection
	if self {
		c, err = getConnectionByUserID(ctx, league.ID, e.Identity.PlatformUserID, sl)
	} else {
		c, err = getConnectionByUsername(ctx, league.ID, name, sl)
	}
	if err != nil {
		r := broken()
		return nil, &r, err
	}
	if c == nil {
		r := say("@%s hasn't linked a team in this league. They can with %sff link <Sleeper name>.", name, e.Prefix)
		if self {
			r = say("@%s, you haven't linked a team yet. Link yours with %sff link <Sleeper name>.", name, e.Prefix)
		}
		return nil, &r, nil
	}
	for i := range rosters {
		if rosters[i].RosterID != c.SleeperRosterID {
			continue
		}
		if rosters[i].OwnerID == "" || rosters[i].OwnerID != c.SleeperUserID {
			r := say("@%s's team has a new owner on Sleeper. Unlink with %sff unlink, then link again.", name, e.Prefix)
			return nil, &r, nil
		}
		return &rosters[i], nil, nil
	}
	r := say("@%s's linked team isn't in the league anymore.", name)
	return nil, &r, nil
}

func getCurrentWeek(league SleeperLeague, state api.NFLState) (int, string) {
	if league.Season != state.Season {
		return 0, "This league isn't in the current NFL season."
	}
	if state.SeasonType != "regular" || state.Week < 1 {
		return 0, "There's no NFL regular-season week on right now."
	}
	return state.Week, ""
}

func getWinningPercentage(settings api.RosterSettings) float64 {
	games := settings.Wins + settings.Losses + settings.Ties
	if games == 0 {
		return 0
	}
	return (float64(settings.Wins) + float64(settings.Ties)/2) / float64(games)
}

// sortStandings orders rosters by record, then points scored.
func sortStandings(rosters []api.Roster) []api.Roster {
	sorted := append([]api.Roster(nil), rosters...)
	sort.Slice(sorted, func(i, j int) bool {
		first, second := sorted[i].Settings, sorted[j].Settings
		if a, b := getWinningPercentage(first), getWinningPercentage(second); a != b {
			return a > b
		}
		if a, b := int64(first.Fpts)*100+int64(first.FptsDecimal), int64(second.Fpts)*100+int64(second.FptsDecimal); a != b {
			return a > b
		}
		return sorted[i].RosterID < sorted[j].RosterID
	})
	return sorted
}

// pointsFor is a roster's season points scored, and against.
func pointsFor(s api.RosterSettings) (float64, float64) {
	return float64(s.Fpts) + float64(s.FptsDecimal)/100, float64(s.FptsAgainst) + float64(s.FptsAgainstDecimal)/100
}
