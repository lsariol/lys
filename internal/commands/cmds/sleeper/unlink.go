package sleeper

import (
	"context"
	"fmt"
	"log"

	"github.com/lsariol/botsuite/internal/adapters/adapter"
	"github.com/lsariol/botsuite/internal/app/dependencies"
	"github.com/lsariol/botsuite/internal/commands"
)

func unlink(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	if len(e.Args) != 2 {
		return adapter.Response{Text: "Usage: !sleeper unlink <user|league>"}, nil
	}

	switch e.Args[1] {
	case "user":

		league, err := getLeagueByChannel(ctx, e.ChannelID, deps)
		if err != nil {
			log.Print(err)
			return adapter.Response{Text: "Internal error unlinking user."}, err
		}

		if league == nil {
			return adapter.Response{Text: "This channel is not associated with any league."}, nil
		}

		found, err := removeUser(ctx, league.ID, e.UserID, deps)
		if err != nil {
			log.Print(err)
			return adapter.Response{Text: "Internal error unlinking user."}, err
		}

		if !found {
			return adapter.Response{Text: "You are not linked to a Sleeper account in this league."}, nil
		}

		return adapter.Response{Text: fmt.Sprintf("@%s has been unlinked from the '%s' League.",
			e.Username, league.LeagueName)}, nil

	case "league":

		if !commands.HasPrivilege(e.UserID, e.ChannelID) {
			return adapter.Response{Text: "You do not have privileges to use this command."}, nil
		}

		league, err := getLeagueByChannel(ctx, e.ChannelID, deps)
		if err != nil {
			log.Print(err)
			return adapter.Response{Text: "Internal error unlinking league."}, err
		}

		if league == nil {
			return adapter.Response{Text: "This channel is not associated with any league."}, nil
		}

		err = removeLeagueLink(ctx, league.ID, deps)
		if err != nil {
			log.Print(err)
			return adapter.Response{Text: "Internal error unlinking league."}, err
		}

		return adapter.Response{Text: fmt.Sprintf("This channel (%s) has been unlinked from the Sleeper league '%s'. All associated user links have been removed.",
			e.ChannelName, league.LeagueName)}, nil
	}

	return adapter.Response{Text: "Usage: !sleeper unlink <user|league>"}, nil
}
