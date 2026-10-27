package sleeper

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/commands"
)

func TestCurrentWeek(t *testing.T) {
	for _, tc := range []struct {
		state SleeperNFLState
		want  int
	}{
		{SleeperNFLState{Season: "2026", SeasonType: "regular", Week: 2}, 2},
		{SleeperNFLState{Season: "2025", SeasonType: "regular", Week: 2}, 0},
		{SleeperNFLState{Season: "2026", SeasonType: "pre", Week: 2}, 0},
		{SleeperNFLState{Season: "2026", SeasonType: "post", Week: 2}, 0},
		{SleeperNFLState{Season: "2026", SeasonType: "regular", Week: 0}, 0},
	} {
		week, message := getCurrentWeek(SleeperLeague{Season: "2026"}, tc.state)
		if week != tc.want || (message != "") != (tc.want == 0) {
			t.Errorf("state %+v: got week %d, message %q", tc.state, week, message)
		}
	}
}

func TestGroupMatchups(t *testing.T) {
	id := 7
	matchups := []SleeperMatchup{
		{RosterID: 4},
		{RosterID: 3, MatchupID: &id},
		{RosterID: 2},
		{RosterID: 1, MatchupID: &id},
	}
	groups := groupMatchups(matchups)
	if len(groups) != 3 || len(groups[0]) != 2 || groups[0][0].RosterID != 1 || groups[0][1].RosterID != 3 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	if groups[1][0].RosterID != 2 || groups[2][0].RosterID != 4 || matchups[0].RosterID != 4 {
		t.Fatal("unpaired teams were combined or original order changed")
	}
	if len(groupMatchups(nil)) != 0 {
		t.Fatal("empty matchups should produce no groups")
	}
}

type scoresTransport func(*http.Request) (*http.Response, error)

func (transport scoresTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func TestMatchupAPI(t *testing.T) {
	for _, tc := range []struct {
		body      string
		status    int
		wantError bool
	}{
		{`[{"roster_id":1,"matchup_id":null,"points":12.75,"custom_points":0},{"roster_id":2,"points":4.25,"custom_points":null}]`, 200, false},
		{`[]`, 200, false},
		{`null`, 200, false},
		{`invalid`, 200, true},
		{`{}`, 503, true},
	} {
		deps := &Sleeper{http: &http.Client{Transport: scoresTransport(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/league/test/matchups/2" {
				t.Errorf("unexpected path %s", req.URL.Path)
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})}}
		matchups, err := getMatchups("test", 2, context.Background(), deps)
		if (err != nil) != tc.wantError {
			t.Fatalf("body %s: error %v", tc.body, err)
		}
		if err == nil && len(*matchups) == 2 {
			if getMatchupPoints((*matchups)[0]) != 0 || getMatchupPoints((*matchups)[1]) != 4.25 || (*matchups)[0].MatchupID != nil {
				t.Fatal("null fields or zero score override handled incorrectly")
			}
		}
	}
}

func TestScoresInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"scores", "0"}, {"scores", "-1"}, {"scores", "abc"}, {"scores", "99999999999999999999999999"}, {"scores", "1", "extra"}} {
		response, err := scores(context.Background(), commands.Request{Args: args}, nil)
		if err != nil || !strings.HasPrefix(response.Text, "Usage:") {
			t.Errorf("args %v: response %q, error %v", args, response.Text, err)
		}
	}
}
