package sleeper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	// ErrNoScores means the week's games have no scores yet.
	ErrNoScores = errors.New("Sleeper: the week has no scores yet")
	// ErrNoGames means the league plays no games that week.
	ErrNoGames = errors.New("Sleeper: the league has no games that week")
)

// RecapDue is when a week's recap goes out: 10 AM Eastern on the first
// Tuesday at least 4 hours after the week's last kickoff, once its last game
// is over. games is the week's schedule, earliest first.
func RecapDue(games []Game) time.Time {
	after := games[len(games)-1].Kickoff.Add(4 * time.Hour).In(Eastern)
	due := time.Date(after.Year(), after.Month(), after.Day(), 10, 0, 0, 0, Eastern)
	for due.Before(after) || due.Weekday() != time.Tuesday {
		due = time.Date(due.Year(), due.Month(), due.Day()+1, 10, 0, 0, 0, Eastern)
	}
	return due
}

// Recap fetches what a week's recap needs from Sleeper and builds it.
func (c *Client) Recap(ctx context.Context, leagueID string, week int, n Names) ([]string, error) {
	league, err := c.League(ctx, leagueID)
	if err != nil {
		return nil, err
	}
	rosters, err := c.Rosters(ctx, leagueID)
	if err != nil {
		return nil, err
	}
	playoffs := league.Settings.PlayoffWeekStart
	from := 1
	if playoffs > 0 && week >= playoffs {
		from = week // the regular season's standings are settled
	}
	weeks := make(map[int][]Matchup)
	for w := from; w <= week; w++ {
		m, err := c.Matchups(ctx, leagueID, w)
		if err != nil {
			return nil, err
		}
		weeks[w] = *m
	}
	return BuildRecap(week, weeks, playoffs, *rosters, n)
}

// BuildRecap is a week's recap, one string per message: the results, the
// highlights and, in the regular season, the top 3 in the standings. weeks
// has the week's matchups, and in the regular season every week before it.
func BuildRecap(week int, weeks map[int][]Matchup, playoffStart int, rosters []Roster, n Names) ([]string, error) {
	matchups := weeks[week]
	if len(matchups) == 0 {
		return nil, ErrNoGames
	}
	scored := false
	for _, m := range matchups {
		if Points(m) != 0 {
			scored = true
		}
	}
	if !scored {
		return nil, ErrNoScores
	}

	var games []game
	for _, g := range GroupMatchups(matchups) {
		if len(g) == 2 {
			games = append(games, newGame(g[0], g[1]))
		}
	}
	if len(games) == 0 {
		return nil, ErrNoGames
	}

	label := fmt.Sprintf("Week %d", week)
	if playoffStart > 0 && week >= playoffStart {
		label += " playoff"
	}
	var results []string
	for _, g := range games {
		results = append(results, g.result(n))
	}
	out := []string{fmt.Sprintf("🏈 %s results: %s", label, strings.Join(results, " · "))}

	if h := highlights(games, matchups, rosters, n); len(h) > 0 {
		out = append(out, fmt.Sprintf("🏈 Week %d highlights: %s", week, strings.Join(h, " · ")))
	}
	if playoffStart == 0 || week < playoffStart {
		if s := topStandings(week, weeks, n, 3); s != "" {
			out = append(out, fmt.Sprintf("🏈 Standings after week %d: %s", week, s))
		}
	}
	return out, nil
}

// game is one decided pairing, the higher score first.
type game struct {
	winner, loser Matchup
	tied          bool
}

func newGame(a, b Matchup) game {
	if Points(b) > Points(a) {
		a, b = b, a
	}
	return game{winner: a, loser: b, tied: cents(Points(a)) == cents(Points(b))}
}

func (g game) margin() float64 { return Points(g.winner) - Points(g.loser) }

func (g game) result(n Names) string {
	if g.tied {
		return fmt.Sprintf("%s and %s tied at %.2f", n.Who(g.winner.RosterID), n.Who(g.loser.RosterID), Points(g.winner))
	}
	return fmt.Sprintf("%s %.2f def. %s %.2f", n.Who(g.winner.RosterID), Points(g.winner), n.Who(g.loser.RosterID), Points(g.loser))
}

