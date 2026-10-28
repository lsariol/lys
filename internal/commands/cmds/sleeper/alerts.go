package sleeper

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

// alertKinds are the league's announcements a channel can turn off; all are on by default.
var alertKinds = []string{"trades", "waivers", "lineups"}

// alerts shows or changes which of the league's announcements are made in chat:
// "!ff alerts", or "!ff alerts <trades|waivers|lineups> <on|off>" for the channel's owner.
func alerts(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	usage := fmt.Sprintf("Usage: %sff alerts, or %sff alerts <%s> <on|off>.", e.Prefix, e.Prefix, strings.Join(alertKinds, "|"))

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return commands.Reply{Text: "Internal error getting league information."}, err
	}
	if league == nil {
		return commands.Reply{Text: "This channel is not associated with any league."}, nil
	}

	switch len(e.Args) {
	case 1:
		var trades, waivers, lineups bool
		if err := sl.db.QueryRow(ctx, `
			SELECT COALESCE((notifications->>'trades')::boolean, true), COALESCE((notifications->>'waivers')::boolean, true),
			       COALESCE((notifications->>'lineups')::boolean, true)
			FROM lys.sleeper_leagues WHERE id = $1`, league.ID).Scan(&trades, &waivers, &lineups); err != nil {
			return commands.Reply{Text: "Internal error getting league information."}, err
		}
		return commands.Reply{Text: fmt.Sprintf("🏈 Announcements in chat: trades %s, waivers %s, lineup reminders %s.", onOff(trades), onOff(waivers), onOff(lineups))}, nil

	case 3:
		kind, value := strings.ToLower(e.Args[1]), strings.ToLower(e.Args[2])
		if !slices.Contains(alertKinds, kind) || (value != "on" && value != "off") {
			return commands.Reply{Text: usage}, nil
		}
		if !commands.HasPrivilege(e) {
			return commands.Reply{Text: "You do not have privileges to use this command."}, nil
		}
		if _, err := sl.db.Exec(ctx, `
			UPDATE lys.sleeper_leagues SET notifications = notifications || jsonb_build_object($2::text, $3::boolean), updated_at = now()
			WHERE id = $1`, league.ID, kind, value == "on"); err != nil {
			return commands.Reply{Text: "There was an error saving that. Please try again."}, err
		}
		if kind == "lineups" {
			return commands.Reply{Text: fmt.Sprintf("🏈 Lineup reminders are %s.", value)}, nil
		}
		return commands.Reply{Text: fmt.Sprintf("🏈 %s announcements are %s.", strings.ToUpper(kind[:1])+kind[1:], value)}, nil

	default:
		return commands.Reply{Text: usage}, nil
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
