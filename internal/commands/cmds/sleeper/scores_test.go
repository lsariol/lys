package sleeper

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

func TestCurrentWeek(t *testing.T) {
	for _, tc := range []struct {
		state api.NFLState
		want  int
	}{
		{api.NFLState{Season: "2026", SeasonType: "regular", Week: 2}, 2},
		{api.NFLState{Season: "2025", SeasonType: "regular", Week: 2}, 0},
		{api.NFLState{Season: "2026", SeasonType: "pre", Week: 2}, 0},
		{api.NFLState{Season: "2026", SeasonType: "post", Week: 2}, 0},
		{api.NFLState{Season: "2026", SeasonType: "regular", Week: 0}, 0},
	} {
		week, message := getCurrentWeek(SleeperLeague{Season: "2026"}, tc.state)
		if week != tc.want || (message != "") != (tc.want == 0) {
			t.Errorf("state %+v: got week %d, message %q", tc.state, week, message)
		}
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
		deps := &Sleeper{client: api.New(&http.Client{Transport: scoresTransport(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/league/test/matchups/2" {
				t.Errorf("unexpected path %s", req.URL.Path)
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})})}
		matchups, err := deps.client.Matchups(context.Background(), "test", 2)
		if (err != nil) != tc.wantError {
			t.Fatalf("body %s: error %v", tc.body, err)
		}
		if err == nil && len(*matchups) == 2 {
			if api.Points((*matchups)[0]) != 0 || api.Points((*matchups)[1]) != 4.25 || (*matchups)[0].MatchupID != nil {
				t.Fatal("null fields or zero score override handled incorrectly")
			}
		}
	}
}
