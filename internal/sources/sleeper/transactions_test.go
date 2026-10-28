package sleeper

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

func testNames() transactionNames {
	return transactionNames{
		team:   func(r int) string { return fmt.Sprintf("Team%d", r) },
		player: func(p string) string { return "P" + p },
	}
}

func TestDescribeTrade(t *testing.T) {
	var tx api.Transaction
	if err := json.Unmarshal([]byte(`{
		"transaction_id": "t1", "type": "trade", "status": "complete",
		"roster_ids": [1, 2],
		"adds": {"11": 1, "22": 2, "23": 2},
		"drops": {"11": 2, "22": 1, "23": 1},
		"draft_picks": [{"season": "2027", "round": 2, "roster_id": 1, "previous_owner_id": 1, "owner_id": 2}],
		"waiver_budget": [{"sender": 2, "receiver": 1, "amount": 15}]
	}`), &tx); err != nil {
		t.Fatal(err)
	}
	got, ok := describeTransaction(tx, testNames())
	if want := "🏈 Trade: Team1 gets P11, $15 FAAB | Team2 gets P22, P23, 2027 2nd-round pick"; !ok || got != want {
		t.Fatalf("got %q, %v\nwant %q", got, ok, want)
	}
}

func TestDescribeWaiver(t *testing.T) {
	bid := 44
	tx := api.Transaction{Type: "waiver", Status: "complete", RosterIDs: []int{3}, Adds: map[string]int{"7": 3}, Drops: map[string]int{"8": 3}}
	tx.Settings = &struct {
		WaiverBid *int `json:"waiver_bid"`
	}{&bid}
	if got, ok := describeTransaction(tx, testNames()); !ok || got != "🏈 Waiver: Team3 adds P7 ($44), drops P8" {
		t.Fatalf("got %q, %v", got, ok)
	}

	for name, tx := range map[string]api.Transaction{
		"failed claim": {Type: "waiver", Status: "failed", RosterIDs: []int{3}, Adds: map[string]int{"7": 3}},
		"free agent":   {Type: "free_agent", Status: "complete", RosterIDs: []int{3}, Adds: map[string]int{"7": 3}},
		"drop only":    {Type: "waiver", Status: "complete", RosterIDs: []int{3}, Drops: map[string]int{"7": 3}},
	} {
		if got, ok := describeTransaction(tx, testNames()); ok {
			t.Errorf("%s announced: %q", name, got)
		}
	}
}

func TestTruncateAndOrdinal(t *testing.T) {
	if got := truncate(strings.Repeat("é", 400)); len(got) > 480 || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncate: %d bytes", len(got))
	}
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q", n, got)
		}
	}
}
