package sleeper

import "testing"

func TestSortStandings(t *testing.T) {
	rosters := []SleeperRoster{
		{RosterID: 4, Settings: SleeperRosterSettings{Wins: 1, Losses: 1, Fpts: 100, FptsDecimal: 25}},
		{RosterID: 3, Settings: SleeperRosterSettings{Ties: 2, Fpts: 100, FptsDecimal: 26}},
		{RosterID: 2, Settings: SleeperRosterSettings{Wins: 2, Losses: 2, Fpts: 100, FptsDecimal: 26}},
		{RosterID: 1, Settings: SleeperRosterSettings{Wins: 1, Fpts: 50}},
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

func TestGetTeamName(t *testing.T) {
	users := []SleeperLeagueUser{
		{UserID: "one", DisplayName: "Owner", Metadata: SleeperUserMetadata{TeamName: "My Team"}},
		{UserID: "two", DisplayName: "Other Owner", Metadata: SleeperUserMetadata{TeamName: " "}},
	}
	for _, tc := range []struct {
		owner string
		want  string
	}{
		{"one", "My Team"},
		{"two", "Other Owner"},
		{"missing", "Team 3"},
		{"", "Team 3"},
	} {
		if got := getTeamName(SleeperRoster{RosterID: 3, OwnerID: tc.owner}, users); got != tc.want {
			t.Errorf("owner %q: got %q, want %q", tc.owner, got, tc.want)
		}
	}
}
