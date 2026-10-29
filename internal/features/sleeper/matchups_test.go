package sleeper

import "testing"

func TestGroupMatchups(t *testing.T) {
	id := 7
	matchups := []Matchup{{RosterID: 4}, {RosterID: 3, MatchupID: &id}, {RosterID: 2}, {RosterID: 1, MatchupID: &id}}
	groups := GroupMatchups(matchups)
	if len(groups) != 3 || len(groups[0]) != 2 || groups[0][0].RosterID != 1 || groups[0][1].RosterID != 3 {
		t.Fatalf("groups: %+v", groups)
	}
	if groups[1][0].RosterID != 2 || groups[2][0].RosterID != 4 || matchups[0].RosterID != 4 {
		t.Fatal("unpaired teams were combined, or the input was reordered")
	}
	if len(GroupMatchups(nil)) != 0 {
		t.Fatal("no matchups, no groups")
	}
}

func TestWords(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{List(nil), ""},
		{List([]string{"A"}), "A"},
		{List([]string{"A", "B"}), "A and B"},
		{List([]string{"A", "B", "C"}), "A, B and C"},
		{Ordinal(1) + Ordinal(2) + Ordinal(3) + Ordinal(4), "1st2nd3rd4th"},
		{Ordinal(11) + Ordinal(12) + Ordinal(13) + Ordinal(21) + Ordinal(112), "11th12th13th21st112th"},
		{Record(6, 2, 0), "6-2"},
		{Record(5, 2, 1), "5-2-1"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}
