package sleeper

import (
	"context"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/commands"
)

func TestPlayoffRound(t *testing.T) {
	standard, firstPlace, winner := 0, 1, 4
	league := League{Season: "2026", Settings: SleeperLeagueSettings{PlayoffWeekStart: 15, PlayoffTeams: 6, PlayoffRoundType: &standard}}
	bracket := []SleeperPlayoffMatchup{{Round: 1, MatchID: 1}, {Round: 2, MatchID: 2}, {Round: 3, MatchID: 3, Place: &firstPlace}}
	for _, tc := range []struct {
		week  int
		round int
	}{{14, 0}, {15, 1}, {16, 2}, {17, 3}} {
		round, _, message := getPlayoffRound(league, SleeperNFLState{Season: "2026", SeasonType: "regular", Week: tc.week}, bracket)
		if round != tc.round || (message != "") != (tc.round == 0) {
			t.Errorf("week %d: round %d, message %q", tc.week, round, message)
		}
	}
	bracket[2].Winner = &winner
	round, week, message := getPlayoffRound(league, SleeperNFLState{Season: "2027"}, bracket)
	if round != 3 || week != 17 || message != "" {
		t.Fatalf("historical championship: %d, %d, %q", round, week, message)
	}
	otherFormat := 1
	league.Settings.PlayoffRoundType = &otherFormat
	if _, _, message := getPlayoffRound(league, SleeperNFLState{}, bracket); message == "" {
		t.Fatal("unsupported round format accepted")
	}
}

func TestPlayoffParticipantsAndScores(t *testing.T) {
	matchID, winner, loser, direct := 1, 4, 5, 6
	bracket := []SleeperPlayoffMatchup{{MatchID: matchID, Winner: &winner, Loser: &loser}}
	if got := resolvePlayoffTeam(nil, &SleeperPlayoffSource{Winner: &matchID}, bracket); got == nil || *got != winner {
		t.Fatal("winner reference not resolved")
	}
	if got := resolvePlayoffTeam(&direct, &SleeperPlayoffSource{Winner: &matchID}, bracket); got == nil || *got != direct {
		t.Fatal("explicit bracket pairing must take priority")
	}
	if got := resolvePlayoffTeam(nil, &SleeperPlayoffSource{Loser: &matchID}, bracket); got == nil || *got != loser {
		t.Fatal("loser reference not resolved")
	}
	if got := formatPlayoffTeam(nil, nil, nil, nil); got != "TBD" {
		t.Fatal(got)
	}
	if got := formatPlayoffTeam(&winner, nil, nil, nil); got != "Team 4 (score unavailable)" {
		t.Fatal(got)
	}
	zero := 0.0
	if got := formatPlayoffTeam(&winner, nil, nil, []SleeperMatchup{{RosterID: winner, Points: 100, CustomPoints: &zero}}); got != "Team 4 0.00" {
		t.Fatal(got)
	}
}

func TestPlayoffsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"playoffs", "0"}, {"playoffs", "-1"}, {"playoffs", "abc"}, {"playoffs", "1", "extra"}} {
		response, err := playoffs(context.Background(), commands.Request{Args: args}, nil)
		if err != nil || !strings.HasPrefix(response.Text, "Usage:") {
			t.Errorf("args %v: %q, %v", args, response.Text, err)
		}
	}
}
