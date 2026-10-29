package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// trades is "!ff trades [@user]": the league's last 3 trades, or one
// team's, from the trades Lys has seen. Each trade is its own message.
func trades(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	if len(e.Args) > 2 {
		return usage(e, "trades [@user]"), nil
	}
	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	rosters, users, err := sl.leagueData(ctx, league)
	if err != nil {
		return unreachable(), err
	}

	roster := 0
	if len(e.Args) == 2 {
		name, self := targetUser(e, e.Args[1:])
		r, reply, err := sl.member(ctx, e, league, name, self, rosters)
		if reply != nil {
			return *reply, err
		}
		roster = r.RosterID
	}

	rows, err := sl.db.Query(ctx, `
		SELECT event_data, occurred_at FROM lys.sleeper_events
		WHERE sleeper_league_id = $1 AND event_type = 'trade'
		  AND ($2 = 0 OR event_data->'roster_ids' @> to_jsonb($2::int))
		ORDER BY occurred_at DESC LIMIT 3`, league.SleeperLeagueID, roster)
	if err != nil {
		return broken(), err
	}
	defer rows.Close()

	var players map[string]api.Player
	if p, err := sl.client.Players(ctx); err == nil {
		players = p
	}
	n := sl.names(ctx, league, rosters, users, players)
	var texts []string
	for rows.Next() {
		var data []byte
		var at time.Time
		if err := rows.Scan(&data, &at); err != nil {
			return broken(), err
		}
		var tx api.Transaction
		if err := json.Unmarshal(data, &tx); err != nil {
			return broken(), fmt.Errorf("sleeper trades: read a trade: %w", err)
		}
		texts = append(texts, fmt.Sprintf("🏈 Trade, %s: %s", at.In(api.Eastern).Format("Jan 2"), api.TradeSides(tx, n)))
	}
	if err := rows.Err(); err != nil {
		return broken(), err
	}
	if len(texts) == 0 {
		if roster != 0 {
			return say("%s hasn't made a trade yet.", n.Who(roster)), nil
		}
		return say("No trades in %s yet.", league.LeagueName), nil
	}
	return commands.Reply{Text: texts[0], More: texts[1:]}, nil
}
