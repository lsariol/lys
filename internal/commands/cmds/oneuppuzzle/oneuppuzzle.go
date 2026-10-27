package oneuppuzzle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/commands"
)

// OneUpPuzzle records finished OneUpPuzzle games from chat and reports stats.
type OneUpPuzzle struct {
	store *store
}

func New(pool *pgxpool.Pool) *OneUpPuzzle { return &OneUpPuzzle{store: &store{pool: pool}} }

func (*OneUpPuzzle) Spec() commands.Spec {
	return commands.Spec{
		Name:        "oneuppuzzle",
		Aliases:     []string{"oneup"},
		Patterns:    []string{`Congratulations,\s*you finished puzzle #\d+\s+in\s+(?:\d+\s*hour[s]?,\s*)?(?:\d+\s*minute[s]?,\s*)?(?:\d+\s*second[s]?)`},
		Description: "command for oneuppuzzle statistics.",
		Usage:       "oneuppuzzle <none> <help> <leaders> <username> <stats>",
		Timeout:     3 * time.Second,
	}
}

func (o *OneUpPuzzle) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	if e.Pattern {
		//store in DB
		game, err := o.validateGameEntry(ctx, e)
		if errors.Is(err, errAlreadyRecorded) {
			return commands.Reply{}, nil
		}
		if err != nil {
			return commands.Reply{}, err
		}

		if 3600 <= game.Details.TimeSeconds || game.Details.TimeSeconds <= 20 {
			err = o.store.storeGame(ctx, game, true)
			if err != nil {
				return commands.Reply{Text: "An error occured while storing your game. Please try again."}, err
			}
			return commands.Reply{Text: "Game has been recorded"}, nil
		} else {
			err = o.store.storeGame(ctx, game, false)
			if err != nil {
				return commands.Reply{Text: "An error occured while storing your game. Please try again."}, err
			}
			return commands.Reply{Text: "Game has been recorded"}, nil
		}
	}

	switch e.Command {
	case "oneuppuzzle", "oneup":

		if len(e.Args) == 0 {
			return commands.Reply{Text: "https://www.oneuppuzzle.com"}, nil
		}

		switch e.Args[0] {
		case "help":
			return commands.Reply{Text: "Valid arguments for the !oneup command are: help, leaders, stats, or <username>"}, nil
		case "leaders":

			//if "!oneup leaders" - return top 3
			// if "!oneup leaders <int>" return top int players

			return commands.Reply{Text: "Leaderboards aren't available yet."}, nil

		case "stats":

			switch {
			case len(e.Args) == 1:
				cp, ok, err := o.store.puzzleStatsByChannel(ctx, e.Connection.ID)
				if err != nil {
					return commands.Reply{Text: "An error occured while fetching channel stats. Please try again."}, err
				}

				if !ok {
					return commands.Reply{Text: "This channel does not have any tracked games."}, nil
				}

				return commands.Reply{Text: fmt.Sprintf("Channel Stats: Total Games %d | Average %s | Fastest Game %s (%s) | Slowest Game %s (%s)", *cp.GamesCompleted, formatTime(*cp.Completions.AverageTime), formatTime(*cp.Completions.FastestTime), *cp.FastestUser, formatTime(*cp.Completions.SlowestTime), *cp.SlowestUser)}, nil

			case len(e.Args) == 2:

				id := e.Args[1]

				//Decide if we are working with a puzzleID or a username

				puzzleID, err := strconv.Atoi(id)
				if err != nil {
					if id[0] == '#' {

						puzzleID, err = strconv.Atoi(id[1:])
						if err != nil {
							return commands.Reply{Text: "Invalid puzzle id. Must be only the number '!oneuppuzzle stats 616' or the number with a leading # '!oneuppuzzle stats #616'."}, nil
						}

						return o.statsForPuzzle(ctx, puzzleID)
					}
				} else {

					return o.statsForPuzzle(ctx, puzzleID)
				}

				return o.statsForUser(ctx, e.Platform, id)

			case len(e.Args) == 3:

				potentialID := e.Args[2]

				puzzleID, err := strconv.Atoi(strings.TrimPrefix(potentialID, "#"))
				if err != nil {
					return commands.Reply{Text: "Invalid puzzle id. Must be only the number '!oneuppuzzle stats 616' or the number with a leading # '!oneuppuzzle stats #616'."}, nil
				}

				return o.statsForPuzzle(ctx, puzzleID)
			}

			return commands.Reply{Text: "Usage: !oneup stats [username | puzzle id]"}, nil

		default:
			//try to look up player stats, otherwise
			return commands.Reply{Text: "Invalid Command"}, nil
		}
	}

	// help - return usage
	// leaders - return top 3 fastest averages
	// username - return total completed, average time, longest time, shortest time
	// stats - total games tracked, channel average, slowest, fastest
	return commands.Reply{Text: "Im not even sure how you managed to input a command to get here... but you did. "}, nil
}