// highlights are the week's top score, closest game, biggest blowout, best
// starter and most points left on a bench.
func highlights(games []game, matchups []Matchup, rosters []Roster, n Names) []string {
	var out []string

	top := matchups[0]
	for _, m := range matchups {
		if Points(m) > Points(top) {
			top = m
		}
	}
	out = append(out, fmt.Sprintf("top score %s (%.2f)", n.Who(top.RosterID), Points(top)))

	closest, blowout := games[0], games[0]
	for _, g := range games {
		if g.margin() < closest.margin() {
			closest = g
		}
		if g.margin() > blowout.margin() {
			blowout = g
		}
	}
	if closest.tied {
		out = append(out, fmt.Sprintf("closest game %s and %s, tied", n.Who(closest.winner.RosterID), n.Who(closest.loser.RosterID)))
	} else {
		out = append(out, fmt.Sprintf("closest game %s over %s by %.2f", n.Who(closest.winner.RosterID), n.Who(closest.loser.RosterID), closest.margin()))
	}
	if len(games) > 1 {
		out = append(out, fmt.Sprintf("biggest blowout %s over %s by %.2f", n.Who(blowout.winner.RosterID), n.Who(blowout.loser.RosterID), blowout.margin()))
	}

	bestPoints, bestPlayer, bestRoster := 0.0, "", 0
	for _, m := range matchups {
		for i, p := range m.Starters {
			if i < len(m.StartersPoints) && p != "" && p != "0" && m.StartersPoints[i] > bestPoints {
				bestPoints, bestPlayer, bestRoster = m.StartersPoints[i], p, m.RosterID
			}
		}
	}
	if bestPlayer != "" {
		out = append(out, fmt.Sprintf("best player %s, %.2f for %s", n.Player(bestPlayer), bestPoints, n.Who(bestRoster)))
	}

	benchPoints, benchRoster := 0.0, 0
	for _, m := range matchups {
		if b := bench(m, rosters); b > benchPoints {
			benchPoints, benchRoster = b, m.RosterID
		}
	}
	if benchPoints > 0 {
		out = append(out, fmt.Sprintf("most left on the bench %s (%.2f)", n.Who(benchRoster), benchPoints))
	}
	return out
}

// bench is the points scored by the team's players it didn't start, leaving
// out its injured reserve and taxi squad.
func bench(m Matchup, rosters []Roster) float64 {
	skip := make(map[string]bool)
	for _, p := range m.Starters {
		skip[p] = true
	}
	for _, r := range rosters {
		if r.RosterID == m.RosterID {
			for _, p := range append(append([]string(nil), r.Reserve...), r.Taxi...) {
				skip[p] = true
			}
		}
	}
	total := 0.0
	for _, p := range m.Players {
		if !skip[p] {
			total += m.PlayersPoints[p]
		}
	}
	return total
}

// topStandings is the top teams after week, by record then points scored,
// counted from each week's matchups (Sleeper's own records can lag a day).
func topStandings(week int, weeks map[int][]Matchup, n Names, top int) string {
	type standing struct {
		roster             int
		wins, losses, ties int
		points             float64
	}
	table := make(map[int]*standing)
	get := func(roster int) *standing {
		if table[roster] == nil {
			table[roster] = &standing{roster: roster}
		}
		return table[roster]
	}
	for w := 1; w <= week; w++ {
		for _, g := range GroupMatchups(weeks[w]) {
			for _, m := range g {
				get(m.RosterID).points += Points(m)
			}
			if len(g) != 2 {
				continue
			}
			result := newGame(g[0], g[1])
			switch {
			case result.tied:
				get(g[0].RosterID).ties++
				get(g[1].RosterID).ties++
			default:
				get(result.winner.RosterID).wins++
				get(result.loser.RosterID).losses++
			}
		}
	}

	var all []*standing
	for _, s := range table {
		all = append(all, s)
	}
	pct := func(s *standing) float64 {
		games := s.wins + s.losses + s.ties
		if games == 0 {
			return 0
		}
		return (float64(s.wins) + float64(s.ties)/2) / float64(games)
	}
	sort.Slice(all, func(i, j int) bool {
		if pct(all[i]) != pct(all[j]) {
			return pct(all[i]) > pct(all[j])
		}
		if cents(all[i].points) != cents(all[j].points) {
			return all[i].points > all[j].points
		}
		return all[i].roster < all[j].roster
	})

	var entries []string
	for i, s := range all {
		if i == top {
			break
		}
		entries = append(entries, fmt.Sprintf("%d. %s %s", i+1, n.Who(s.roster), Record(s.wins, s.losses, s.ties)))
	}
	return strings.Join(entries, " · ")
}

// cents compares scores at the two decimals chat shows.
func cents(points float64) int64 { return int64(math.Round(points * 100)) }
