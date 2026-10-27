package oneuppuzzle

import (
	"testing"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
)

func TestPackageGame(t *testing.T) {
	const text = "Congratulations, you finished puzzle #616 in 3 minutes, 5 seconds"
	game, err := packageGame(commands.Request{Message: core.Message{
		Channel: core.Channel{ID: "1", Name: "Streamer"},
		User:    core.User{ID: "2", Name: "Viewer"},
		Text:    text,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if game.RawMessage != "Streamer:Viewer-"+text {
		t.Fatalf("raw message changed format: %q", game.RawMessage)
	}
	if game.Details.PuzzleID != 616 || game.Details.TimeSeconds != 185 {
		t.Fatalf("details: %+v", game.Details)
	}
	if game.UserID != "2" || game.ChannelName != "Streamer" || game.GameCode != "oneuppuzzle" {
		t.Fatalf("game: %+v", game)
	}
}
