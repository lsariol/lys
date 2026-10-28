package sleeper

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

func link(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {

	if len(e.Args) != 3 {
		return commands.Reply{Text: "Usage: !sleeper link <user|league> <username|league ID or URL>"}, nil
	}

	switch e.Args[1] {
	case "user":

		league, err := getLeague(ctx, e.Connection.CommunityID, sl)
		if err != nil {
			log.Print(err)
			return commands.Reply{Text: "Internal error Link User."}, err
		}

		if league == nil {
			return commands.Reply{Text: "This channel is not associated with any league."}, err
		}

		if e.Identity.PlatformUserID == 0 {
			return commands.Reply{Text: "Internal error Link User."}, fmt.Errorf("sleeper link user: %q has no identity", e.User.Name)
		}

		sleeperUsers, err := sl.client.LeagueUsers(ctx, league.SleeperLeagueID)
		if err != nil {
			return commands.Reply{Text: "Unable to link user due to an invalid response from the Sleeper API."}, fmt.Errorf("Sleeper: get league users: %w", err)
		}

		sleeperRosters, err := sl.client.Rosters(ctx, league.SleeperLeagueID)
		if err != nil {
			return commands.Reply{Text: "Unable to link user due to an invalid response from the Sleeper API."}, fmt.Errorf("Sleeper: get league rosters: %w", err)
		}

		for _, user := range *sleeperUsers {
			if strings.EqualFold(user.DisplayName, e.Args[2]) {

				for _, roster := range *sleeperRosters {
					if strings.EqualFold(roster.OwnerID, user.UserID) {
						err := linkUser(ctx, league.ID, e.Identity.PlatformUserID, user.UserID, user.DisplayName, roster.RosterID, sl)
						if err != nil {
							log.Printf("error linking user: %v", err)
							return commands.Reply{Text: "Internal error linking user."}, err
						}

						if user.Metadata.TeamName != "" {
							return commands.Reply{Text: fmt.Sprintf("@%s has been linked to the username '%s', and team '%s' in the '%s' League.",
								e.User.Name, user.DisplayName, user.Metadata.TeamName, league.LeagueName)}, nil
						}
						return commands.Reply{Text: fmt.Sprintf("@%s has been linked to the username '%s' in the '%s' League.",
							e.User.Name, user.DisplayName, league.LeagueName)}, nil
					}
				}

			}
		}

		return commands.Reply{Text: fmt.Sprintf("No one in the '%s' League with a team goes by '%s' on Sleeper.", league.LeagueName, e.Args[2])}, nil

	case "league":

		if !commands.HasPrivilege(e) {
			return commands.Reply{Text: "You do not have privileges to use this command."}, nil
		}

		if e.Connection.CommunityID == 0 {
			return commands.Reply{Text: "This channel isn't set up in Lys yet, so it can't be linked to a league."}, fmt.Errorf("sleeper link league: %q has no community", e.Channel.Name)
		}

		//parse leagueID from user input
		leagueID, err := extractLeagueID(e.Args[2])
		if err != nil {
			return commands.Reply{Text: "Invalid league ID or URL."}, err
		}

		//Does this channel or league already have a link
		existing, err := FindLeagueLinkConflict(leagueID, e.Connection.CommunityID, ctx, sl)
		if err != nil {
			return commands.Reply{Text: "An error occured when validating channel to league link."}, err
		}

		if existing != nil {
			switch {
			case existing.CommunityID == e.Connection.CommunityID &&
				existing.SleeperLeagueID == leagueID:
				return commands.Reply{Text: "This channel is already linked to this league."}, err

			case existing.CommunityID == e.Connection.CommunityID:
				return commands.Reply{Text: "This channel is already linked to another league."}, err

			case existing.SleeperLeagueID == leagueID:
				return commands.Reply{Text: "This league is already linked to another channel."}, err

			}
		}

		//Get League from API
		league, err := sl.client.League(ctx, leagueID)
		if err != nil {
			log.Println(err)
			return commands.Reply{Text: "Unable to link league due to a invalid response from sleeper API."}, err
		}

		// Link in database
		err, ok := addLeagueLink(league, e.Connection.CommunityID, ctx, sl)
		if !ok {
			return commands.Reply{Text: "Internal error occured while inserting into database."}, err
		}

		return commands.Reply{Text: fmt.Sprintf("This channel (%s) has successfully been linked to the Sleeper league %s. People in the league can now connect their accounts using !sleeper link user <sleeper username>.", e.Channel.Name, league.Name)}, err
	}

	return commands.Reply{Text: "Usage: !sleeper link <user|league> <username|league ID or URL>"}, nil
}

var leagueIDRegex = regexp.MustCompile(`(?:^|/leagues/)(\d+)`)

func extractLeagueID(input string) (string, error) {

	match := leagueIDRegex.FindStringSubmatch(input)
	if len(match) < 2 {
		return "", fmt.Errorf("invalid Sleeper league or URL")
	}
	return match[1], nil
}
