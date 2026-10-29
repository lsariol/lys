package sleeper

import (
	"testing"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

func TestPlayoffRound(t *testing.T) {
	standard, firstPlace, winner := 0, 1, 4
	league := api.League{Season: "2026", Settings: api.LeagueSettings{PlayoffWeekStart: 15, PlayoffTeams: 6, PlayoffRoundType: &standard}}
	bracket := []api.PlayoffMatchup{{Round: 1, MatchID: 1}, {Round: 2, MatchID: 2}, {Round: 3, MatchID: 3, Place: &firstPlace}}
	for _, tc := range []struct {
		week  int
		round int
	}{{14, 0}, {15, 1}, {16, 2}, {17, 3}} {
		round, _, message := getPlayoffRound(league, api.NFLState{Season: "2026", SeasonType: "regular", Week: tc.week}, bracket)
		if round != tc.round || (message != "") != (tc.round == 0) {
			t.Errorf("week %d: round %d, message %q", tc.week, round, message)
		}
	}
	bracket[2].Winner = &winner
	round, week, message := getPlayoffRound(league, api.NFLState{Season: "2027"}, bracket)
	if round != 3 || week != 17 || message != "" {
		t.Fatalf("historical championship: %d, %d, %q", round, week, message)
	}
	otherFormat := 1
	league.Settings.PlayoffRoundType = &otherFormat
	if _, _, message := getPlayoffRound(league, api.NFLState{}, bracket); message == "" {
		t.Fatal("unsupported round format accepted")
	}
}

func TestPlayoffParticipantsAndScores(t *testing.T) {
	matchID, winner, loser, direct := 1, 4, 5, 6
	bracket := []api.PlayoffMatchup{{MatchID: matchID, Winner: &winner, Loser: &loser}}
	if got := resolvePlayoffTeam(nil, &api.PlayoffSource{Winner: &matchID}, bracket); got == nil || *got != winner {
		t.Fatal("winner reference not resolved")
	}
	if got := resolvePlayoffTeam(&direct, &api.PlayoffSource{Winner: &matchID}, bracket); got == nil || *got != direct {
		t.Fatal("explicit bracket pairing must take priority")
	}
	if got := resolvePlayoffTeam(nil, &api.PlayoffSource{Loser: &matchID}, bracket); got == nil || *got != loser {
		t.Fatal("loser reference not resolved")
	}
	if got := formatPlayoffTeam(nil, api.Names{}, nil); got != "TBD" {
		t.Fatal(got)
	}
	if got := formatPlayoffTeam(&winner, api.Names{Mentions: map[int]string{4: "lu"}}, nil); got != "@lu" {
		t.Fatal(got)
	}
	zero := 0.0
	if got := formatPlayoffTeam(&winner, api.Names{}, []api.Matchup{{RosterID: winner, Points: 100, CustomPoints: &zero}}); got != "Team 4 0.00" {
		t.Fatal(got)
	}
}
