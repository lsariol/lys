package sleeper

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type Sleeper struct {
	commandPrefix string
}

func (Sleeper) Name() string           { return "sleeper" }
func (Sleeper) Aliases() []string      { return []string{"ff"} }
func (Sleeper) Regexes() []string      { return nil }
func (Sleeper) Description() string    { return "Fantasy Football ran through Sleeper." }
func (Sleeper) Usage() string          { return "!sleeper" }
func (Sleeper) Timeout() time.Duration { return 10 * time.Second }

func (Sleeper) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	err := updateTwitchUsername(ctx, e.UserID, e.Username, deps)
	if err != nil {
		log.Printf("error updating Twitch username: %v", err)
	}

	if len(e.Args) == 0 {
		return adapter.Response{Text: "https://sleeper.com/leagues/1388284269125726208"}, nil
	}

	switch e.Args[0] {
	case "draft":

		timeUntilDraft := TimeUntilDraft()
		return adapter.Response{Text: timeUntilDraft}, nil

	case "link":
		return link(ctx, e, deps)

	case "unlink":
		return unlink(ctx, e, deps)

	case "standings":
		return standings(ctx, e, deps)

	case "team":
		return team(ctx, e, deps)

	case "matchup":
		return matchup(ctx, e, deps)
	case "scores":
		return scores(ctx, e, deps)
	case "roster":
		return roster(ctx, e, deps)
	case "highscore":
		return highscore(ctx, e, deps)
	case "trades":
		return adapter.Response{Text: fmt.Sprintf("🏈 Recent trades: %s ↔ noto — Lamb + 2nd for Bijan | Alex ↔ Chris — Andrews for Flowers", e.Args[1])}, nil
	case "whohas":
		return whohas(ctx, e, deps)
	case "playoffs":
		return playoffs(ctx, e, deps)

	default:
		return adapter.Response{Text: "🏈 Invalid command"}, nil
	}

}
