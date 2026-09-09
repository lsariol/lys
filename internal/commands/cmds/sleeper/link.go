package sleeper

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/lsariol/botsuite/internal/adapters/adapter"
	"github.com/lsariol/botsuite/internal/app/dependencies"
	"github.com/lsariol/botsuite/internal/commands"
)

func link(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	if len(e.Args) != 3 {
		return adapter.Response{Text: "Usage: !sleeper link <user|league> <username|league ID or URL>"}, nil
	}

	switch e.Args[1] {
	case "user":

		league, err := getLeagueByChannel(ctx, e.ChannelID, deps)
		if err != nil {
			log.Print(err)
			return adapter.Response{Text: "Internal error Link User."}, err
		}

		if league == nil {
			return adapter.Response{Text: "This channel is not associated with any league."}, err
		}

		sleeperUsers, err := getPlayersInLeague(league.SleeperLeagueID, ctx, deps)

		sleeperRosters, err := getRosters(league.SleeperLeagueID, ctx, deps)

		for _, user := range *sleeperUsers {
			if strings.EqualFold(user.DisplayName, e.Args[2]) {

				for _, roster := range *sleeperRosters {
					if strings.EqualFold(roster.OwnerID, user.UserID) {
						err := linkUser(ctx, league.ID, e.UserID, e.Username, user.UserID, user.DisplayName, roster.RosterID, deps)
						if err != nil {
							log.Printf("error linking user: %v", err)
							return adapter.Response{Text: "Internal error linking user."}, err
						}

						if user.Metadata.TeamName != "" {
							return adapter.Response{Text: fmt.Sprintf("@%s has been linked to the username '%s', and team '%s' in the '%s' League.",
								e.Username, user.DisplayName, user.Metadata.TeamName, league.LeagueName)}, nil
						}
						return adapter.Response{Text: fmt.Sprintf("@%s has been linked to the username '%s' in the '%s' League.",
							e.Username, user.DisplayName, league.LeagueName)}, nil
					}
				}

			}
		}

		return adapter.Response{Text: "Legit dont know how you got here in the code. Probably check your command syntax or something idk."}, err
		//err := linkUser(ctx, string(league.ID), e.UserID)

	case "league":

		if !commands.HasPrivilege(e.UserID, e.ChannelID) {
			return adapter.Response{Text: "You do not have privileges to use this command."}, nil
		}

		//parse leagueID from user input
		leagueID, err := extractLeagueID(e.Args[2])
		if err != nil {
			return adapter.Response{Text: "Invalid league ID or URL."}, err
		}

		//Does this channel or league already have a link
		existing, err := FindLeagueLinkConflict(leagueID, e.ChannelID, ctx, deps)
		if err != nil {
			return adapter.Response{Text: "An error occured when validating channel to league link."}, err
		}

		if existing != nil {
			switch {
			case existing.TwitchChannelID == e.ChannelID &&
				existing.SleeperLeagueID == leagueID:
				return adapter.Response{Text: "This channel is already linked to this league."}, err

			case existing.TwitchChannelID == e.ChannelID:
				return adapter.Response{Text: "This channel is already linked to another league."}, err

			case existing.SleeperLeagueID == leagueID:
				return adapter.Response{Text: "This league is already linked to another channel."}, err

			}
		}

		//Get League from API
		league, err := GetLeague(ctx, leagueID, deps.HTTP)
		if err != nil {
			log.Println(err)
			return adapter.Response{Text: "Unable to link league due to a invalid response from sleeper API."}, err
		}

		// Link in database
		err, ok := addLeagueLink(league, e.ChannelID, ctx, deps)
		if !ok {
			return adapter.Response{Text: "Internal error occured while inserting into database."}, err
		}

		return adapter.Response{Text: fmt.Sprintf("This channel (%s) has successfully been linked to the Sleeper league %s. People in the league can now connect their accounts using !sleeper link user <sleeper username>.", e.ChannelName, league.Name)}, err
	}

	return adapter.Response{Text: "Usage: !sleeper link <user|league> <username|league ID or URL>"}, nil
}

var leagueIDRegex = regexp.MustCompile(`(?:^|/leagues/)(\d+)`)

func extractLeagueID(input string) (string, error) {

	match := leagueIDRegex.FindStringSubmatch(input)
	if len(match) < 2 {
		return "", fmt.Errorf("invalid Sleeper league or URL")
	}
	return match[1], nil
}
