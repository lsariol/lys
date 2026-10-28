package sleeper

import (
	"reflect"
	"testing"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

func TestFindPlayers(t *testing.T) {
	players := map[string]api.Player{
		"1":   {FullName: "Alex Smith"},
		"2":   {FullName: "Alex Smith"},
		"3":   {FullName: "Alex Smith Jr."},
		"4":   {FirstName: "Bob", LastName: "Smith"},
		"DET": {Position: "DEF", Team: "DET"},
	}
	for _, tc := range []struct {
		search string
		want   []string
	}{
		{" ALEX   smith ", []string{"1", "2"}},
		{"smith", []string{"1", "2", "3", "4"}},
		{"Bob Smith", []string{"4"}},
		{"2", []string{"2"}},
		{"det", []string{"DET"}},
		{"missing", nil},
		{" ", nil},
	} {
		if got := findPlayers(tc.search, players); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("search %q: got %v, want %v", tc.search, got, tc.want)
		}
	}
}

func TestRosterHasPlayer(t *testing.T) {
	for _, roster := range []api.Roster{
		{Players: []string{"1"}},
		{Starters: []string{"1"}},
		{Reserve: []string{"1"}},
		{Taxi: []string{"1"}},
	} {
		if !rosterHasPlayer(roster, "1") || rosterHasPlayer(roster, "2") {
			t.Errorf("incorrect ownership for %+v", roster)
		}
	}
	if rosterHasPlayer(api.Roster{}, "1") {
		t.Fatal("empty roster should not own a player")
	}
}
