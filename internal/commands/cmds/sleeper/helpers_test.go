package sleeper

import (
	"testing"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

func TestSortStandings(t *testing.T) {
	rosters := []api.Roster{
		{RosterID: 4, Settings: api.RosterSettings{Wins: 1, Losses: 1, Fpts: 100, FptsDecimal: 25}},
		{RosterID: 3, Settings: api.RosterSettings{Ties: 2, Fpts: 100, FptsDecimal: 26}},
		{RosterID: 2, Settings: api.RosterSettings{Wins: 2, Losses: 2, Fpts: 100, FptsDecimal: 26}},
		{RosterID: 1, Settings: api.RosterSettings{Wins: 1, Fpts: 50}},
		{RosterID: 5},
	}

	sorted := sortStandings(rosters)
	want := []int{1, 2, 3, 4, 5}
	for i, id := range want {
		if sorted[i].RosterID != id {
			t.Fatalf("position %d: got roster %d, want %d", i, sorted[i].RosterID, id)
		}
	}
	if rosters[0].RosterID != 4 {
		t.Fatal("sorting changed the original rosters")
	}
	if len(sortStandings(nil)) != 0 {
		t.Fatal("empty league should have no standings")
	}
}
