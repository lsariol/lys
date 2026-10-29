package sleeper

import (
	"fmt"
	"sort"
	"strings"
)

// Points is the matchup's score, the commissioner's when they set one.
func Points(m Matchup) float64 {
	if m.CustomPoints != nil {
		return *m.CustomPoints
	}
	return m.Points
}

// GroupMatchups pairs the teams that play each other, by roster ID; a team
// without an opponent is a group of one.
func GroupMatchups(matchups []Matchup) [][]Matchup {
	sorted := append([]Matchup(nil), matchups...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].RosterID < sorted[j].RosterID })

	var groups [][]Matchup
	index := make(map[int]int)
	for _, m := range sorted {
		if m.MatchupID == nil || *m.MatchupID <= 0 {
			groups = append(groups, []Matchup{m})
			continue
		}
		if i, ok := index[*m.MatchupID]; ok {
			groups[i] = append(groups[i], m)
			continue
		}
		index[*m.MatchupID] = len(groups)
		groups = append(groups, []Matchup{m})
	}
	return groups
}

// Ordinal is 1st, 2nd, 3rd, 4th and so on.
func Ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// List joins names as "A", "A and B" or "A, B and C".
func List(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Record is a win-loss record, with ties only when there are some.
func Record(wins, losses, ties int) string {
	if ties > 0 {
		return fmt.Sprintf("%d-%d-%d", wins, losses, ties)
	}
	return fmt.Sprintf("%d-%d", wins, losses)
}
