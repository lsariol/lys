package sleeper

import (
	"context"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

// unlink is "!ff unlink" (or "!ff unlink user") for your own team,
// "!ff unlink @user" for moderators and the owner, and "!ff unlink league"
// for the owner.
func unlink(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	args := e.Args[1:]
	if len(args) > 1 {
		return usage(e, "unlink [@user|league]"), nil
	}
	if len(args) == 1 && strings.EqualFold(args[0], "league") {
		return unlinkLeague(ctx, e, sl)
	}

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}

	if len(args) == 0 || strings.EqualFold(args[0], "user") {
		found, err := removeUser(ctx, league.ID, e.Identity.PlatformUserID, sl)
		if err != nil {
			return broken(), err
		}
		if !found {
			return say("@%s, you haven't linked a team in this league.", e.User.Name), nil
		}
		return say("@%s is no longer linked to a team in %s.", e.User.Name, league.LeagueName), nil
	}

	name, self := targetUser(e, args)
	if !strings.HasPrefix(args[0], "@") {
		return usage(e, "unlink [@user|league]"), nil
	}
	if !self && !e.Allowed(commands.Moderator) {
		return commands.Refuse(commands.Moderator, "unlink someone else"), nil
	}
	c, err := getConnectionByUsername(ctx, league.ID, name, sl)
	if err != nil {
		return broken(), err
	}
	if c == nil {
		return say("@%s hasn't linked a team in this league.", name), nil
	}
	if _, err := removeUser(ctx, league.ID, c.PlatformUserID, sl); err != nil {
		return broken(), err
	}
	return say("@%s is no longer linked to a team in %s.", c.DisplayName, league.LeagueName), nil
}

func unlinkLeague(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	if !e.Allowed(commands.Owner) {
		return commands.Refuse(commands.Owner, "unlink the league"), nil
	}
	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	if err := removeLeagueLink(ctx, league.ID, sl); err != nil {
		return broken(), err
	}
	return say("Unlinked this channel from %s, and everyone's team links with it.", league.LeagueName), nil
}
