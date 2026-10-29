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

// playoffs is "!ff playoffs": this playoff round's games, or the champion.
func playoffs(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	sleeperLeague, err := sl.client.League(ctx, league.SleeperLeagueID)
	if err != nil {
		return unreachable(), err
	}
	settings := sleeperLeague.Settings
	if settings.PlayoffWeekStart < 1 || settings.PlayoffTeams < 2 {
		return say("This league has no playoffs set up."), nil
	}
	if settings.PlayoffRoundType == nil || *settings.PlayoffRoundType != 0 {
		return say("Only one-week playoff rounds are supported so far."), nil
	}

	sleeperBracket, err := sl.client.PlayoffBracket(ctx, league.SleeperLeagueID)
	if err != nil {
		return unreachable(), err
	}
	bracket := *sleeperBracket
	championship := getChampionship(bracket)

	var state api.NFLState
	if championship == nil || championship.Winner == nil || *championship.Winner <= 0 {
		nflState, err := sl.client.NFLState(ctx)
		if err != nil {
			return unreachable(), err
		}
		state = *nflState
	}
	round, week, message := getPlayoffRound(*sleeperLeague, state, bracket)
	if message != "" {
		return say("%s", message), nil
	}
	var games []api.PlayoffMatchup
	for _, game := range bracket {
		if game.Round == round {
			games = append(games, game)
		}
	}
	sort.Slice(games, func(i, j int) bool { return games[i].MatchID < games[j].MatchID })
	if len(games) == 0 {
		return say("This playoff round isn't set yet."), nil
	}

	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	n := sl.names(ctx, league, rosters, users, nil)
	var matchups []api.Matchup
	if m, err := sl.client.Matchups(ctx, league.SleeperLeagueID, week); err != nil {
		log.Printf("[Sleeper] %v", err)
	} else {
		matchups = *m
	}

	var entries []string
	for _, game := range games {
		first := resolvePlayoffTeam(game.Team1, game.Team1From, bracket)
		second := resolvePlayoffTeam(game.Team2, game.Team2From, bracket)
		label := ""
		if championship != nil && game.MatchID == championship.MatchID {
			label = "Championship: "
		} else if game.Place != nil {
			label = fmt.Sprintf("For %s place: ", api.Ordinal(*game.Place))
		}
		entry := label + formatPlayoffTeam(first, n, matchups) + " vs " + formatPlayoffTeam(second, n, matchups)
		if game.Winner != nil && *game.Winner > 0 {
			entry += ", won by " + n.Who(*game.Winner)
		}
		entries = append(entries, entry)
	}

	heading := fmt.Sprintf("Playoffs, round %d (week %d)", round, week)
	if championship != nil && championship.Winner != nil && *championship.Winner > 0 {
		heading = fmt.Sprintf("Champion: %s · Week %d", n.Who(*championship.Winner), week)
	}
	return say("%s: %s", heading, strings.Join(entries, " · ")), nil
}

func getChampionship(bracket []api.PlayoffMatchup) *api.PlayoffMatchup {

	lastRound := 0
	for i := range bracket {
		if bracket[i].Place != nil && *bracket[i].Place == 1 {
			return &bracket[i]
		}
		if bracket[i].Round > lastRound {
			lastRound = bracket[i].Round
		}
	}
	var championship *api.PlayoffMatchup
	for i := range bracket {
		if bracket[i].Round == lastRound && bracket[i].Place == nil {
			if championship != nil {
				return nil
			}
			championship = &bracket[i]
		}
	}
	return championship
}

func getPlayoffRound(league api.League, state api.NFLState, bracket []api.PlayoffMatchup) (int, int, string) {

	settings := league.Settings
	if settings.PlayoffRoundType == nil || *settings.PlayoffRoundType != 0 {
		return 0, 0, "This playoff round format is not supported yet."
	}
	championship := getChampionship(bracket)
	if championship != nil && championship.Winner != nil && *championship.Winner > 0 {
		week := settings.PlayoffWeekStart + championship.Round - 1
		if week < 1 || week > 18 {
			return 0, 0, "Sleeper returned an invalid playoff schedule."
		}
		return championship.Round, week, ""
	}
	if league.Season != state.Season {
		return 0, 0, "This league is not in the current NFL season, and no championship result is available."
	}
	if state.SeasonType == "pre" || (state.SeasonType == "regular" && state.Week < settings.PlayoffWeekStart) {
		return 0, 0, fmt.Sprintf("Playoffs begin in Week %d.", settings.PlayoffWeekStart)
	}
	if len(bracket) == 0 {
		return 0, 0, "The playoff bracket is not available yet."
	}
	if state.SeasonType != "regular" || state.Week < 1 || state.Week > 18 {
		return 0, 0, "No current playoff week or completed championship result is available."
	}
	return state.Week - settings.PlayoffWeekStart + 1, state.Week, ""
}

func resolvePlayoffTeam(team *int, source *api.PlayoffSource, bracket []api.PlayoffMatchup) *int {

	if team != nil && *team > 0 {
		return team
	}
	if source == nil {
		return nil
	}
	for _, game := range bracket {
		if source.Winner != nil && game.MatchID == *source.Winner {
			return game.Winner
		}
		if source.Loser != nil && game.MatchID == *source.Loser {
			return game.Loser
		}
	}
	return nil
}

func formatPlayoffTeam(team *int, n api.Names, matchups []api.Matchup) string {
	if team == nil || *team <= 0 {
		return "TBD"
	}
	for _, m := range matchups {
		if m.RosterID == *team {
			return fmt.Sprintf("%s %.2f", n.Who(*team), api.Points(m))
		}
	}
	return n.Who(*team)
}
