package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/lsariol/lys/internal/commands"
)

// alertKinds are the league's announcements a channel can turn off, with
// how they're shown; all are on by default.
var alertKinds = []struct{ key, label string }{
	{"trades", "trades"},
	{"waivers", "waivers"},
	{"freeagents", "free agents"},
	{"lineups", "lineup reminders"},
	{"recap", "weekly recaps"},
}

// alerts shows or changes which of the league's announcements are made in chat:
// "!ff alerts", or "!ff alerts <kind> <on|off>" for the channel's owner.
func alerts(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	var keys []string
	for _, k := range alertKinds {
		keys = append(keys, k.key)
	}
	form := fmt.Sprintf("alerts [%s] [on|off]", strings.Join(keys, "|"))

	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}

	switch len(e.Args) {
	case 1:
		var raw []byte
		if err := sl.db.QueryRow(ctx, `SELECT COALESCE(notifications, '{}'::jsonb) FROM lys.sleeper_leagues WHERE id = $1`, league.ID).Scan(&raw); err != nil {
			return broken(), err
		}
		set := map[string]bool{}
		if err := json.Unmarshal(raw, &set); err != nil {
			return broken(), fmt.Errorf("sleeper alerts: read the league's switches: %w", err)
		}
		var states []string
		for _, k := range alertKinds {
			on, ok := set[k.key]
			states = append(states, fmt.Sprintf("%s %s", k.label, onOff(on || !ok)))
		}
		return say("Announcements in chat: %s.", strings.Join(states, ", ")), nil

	case 3:
		key, value := strings.ToLower(e.Args[1]), strings.ToLower(e.Args[2])
		i := slices.IndexFunc(alertKinds, func(k struct{ key, label string }) bool { return k.key == key })
		if i < 0 || (value != "on" && value != "off") {
			return usage(e, form), nil
		}
		if !e.Allowed(commands.Owner) {
			return commands.Refuse(commands.Owner, "change the alerts"), nil
		}
		if _, err := sl.db.Exec(ctx, `
			UPDATE lys.sleeper_leagues SET notifications = notifications || jsonb_build_object($2::text, $3::boolean), updated_at = now()
			WHERE id = $1`, league.ID, key, value == "on"); err != nil {
			return say("Something went wrong saving that."), err
		}
		label := alertKinds[i].label
		return say("%s are %s.", strings.ToUpper(label[:1])+label[1:], value), nil
	}
	return usage(e, form), nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
