package sleeper

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

func TestRosterEntries(t *testing.T) {
	roster := api.Roster{Starters: []string{"1", "0"}, Players: []string{"1", "2", "3", "4", "2"}, Reserve: []string{"3"}, Taxi: []string{"4"}}
	players := map[string]api.Player{
		"1": {FullName: "Starter"}, "2": {FirstName: "Bench", LastName: "Player"},
		"3": {FullName: "Reserve"}, "4": {FullName: "Prospect"},
	}
	got := getRosterEntries(roster, []string{"QB", "FLEX", "BN"}, players)
	want := []string{"QB Starter", "FLEX Empty", "BN Bench Player", "IR Reserve", "Taxi Prospect"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	entry := formatRosterEntry("WR", strings.Repeat("界", 100))
	if len(entry) > 80 || !utf8.ValidString(entry) {
		t.Fatal("entry exceeds the byte limit or contains broken UTF-8")
	}
}
