package sleeper

import (
	"strings"
	"testing"
)

func TestTrades(t *testing.T) {
	if got := trades([]string{"trades"}).Text; !strings.Contains(got, "Usage") {
		t.Fatalf("no name: got %q, want the usage", got)
	}
	if got := trades([]string{"trades", "alex"}).Text; !strings.Contains(got, "alex") {
		t.Fatalf("with a name: got %q, want it to mention the name", got)
	}
}
