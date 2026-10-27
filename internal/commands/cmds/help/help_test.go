package help

import (
	"context"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/commands"
)

type stub struct{ spec commands.Spec }

func (s stub) Spec() commands.Spec { return s.spec }
func (s stub) Run(context.Context, commands.Request) (commands.Reply, error) {
	return commands.Reply{}, nil
}

func TestHelp(t *testing.T) {
	reg := commands.NewRegistry()
	h := New(reg)
	for _, c := range []commands.Command{h, stub{commands.Spec{Name: "sleeper", Aliases: []string{"ff"}, Description: "Fantasy football.", Usage: "!sleeper"}}} {
		if err := reg.Register(c); err != nil {
			t.Fatal(err)
		}
	}

	run := func(args ...string) string {
		r, err := h.Run(context.Background(), commands.Request{Prefix: "?", Args: args})
		if err != nil {
			t.Fatal(err)
		}
		return r.Text
	}

	if got := run(); !strings.Contains(got, "?help, ?sleeper") {
		t.Errorf("list: %q", got)
	}
	if got := run("ff"); !strings.Contains(got, "?sleeper: Fantasy football.") || !strings.Contains(got, "also ?ff") {
		t.Errorf("by alias: %q", got)
	}
	if got := run("?sleeper"); !strings.Contains(got, "?sleeper:") {
		t.Errorf("with the prefix: %q", got)
	}
	if got := run("nope"); !strings.Contains(got, "no command called 'nope'") {
		t.Errorf("unknown: %q", got)
	}
}
