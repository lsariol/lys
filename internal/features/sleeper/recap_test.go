package sleeper

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRecapDue(t *testing.T) {
	at := func(s string) time.Time {
		t.Helper()
		v, err := time.ParseInLocation("2006-01-02 15:04", s, Eastern)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct{ last, want string }{
		{"2026-10-26 20:15", "2026-10-27 10:00"}, // Monday night: the next morning
		{"2026-11-02 20:15", "2026-11-03 10:00"}, // the day after the clocks go back: still 10 AM Eastern
		{"2026-11-01 13:00", "2026-11-03 10:00"}, // a week that ends on Sunday
		{"2026-11-03 07:00", "2026-11-10 10:00"}, // a game on Tuesday morning: not over by 10 AM
		{"2026-11-03 05:30", "2026-11-03 10:00"}, // over by 9:30 AM, so the same morning
	} {
		games := []Game{{Kickoff: at("2026-10-22 20:15")}, {Kickoff: at(tc.last)}}
		if got := RecapDue(games); !got.Equal(at(tc.want)) {
			t.Errorf("last kickoff %s: due %s, want %s", tc.last, got.In(Eastern).Format("2006-01-02 15:04 MST"), tc.want)
		}
	}
}

func testMatchup(id, roster int, points float64, starters []string, startersPoints []float64, players map[string]float64) Matchup {
	m := Matchup{RosterID: roster, Points: points, Starters: starters, StartersPoints: startersPoints, PlayersPoints: players}
	if id > 0 {
		m.MatchupID = &id
	}
	for p := range players {
		m.Players = append(m.Players, p)
	}
	return m
}

func TestBuildRecap(t *testing.T) {
	n := Names{
		Teams:    map[int]string{1: "Gridiron Gang", 2: "Benchwarmers", 3: "Sam's Team", 4: "Kyle's Team"},
		Mentions: map[int]string{1: "lu", 3: "sam", 4: "kyle"},
		Players:  map[string]Player{"chase": {FullName: "Ja'Marr Chase"}, "a": {FullName: "Player A"}, "b": {FullName: "Bench Guy"}},
	}
	week2 := []Matchup{
		testMatchup(1, 1, 132.4, []string{"a"}, []float64{20}, map[string]float64{"a": 20}),
		testMatchup(1, 4, 98.1, []string{"chase"}, []float64{38.2}, map[string]float64{"chase": 38.2}),
		testMatchup(2, 3, 120.6, []string{"a"}, []float64{10}, map[string]float64{"a": 10, "b": 41.5}),
		testMatchup(2, 2, 119.9, nil, nil, nil),
	}
	week1 := []Matchup{
		testMatchup(1, 1, 100, nil, nil, nil), testMatchup(1, 2, 90, nil, nil, nil),
		testMatchup(2, 3, 80, nil, nil, nil), testMatchup(2, 4, 85, nil, nil, nil),
	}
	got, err := BuildRecap(2, map[int][]Matchup{1: week1, 2: week2}, 15, nil, n)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"🏈 Week 2 results: @lu 132.40 def. @kyle 98.10 · @sam 120.60 def. Benchwarmers 119.90",
		"🏈 Week 2 highlights: top score @lu (132.40) · closest game @sam over Benchwarmers by 0.70 · biggest blowout @lu over @kyle by 34.30 · best player Ja'Marr Chase, 38.20 for @kyle · most left on the bench @sam (41.50)",
		"🏈 Standings after week 2: 1. @lu 2-0 · 2. @sam 1-1 · 3. @kyle 1-1", // Sam has more points
	}
	if len(got) != len(want) {
		t.Fatalf("got %d messages: %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("message %d:\n got  %q\n want %q", i, got[i], want[i])
		}
	}

	// In the playoffs: no standings, and the results say so.
	playoffs, err := BuildRecap(15, map[int][]Matchup{15: week2[:2]}, 15, nil, n)
	if err != nil || len(playoffs) != 2 || !strings.HasPrefix(playoffs[0], "🏈 Week 15 playoff results: @lu") || strings.Contains(playoffs[1], "biggest blowout") {
		t.Fatalf("playoffs: %q, %v", playoffs, err)
	}

	tied := []Matchup{testMatchup(1, 1, 100, nil, nil, nil), testMatchup(1, 2, 100, nil, nil, nil)}
	if got, _ := BuildRecap(3, map[int][]Matchup{3: tied}, 0, nil, n); !strings.Contains(got[0], "@lu and Benchwarmers tied at 100.00") {
		t.Fatalf("tie: %q", got)
	}

	unplayed := []Matchup{testMatchup(1, 1, 0, nil, nil, nil), testMatchup(1, 2, 0, nil, nil, nil)}
	if _, err := BuildRecap(3, map[int][]Matchup{3: unplayed}, 0, nil, n); !errors.Is(err, ErrNoScores) {
		t.Fatalf("no scores: %v", err)
	}
	if _, err := BuildRecap(3, nil, 0, nil, n); !errors.Is(err, ErrNoGames) {
		t.Fatalf("no games: %v", err)
	}
}
