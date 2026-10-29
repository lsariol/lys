package sleeper

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// testNames links roster 1 to the viewer "lu"; roster 2 has only a team name.
func testNames() Names {
	return Names{
		Teams:    map[int]string{1: "Gridiron Gang", 2: "Benchwarmers"},
		Mentions: map[int]string{1: "lu"},
		Players:  map[string]Player{"7": {FullName: "Jaylen Warren"}, "8": {FullName: "Zach Ertz"}, "9": {FullName: "Rashid Shaheed"}},
	}
}

func TestDescribeTrade(t *testing.T) {
	var tx Transaction
	if err := json.Unmarshal([]byte(`{
		"type": "trade", "status": "complete", "roster_ids": [1, 2],
		"adds": {"7": 1, "8": 2, "9": 2},
		"draft_picks": [{"season": "2027", "round": 2, "owner_id": 2}],
		"waiver_budget": [{"sender": 2, "receiver": 1, "amount": 15}]
	}`), &tx); err != nil {
		t.Fatal(err)
	}
	want := "🏈 Trade: @lu gets Jaylen Warren, $15 FAAB | Benchwarmers gets Zach Ertz, Rashid Shaheed, 2027 2nd-round pick"
	if got := DescribeTrade(tx, testNames()); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDescribeMoves(t *testing.T) {
	bid := 12
	claim := Transaction{Type: "waiver", RosterIDs: []int{1}, Adds: map[string]int{"9": 1}, Drops: map[string]int{"8": 1}}
	claim.Settings = &struct {
		WaiverBid *int `json:"waiver_bid"`
	}{&bid}
	pickup := Transaction{Type: "free_agent", RosterIDs: []int{1}, Adds: map[string]int{"7": 1}, Drops: map[string]int{"8": 1}}
	second := Transaction{Type: "free_agent", RosterIDs: []int{1}, Adds: map[string]int{"9": 1}}
	dropOnly := Transaction{Type: "free_agent", RosterIDs: []int{2}, Drops: map[string]int{"7": 2}}

	for name, tc := range map[string]struct {
		kind   string
		roster int
		txs    []Transaction
		want   string
	}{
		"two pickups": {"free_agent", 1, []Transaction{pickup, second}, "🏈 @lu picked up Jaylen Warren and Rashid Shaheed, dropped Zach Ertz"},
		"drop only":   {"free_agent", 2, []Transaction{dropOnly}, "🏈 Benchwarmers dropped Jaylen Warren"},
		"waiver bid":  {"waiver", 1, []Transaction{claim}, "🏈 Waivers: @lu claimed Rashid Shaheed ($12), dropped Zach Ertz"},
		"nothing":     {"free_agent", 2, []Transaction{pickup}, ""},
	} {
		if got := DescribeMoves(tc.kind, tc.roster, tc.txs, testNames()); got != tc.want {
			t.Errorf("%s:\n got  %q\n want %q", name, got, tc.want)
		}
	}

	// Too many to list in one message: a count instead.
	n := testNames()
	var many []Transaction
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("p%d", i)
		n.Players[id] = Player{FullName: "Somebody With A Long Name " + id}
		many = append(many, Transaction{Type: "free_agent", RosterIDs: []int{1}, Adds: map[string]int{id: 1}})
	}
	if got := DescribeMoves("free_agent", 1, many, n); got != "🏈 @lu made 30 free agent moves" {
		t.Fatalf("many: %q", got)
	}
	if one := DescribeMoves("waiver", 1, many[:1], n); utf8.RuneCountInString(one) > MaxMessage || !strings.HasPrefix(one, "🏈 Waivers: @lu claimed") {
		t.Fatalf("one: %q", one)
	}
}

func TestNamesWho(t *testing.T) {
	n := testNames()
	n.Teams[3] = "  A   Very Long Team Name That Goes On  "
	for _, tc := range []struct{ got, want string }{
		{n.Who(1), "@lu"},
		{n.Who(2), "Benchwarmers"},
		{n.Who(4), "Team 4"},
		{n.TeamAndWho(1), "Gridiron Gang @lu"},
		{n.TeamAndWho(2), "Benchwarmers"},
		{n.Team(3), "A Very Long Team Name That ..."},
		{n.Player("7"), "Jaylen Warren"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}
