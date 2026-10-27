package eventsub

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/core"
)

func TestDiffChannels(t *testing.T) {
	joined := map[string]Channel{"1": {UserID: "1"}, "2": {UserID: "2"}}
	join, leave := diffChannels(joined, []core.Channel{{ID: "2"}, {ID: "3", Name: "New"}})
	sort.Strings(leave)
	if len(join) != 1 || join[0].ID != "3" || join[0].Name != "New" || len(leave) != 1 || leave[0] != "1" {
		t.Fatalf("join %v, leave %v", join, leave)
	}
	if join, leave := diffChannels(joined, []core.Channel{{ID: "1"}, {ID: "2"}}); len(join)+len(leave) != 0 {
		t.Fatalf("no change: join %v, leave %v", join, leave)
	}
}

func TestSubscribeError(t *testing.T) {
	resp := func(status int, body string) *http.Response {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
	}
	for _, tc := range []struct {
		resp *http.Response
		want string
	}{
		{resp(400, `{"error":"Bad Request","status":400,"message":"invalid broadcaster_user_id"}`), "status 400: invalid broadcaster_user_id (check the channel's Twitch ID"},
		{resp(403, `{"message":"subscription missing proper authorization"}`), "status 403: subscription missing proper authorization (the bot needs user:read:chat"},
		{resp(500, ``), "status 500: unreadable answer"},
		{resp(503, `{}`), "status 503: Twitch gave no reason"},
	} {
		if got := subscribeError(tc.resp).Error(); !strings.Contains(got, tc.want) {
			t.Errorf("got %q, want it to contain %q", got, tc.want)
		}
	}
}
