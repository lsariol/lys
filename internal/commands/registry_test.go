package commands

import (
	"context"
	"testing"
)

type stub struct{ spec Spec }

func (s stub) Spec() Spec                                  { return s.spec }
func (s stub) Run(context.Context, Request) (Reply, error) { return Reply{}, nil }

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stub{Spec{Name: "sleeper", Aliases: []string{"ff"}}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(stub{Spec{Name: "oneup", Patterns: []string{`puzzle #(\d+)`}}}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"sleeper", "FF", "Sleeper"} {
		if c, ok := r.Lookup(name); !ok || c.Spec().Name != "sleeper" {
			t.Fatalf("Lookup(%q) didn't find sleeper", name)
		}
	}
	if _, ok := r.Lookup("nope"); ok {
		t.Fatal("Lookup found a command that doesn't exist")
	}

	c, args, ok := r.Match("you finished puzzle #616 in 3 minutes")
	if !ok || c.Spec().Name != "oneup" || len(args) != 1 || args[0] != "616" {
		t.Fatalf("Match: got %v %v %v", c, args, ok)
	}

	if err := r.Register(stub{Spec{Name: "other", Aliases: []string{"ff"}}}); err == nil {
		t.Fatal("a duplicate alias was accepted")
	}
	if err := r.Register(stub{Spec{Name: "bad", Patterns: []string{"("}}}); err == nil {
		t.Fatal("an invalid pattern was accepted")
	}
	if got := len(r.Commands()); got != 2 {
		t.Fatalf("Commands: got %d, want 2", got)
	}
}
