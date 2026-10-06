package oneuppuzzle

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/lsariol/lys/internal/adapters/adapter"
)

func packageGame(e adapter.Envelope) (OneUpGame, error) {
	var game OneUpGame

	gameDetails, err := parseGameString(e.RawMessage)
	if err != nil {
		return game, err
	}

	game.UserID = e.UserID
	game.Username = e.Username
	game.ChannelID = e.ChannelID
	game.ChannelName = e.ChannelName
	game.GameCode = "oneuppuzzle"
	game.RawMessage = e.RawMessage
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
