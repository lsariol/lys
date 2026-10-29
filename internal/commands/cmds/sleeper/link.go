package sleeper

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

// link is "!ff link <Sleeper name>" (or the older "!ff link user <name>"),
// and "!ff link league <league URL or ID>" for the channel's owner.
func link(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	args := e.Args[1:]
	if len(args) > 0 && strings.EqualFold(args[0], "league") {
		if len(args) != 2 {
			return usage(e, "link league <league URL or ID>"), nil
		}
		return linkLeague(ctx, e, sl, args[1])
	}
	if len(args) > 0 && strings.EqualFold(args[0], "user") {
		args = args[1:]
	}
	if len(args) != 1 {
		return usage(e, "link <your Sleeper name>"), nil
	}
	return linkUser(ctx, e, sl, args[0])
}

func linkUser(ctx context.Context, e commands.Request, sl *Sleeper, sleeperName string) (commands.Reply, error) {
	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	if e.Identity.PlatformUserID == 0 {
		return broken(), fmt.Errorf("sleeper link: %q has no identity", e.User.Name)
	}
	if mine, err := getConnectionByUserID(ctx, league.ID, e.Identity.PlatformUserID, sl); err != nil {
		return broken(), err
	} else if mine != nil {
		return say("@%s, you're already linked to %s. Use %sff unlink first to link another team.", e.User.Name, mine.SleeperUsername, e.Prefix), nil
	}

	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}
	for _, user := range users {
		if !strings.EqualFold(user.DisplayName, sleeperName) {
			continue
		}
		for _, roster := range rosters {
			if roster.OwnerID != user.UserID {
				continue
			}
			if taken, err := getConnectionByRoster(ctx, league.ID, roster.RosterID, sl); err != nil {
				return broken(), err
			} else if taken != nil {
				return say("That team is already linked to @%s. A moderator can remove the link with %sff unlink @%s.", taken.DisplayName, e.Prefix, taken.DisplayName), nil
			}
			if err := addMember(ctx, league.ID, e.Identity.PlatformUserID, user.UserID, user.DisplayName, roster.RosterID, sl); err != nil {
				return broken(), err
			}
			team := user.DisplayName
			if user.Metadata.TeamName != "" {
				team = user.Metadata.TeamName
			}
			return say("@%s is now linked to %s (%s on Sleeper).", e.User.Name, team, user.DisplayName), nil
		}
	}
	return say("No one with a team in %s goes by %q on Sleeper.", league.LeagueName, sleeperName), nil
}

func linkLeague(ctx context.Context, e commands.Request, sl *Sleeper, input string) (commands.Reply, error) {
	if !e.Allowed(commands.Owner) {
		return commands.Refuse(commands.Owner, "link a league"), nil
	}
	if e.Connection.CommunityID == 0 {
		return broken(), fmt.Errorf("sleeper link league: %q has no community", e.Channel.Name)
	}
	leagueID, err := extractLeagueID(input)
	if err != nil {
		return say("That isn't a Sleeper league URL or ID, like https://sleeper.com/leagues/1234567890."), nil
	}

	existing, err := FindLeagueLinkConflict(leagueID, e.Connection.CommunityID, ctx, sl)
	if err != nil {
		return broken(), err
	}
	if existing != nil {
		switch {
		case existing.CommunityID == e.Connection.CommunityID && existing.SleeperLeagueID == leagueID:
			return say("This channel is already linked to that league."), nil
		case existing.CommunityID == e.Connection.CommunityID:
			return say("This channel is already linked to another league. Unlink it first with %sff unlink league.", e.Prefix), nil
		default:
			return say("That league is already linked to another channel."), nil
		}
	}

	league, err := sl.client.League(ctx, leagueID)
	if err != nil {
		return unreachable(), err
	}
	if err, ok := addLeagueLink(league, e.Connection.CommunityID, ctx, sl); !ok {
		return broken(), err
	}
	return say("Linked this channel to the Sleeper league %s. Everyone in it can now link their team with %sff link <Sleeper name>.", league.Name, e.Prefix), nil
}

var leagueIDRegex = regexp.MustCompile(`(?:^|/leagues/)(\d+)`)

func extractLeagueID(input string) (string, error) {
	match := leagueIDRegex.FindStringSubmatch(input)
	if len(match) < 2 {
		return "", fmt.Errorf("invalid Sleeper league or URL")
	}
	return match[1], nil
}
