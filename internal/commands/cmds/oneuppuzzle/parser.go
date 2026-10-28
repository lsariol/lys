package oneuppuzzle

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/lsariol/lys/internal/commands"
)

// rawMessage is how a game's message is stored in game_events.raw_message:
// "channel:chatter-text", as the bot has always written it.
func rawMessage(e commands.Request) string {
	return fmt.Sprintf("%s:%s-%s", e.Channel.Name, e.User.Name, e.Text)
}

func packageGame(e commands.Request) (OneUpGame, error) {
	var game OneUpGame

	gameDetails, err := parseGameString(rawMessage(e))
	if err != nil {
		return game, err
	}

	game.GameCode = "oneuppuzzle"
	game.RawMessage = rawMessage(e)
	game.ConnectionID = e.Connection.ID
	game.PlatformUserID = e.Identity.PlatformUserID
	game.Details = gameDetails

	return game, nil
}

var (
	hourRe   = regexp.MustCompile(`(\d+)\s*hour`)
	minuteRe = regexp.MustCompile(`(\d+)\s*minute`)
	secondRe = regexp.MustCompile(`(\d+)\s*second`)
	gameRe   = regexp.MustCompile(`#(\d+)`)
)

func parseGameString(s string) (OneUpGameDetails, error) {
	game := 0
	hours := 0
	minutes := 0
	seconds := 0

	if m := hourRe.FindStringSubmatch(s); len(m) == 2 {
		hours, _ = strconv.Atoi(m[1])
	}

	if m := minuteRe.FindStringSubmatch(s); len(m) == 2 {
		minutes, _ = strconv.Atoi(m[1])
	}

	if m := secondRe.FindStringSubmatch(s); len(m) == 2 {
		seconds, _ = strconv.Atoi(m[1])
	}

	if m := gameRe.FindStringSubmatch(s); len(m) == 2 {
		game, _ = strconv.Atoi(m[1])
	}

	totalSeconds := hours*3600 + minutes*60 + seconds

	g := OneUpGameDetails{
		PuzzleID:    game,
		TimeSeconds: totalSeconds,
	}

	if game == 0 || totalSeconds == 0 {
		return g, errors.New("invalid input")
	}

	return g, nil
}
