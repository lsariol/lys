package sleeper

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/sleeper"
)

// draft is "!ff draft": when the league's draft is, from Sleeper.
func draft(ctx context.Context, e commands.Request, sl *Sleeper) (commands.Reply, error) {
	league, err := getLeague(ctx, e.Connection.CommunityID, sl)
	if err != nil {
		return broken(), err
	}
	if league == nil {
		return noLeague(e), nil
	}
	drafts, err := sl.client.Drafts(ctx, league.SleeperLeagueID)
	if err != nil {
		return unreachable(), err
	}
	if len(drafts) == 0 {
		return say("%s has no draft set up on Sleeper.", league.LeagueName), nil
	}
	return say("%s", describeDraft(drafts[0], time.Now())), nil
}

// describeDraft says where the draft is at.
func describeDraft(d api.Draft, now time.Time) string {
	at := func(ms *int64) (time.Time, bool) {
		if ms == nil || *ms <= 0 {
			return time.Time{}, false
		}
		return time.UnixMilli(*ms), true
	}
	switch d.Status {
	case "drafting":
		return "The draft is on now!"
	case "paused":
		return "The draft is paused."
	case "complete":
		if t, ok := at(d.LastPicked); ok {
			return fmt.Sprintf("The %s draft finished on %s.", d.Season, t.In(api.Eastern).Format("Mon Jan 2"))
		}
		return fmt.Sprintf("The %s draft is done.", d.Season)
	}
	start, ok := at(d.StartTime)
	if !ok {
		return fmt.Sprintf("The %s draft isn't scheduled yet.", d.Season)
	}
	when := start.In(api.Eastern).Format("Mon Jan 2, 3:04 PM") + " ET"
	if !start.After(now) {
		return fmt.Sprintf("The draft was due to start %s.", when)
	}
	return fmt.Sprintf("The draft starts in %s: %s.", until(start.Sub(now)), when)
}

// until is a duration as "3 days, 4 hours", "4 hours, 5 minutes" or "5 minutes".
func until(d time.Duration) string {
	days, hours, minutes := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	switch {
	case days > 0:
		return plural(days, "day") + ", " + plural(hours, "hour")
	case hours > 0:
		return plural(hours, "hour") + ", " + plural(minutes, "minute")
	}
	return plural(max(minutes, 1), "minute")
}
