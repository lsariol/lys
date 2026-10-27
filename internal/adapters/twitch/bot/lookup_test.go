package bot

import (
	"context"
	"errors"
	"testing"

	"github.com/lsariol/lys/internal/core"
)

func TestLookupChannelRefusesNonNumericIDs(t *testing.T) {
	c := &TwitchClient{} // must not reach Twitch
	for _, id := range []string{"", "<id>", "12a", " 12"} {
		if _, err := c.LookupChannel(context.Background(), id); !errors.Is(err, core.ErrNoSuchChannel) {
			t.Errorf("%q: %v", id, err)
		}
	}
}
