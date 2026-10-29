package sleeper

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// Sleeper answers fantasy football questions from a channel's Sleeper league.
type Sleeper struct {
	client *api.Client
	db     *pgxpool.Pool
}

func New(client *api.Client, db *pgxpool.Pool) *Sleeper { return &Sleeper{client: client, db: db} }

// subcommands is what !ff does, for !ff help, !help ff and unknown words.
const subcommands = "team, matchup, scores, standings, roster, whohas, trades, recap, highscore, playoffs, draft, link, unlink, alerts"

func (*Sleeper) Spec() commands.Spec {
	return commands.Spec{
		Name:        "sleeper",
		Aliases:     []string{"ff"},
		Description: "Fantasy football from the channel's Sleeper league.",
		Usage:       "!ff <" + strings.ReplaceAll(subcommands, ", ", "|") + ">",
		Timeout:     20 * time.Second,
		Concurrency: 16,
	}
}

func (sl *Sleeper) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) == 0 {
		return root(ctx, e, sl)
	}
	switch strings.ToLower(e.Args[0]) {
	case "help":
		return say("%sff %s. Most take an @user, like %sff team @name.", e.Prefix, subcommands, e.Prefix), nil
	case "draft":
		return draft(ctx, e, sl)
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
		return trades(ctx, e, sl)
	case "recap":
		return recap(ctx, e, sl)
	case "whohas":
		return whohas(ctx, e, sl)
	case "alerts":
		return alerts(ctx, e, sl)
	case "playoffs":
		return playoffs(ctx, e, sl)
	}
	return say("There's no %sff %s. Try: %s.", e.Prefix, e.Args[0], subcommands), nil
}

// root answers a bare !ff with the channel's league.
func root(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	return say("%s: https://sleeper.com/leagues/%s · Try %sff help for the commands.", league.LeagueName, league.SleeperLeagueID, e.Prefix), nil
}
