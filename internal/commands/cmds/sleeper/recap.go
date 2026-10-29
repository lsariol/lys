package sleeper

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// recap is "!ff recap [week]": a week's results, highlights and standings,
// the last finished week's by default. It's the recap Lys posts on Tuesdays.
func recap(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	if len(e.Args) > 2 {
		return usage(e, "recap [week]"), nil
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

	week := 0
	if len(e.Args) == 2 {
		w, err := strconv.Atoi(e.Args[1])
		if err != nil || w < 1 || w > 18 {
			return usage(e, "recap [week] (a week from 1 to 18)"), nil
		}
		if message == "" && w > current {
			return say("Week %d hasn't started yet.", w), nil
		}
		week = w
	} else {
		if message != "" {
			return usage(e, "recap <week>"), nil
		}
		week = current
		// This week is finished once its last game has (RecapDue's rule).
		games, err := sl.client.Games(ctx, state.Season, week)
		if err != nil || len(games) == 0 || time.Now().Before(games[len(games)-1].Kickoff.Add(4*time.Hour)) {
			week--
		}
		if week < 1 {
			return say("No week has finished yet."), nil
		}
	}

	texts, err := sl.client.Recap(ctx, league.SleeperLeagueID, week, sl.client.LoadNames(ctx, sl.db, league.SleeperLeagueID))
	switch {
	case errors.Is(err, api.ErrNoScores):
		return say("Week %d has no scores yet.", week), nil
	case errors.Is(err, api.ErrNoGames):
		return say("%s has no games in week %d.", league.LeagueName, week), nil
	case err != nil:
		return unreachable(), err
	}
	return commands.Reply{Text: texts[0], More: texts[1:]}, nil
}
