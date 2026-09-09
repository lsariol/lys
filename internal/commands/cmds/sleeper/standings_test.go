package sleeper

import (
	"context"
	"strings"
	"testing"

	"github.com/lsariol/botsuite/internal/adapters/adapter"
)

func TestStandingsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"standings", "0"},
		{"standings", "-1"},
		{"standings", "abc"},
		{"standings", "999999999999999999999999999999"},
		{"standings", "1", "extra"},
	} {
		response, err := standings(context.Background(), adapter.Envelope{Args: args}, nil)
		if err != nil || !strings.HasPrefix(response.Text, "Usage:") {
			t.Errorf("args %v: got response %q, error %v", args, response.Text, err)
		}
	}
}
