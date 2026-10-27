package cli

import (
	"testing"

	"github.com/lsariol/lys/internal/connections"
)

func TestState(t *testing.T) {
	for want, c := range map[string]connections.Connection{
		"active":                         {Authorized: true, WasAuthorized: true, Enabled: true},
		"revoked":                        {WasAuthorized: true, Enabled: true},
		"not authorized":                 {Enabled: true},
		"disabled":                       {Authorized: true, WasAuthorized: true},
		"blocked (spam), not authorized": {AdminBlocked: true, BlockReason: "spam", Enabled: true},
		"blocked, revoked, disabled":     {AdminBlocked: true, WasAuthorized: true},
	} {
		if got := state(c); got != want {
			t.Errorf("state(%+v) = %q, want %q", c, got, want)
		}
	}
}
