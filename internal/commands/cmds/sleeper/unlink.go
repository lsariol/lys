package sleeper

import (
	"context"
	"fmt"
	"log"

	"github.com/lsariol/lys/internal/commands"
)

func unlink(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	if len(e.Args) != 2 {
		return commands.Reply{Text: "Usage: !sleeper unlink <user|league>"}, nil
	}

	switch e.Args[1] {
	case "user":

		league, err := getLeague(ctx, e.Connection.CommunityID, sl)
		if err != nil {
			log.Print(err)
			return commands.Reply{Text: "Internal error unlinking user."}, err
		}

		if league == nil {
			return commands.Reply{Text: "This channel is not associated with any league."}, nil
		}

		found, err := removeUser(ctx, league.ID, e.Identity.PlatformUserID, sl)
		if err != nil {
			log.Print(err)
			return commands.Reply{Text: "Internal error unlinking user."}, err
		}

		if !found {
			return commands.Reply{Text: "You are not linked to a Sleeper account in this league."}, nil
		}

		return commands.Reply{Text: fmt.Sprintf("@%s has been unlinked from the '%s' League.",
			e.User.Name, league.LeagueName)}, nil

	case "league":

		if !commands.HasPrivilege(e) {
			return commands.Reply{Text: "You do not have privileges to use this command."}, nil
		}

		league, err := getLeague(ctx, e.Connection.CommunityID, sl)
		if err != nil {
			log.Print(err)
			return commands.Reply{Text: "Internal error unlinking league."}, err
		}

		if league == nil {
			return commands.Reply{Text: "This channel is not associated with any league."}, nil
		}

		err = removeLeagueLink(ctx, league.ID, sl)
		if err != nil {
			log.Print(err)
			return commands.Reply{Text: "Internal error unlinking league."}, err
		}

		return commands.Reply{Text: fmt.Sprintf("This channel (%s) has been unlinked from the Sleeper league '%s'. All associated user links have been removed.",
			e.Channel.Name, league.LeagueName)}, nil
	}

	return commands.Reply{Text: "Usage: !sleeper unlink <user|league>"}, nil
}
