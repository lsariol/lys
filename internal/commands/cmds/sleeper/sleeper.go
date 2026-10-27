package sleeper

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/commands"
)

// Sleeper answers fantasy football questions from a channel's Sleeper league.
type Sleeper struct {
	http *http.Client
	db   *pgxpool.Pool
}

func New(http *http.Client, db *pgxpool.Pool) *Sleeper { return &Sleeper{http: http, db: db} }

func (*Sleeper) Spec() commands.Spec {
	return commands.Spec{
		Name:        "sleeper",
		Aliases:     []string{"ff"},
		Description: "Fantasy Football ran through Sleeper.",
		Usage:       "!sleeper",
		Timeout:     10 * time.Second,
	}
}

func (sl *Sleeper) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	err := updateTwitchUsername(ctx, e.User.ID, e.User.Name, sl)
	if err != nil {
		log.Printf("error updating Twitch username: %v", err)
	}

	if len(e.Args) == 0 {
		return commands.Reply{Text: "https://sleeper.com/leagues/1388284269125726208"}, nil
	}

	switch e.Args[0] {
	case "draft":

		timeUntilDraft := TimeUntilDraft()
		return commands.Reply{Text: timeUntilDraft}, nil

	case "link":
		return link(ctx, e, sl)

	case "unlink":
		return unlink(ctx, e, sl)

	case "standings":
		return standings(ctx, e, sl)

	case "team":
		return team(ctx, e, sl)

	case "matchup":
		return matchup(ctx, e, sl)
	case "scores":
		return scores(ctx, e, sl)
	case "roster":
		return roster(ctx, e, sl)
	case "highscore":
		return highscore(ctx, e, sl)
	case "trades":
		return trades(e.Args), nil
	case "whohas":
		return whohas(ctx, e, sl)
	case "playoffs":
		return playoffs(ctx, e, sl)

	default:
		return commands.Reply{Text: "🏈 Invalid command"}, nil
	}

}

// trades answers "!ff trades <name>"; args includes "trades".
func trades(args []string) commands.Reply {
	if len(args) < 2 {
		return commands.Reply{Text: "🏈 Usage: !ff trades <name>"}
	}
	return commands.Reply{Text: fmt.Sprintf("🏈 Recent trades: %s ↔ noto — Lamb + 2nd for Bijan | Alex ↔ Chris — Andrews for Flowers", args[1])}
}
