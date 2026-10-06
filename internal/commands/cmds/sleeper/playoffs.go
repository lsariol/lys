package sleeper

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

func playoffs(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	if len(e.Args) < 1 || len(e.Args) > 2 {
		return adapter.Response{Text: "Usage: !sleeper playoffs [page]"}, nil
	}
	page := 1
	if len(e.Args) == 2 {
		requestedPage, err := strconv.Atoi(e.Args[1])
		if err != nil || requestedPage < 1 {
			return adapter.Response{Text: "Usage: !sleeper playoffs [page]. Page must be a positive number."}, nil
		}
		page = requestedPage
	}

	league, err := getLeagueByChannel(ctx, e.ChannelID, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Internal error getting league information."}, err
	}
	if league == nil {
		return adapter.Response{Text: "This channel is not associated with any league."}, nil
	}

	sleeperLeague, err := GetLeague(ctx, league.SleeperLeagueID, deps.HTTP)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get playoff settings from Sleeper."}, err
	}
	if sleeperLeague == nil || sleeperLeague.LeagueID == "" || sleeperLeague.Season == "" {
		return adapter.Response{Text: "Sleeper did not return valid league settings."}, nil
	}
	settings := sleeperLeague.Settings
	if settings.PlayoffWeekStart < 1 || settings.PlayoffTeams < 2 {
		return adapter.Response{Text: "Playoffs are not configured for this league."}, nil
	}
	if settings.PlayoffRoundType == nil || *settings.PlayoffRoundType != 0 {
		return adapter.Response{Text: "This playoff round format is not supported yet. Automatic timing currently supports standard one-week rounds."}, nil
	}

	sleeperBracket, err := getPlayoffBracket(league.SleeperLeagueID, ctx, deps)
	if err != nil {
		log.Print(err)
		return adapter.Response{Text: "Unable to get the playoff bracket from Sleeper."}, err
	}
	var bracket []SleeperPlayoffMatchup
	if sleeperBracket != nil {
		bracket = *sleeperBracket
	}
	championship := getChampionship(bracket)

	var state SleeperNFLState
	if championship == nil || championship.Winner == nil || *championship.Winner <= 0 {
		nflState, err := getNFLState(ctx, deps)
		if err != nil {
			log.Print(err)
			return adapter.Response{Text: "Unable to get the current week from Sleeper."}, err
		}
		state = *nflState
	}

	round, week, message := getPlayoffRound(*sleeperLeague, state, bracket)
	if message != "" {
		return adapter.Response{Text: message}, nil
	}
	var games []SleeperPlayoffMatchup
	for _, game := range bracket {
		if game.Round == round {
			games = append(games, game)
		}
	}
	sort.Slice(games, func(i, j int) bool { return games[i].MatchID < games[j].MatchID })
	if len(games) == 0 {
		return adapter.Response{Text: "The current playoff round is not available yet."}, nil
	}
	const pageSize = 3
	totalPages := (len(games) + pageSize - 1) / pageSize
	if page > totalPages {
		return adapter.Response{Text: fmt.Sprintf("Playoffs has %d page(s).", totalPages)}, nil
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

	var matchups []SleeperMatchup
	sleeperMatchups, err := getMatchups(league.SleeperLeagueID, week, ctx, deps)
	if err != nil {
		log.Print(err)
	} else if sleeperMatchups != nil {
		matchups = *sleeperMatchups
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(games) {
		end = len(games)
	}
	var entries []string
	for _, game := range games[start:end] {
		first := resolvePlayoffTeam(game.Team1, game.Team1From, bracket)
		second := resolvePlayoffTeam(game.Team2, game.Team2From, bracket)
		label := ""
		if championship != nil && game.MatchID == championship.MatchID {
			label = "Championship: "
		} else if game.Place != nil {
			label = fmt.Sprintf("Place %d: ", *game.Place)
		}
		entry := label + formatPlayoffTeam(first, rosters, users, matchups) + " vs " +
			formatPlayoffTeam(second, rosters, users, matchups)
		if game.Winner != nil && *game.Winner > 0 {
			entry += " | Winner: " + getMatchupTeamName(*game.Winner, rosters, users)
		}
		entries = append(entries, entry)
	}

	heading := fmt.Sprintf("Playoffs - Round %d, Week %d (%d/%d)", round, week, page, totalPages)
	if championship != nil && championship.Winner != nil && *championship.Winner > 0 {
		heading = fmt.Sprintf("Champion: %s | Week %d (%d/%d)",
			getMatchupTeamName(*championship.Winner, rosters, users), week, page, totalPages)
	}
	return adapter.Response{Text: heading + ": " + strings.Join(entries, " | ")}, nil
}

func getChampionship(bracket []SleeperPlayoffMatchup) *SleeperPlayoffMatchup {

	lastRound := 0
	for i := range bracket {
		if bracket[i].Place != nil && *bracket[i].Place == 1 {
			return &bracket[i]
		}
		if bracket[i].Round > lastRound {
			lastRound = bracket[i].Round
		}
	}
	var championship *SleeperPlayoffMatchup
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

func getPlayoffRound(league League, state SleeperNFLState, bracket []SleeperPlayoffMatchup) (int, int, string) {

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

func resolvePlayoffTeam(team *int, source *SleeperPlayoffSource, bracket []SleeperPlayoffMatchup) *int {

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

func formatPlayoffTeam(team *int, rosters []SleeperRoster, users []SleeperLeagueUser, matchups []SleeperMatchup) string {

	if team == nil || *team <= 0 {
		return "TBD"
	}
	name := getMatchupTeamName(*team, rosters, users)
	for _, matchup := range matchups {
		if matchup.RosterID == *team {
			return fmt.Sprintf("%s %.2f", name, getMatchupPoints(matchup))
		}
	}
	return name + " (score unavailable)"
}
