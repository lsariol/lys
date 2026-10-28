package sleeper

import "testing"

func TestTeamName(t *testing.T) {
	users := []LeagueUser{
		{UserID: "one", DisplayName: "Owner", Metadata: UserMetadata{TeamName: "My Team"}},
		{UserID: "two", DisplayName: "Other Owner", Metadata: UserMetadata{TeamName: " "}},
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
		if got := TeamName(Roster{RosterID: 3, OwnerID: tc.owner}, users); got != tc.want {
			t.Errorf("owner %q: got %q, want %q", tc.owner, got, tc.want)
		}
	}
}

func TestPlayerName(t *testing.T) {
	players := map[string]Player{
		"1":   {FullName: "Jamie Runner", Position: "RB"},
		"2":   {FirstName: "Sam", LastName: "Catcher"},
		"KC":  {Position: "DEF", Team: "KC"},
		"DEF": {Position: "DEF"},
	}
	for id, want := range map[string]string{"1": "Jamie Runner", "2": "Sam Catcher", "KC": "KC D/ST", "DEF": "DEF D/ST", "9": "Player 9", "0": "Empty"} {
		if got := PlayerName(id, players); got != want {
			t.Errorf("%q: got %q, want %q", id, got, want)
		}
	}
}
