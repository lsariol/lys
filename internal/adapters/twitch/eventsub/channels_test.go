package eventsub

import (
	"sort"
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