// errAlreadyRecorded means the user already posted this puzzle.
var errAlreadyRecorded = errors.New("oneup: game already recorded")

func (o *OneUpPuzzle) validateGameEntry(ctx context.Context, e commands.Request) (OneUpGame, error) {

	// Attempt to parse the game entry. If Error in parsing, its invalid
	newGame, err := packageGame(e)
	if err != nil {
		return newGame, err
	}

	//Need some kind of check here to see if the game is the correct numbered game.

	if newGame.PlatformUserID == 0 {
		return newGame, fmt.Errorf("oneup: the player's identity couldn't be looked up, so the game can't be recorded")
	}

	// Validate the game ID as well as if the user has sumbitted this before
	recorded, err := o.store.isPuzzleRecorded(ctx, newGame.PlatformUserID, newGame.Details.PuzzleID)
	if err != nil {
		return newGame, fmt.Errorf("ispuzzlerecorded: %w", err)
	}

	if recorded {
		return newGame, errAlreadyRecorded
	}

	return newGame, nil
}

func (o *OneUpPuzzle) statsForPuzzle(ctx context.Context, puzzleID int) (commands.Reply, error) {

	gameProfile, ok, err := o.store.puzzleStatsByPuzzleID(ctx, puzzleID)
	if err != nil {
		return commands.Reply{Text: fmt.Sprintf("An error occured while fetching stats for puzzle #%d. Please try again in a moment.", puzzleID)}, err
	}

	if !ok {
		return commands.Reply{Text: fmt.Sprintf("There are no puzzles tracked for Puzzle #%d.", puzzleID)}, nil
	}

	return commands.Reply{Text: fmt.Sprintf("Stats for Puzzle #%d: Total Games %d | Average %s | Fastest Game %s (%s) | Slowest Game %s (%s)", gameProfile.GameID, *gameProfile.GamesCompleted, formatTime(*gameProfile.Completions.AverageTime), formatTime(*gameProfile.Completions.FastestTime), *gameProfile.FastestUser, formatTime(*gameProfile.Completions.SlowestTime), *gameProfile.SlowestUser)}, nil

}

func (o *OneUpPuzzle) statsForUser(ctx context.Context, platform, username string) (commands.Reply, error) {

	userProfile, ok, err := o.store.puzzleStatsByUser(ctx, platform, username)
	if err != nil {
		return commands.Reply{Text: "An error occured while fetching user stats. Please try again."}, err
	}

	if !ok {
		return commands.Reply{Text: "This user does not have any tracked games."}, nil
	}

	return commands.Reply{Text: fmt.Sprintf("Stats for @%s: Total Games %d | Average  %s | Fastest Game %s | Slowest Game %s", username, *userProfile.GamesCompleted, formatTime(*userProfile.Completions.AverageTime), formatTime(*userProfile.Completions.FastestTime), formatTime(*userProfile.Completions.SlowestTime))}, nil

}

func formatTime(seconds int) string {

	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60

	if h == 0 {
		if m == 0 {
			return fmt.Sprintf("%d seconds", s)
		}
		return fmt.Sprintf("%d minutes, %d seconds", m, s)
	}

	return fmt.Sprintf("%d hours, %d minutes, %d seconds", h, m, s)

}
