package sleeper

import (
	"context"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/commands"
)

// Wrong arguments are answered before anything is looked up, so these run
// without a database or Sleeper.
func TestArguments(t *testing.T) {
	sl := &Sleeper{}
	for _, args := range [][]string{
		{"team", "@a", "@b"},
		{"matchup", "@a", "0"},
		{"matchup", "@a", "19"},
		{"matchup", "abc"},
		{"matchup", "@a", "3", "extra"},
		{"roster", "@a", "2"},
		{"highscore", "0"},
		{"highscore", "abc"},
		{"recap", "1", "2"},
		{"trades", "@a", "@b"},
		{"link"},
		{"link", "league"},
		{"link", "user"},
		{"link", "a", "b"},
		{"unlink", "a", "b"},
		{"whohas"},
	} {
		reply, err := sl.Run(context.Background(), commands.Request{Args: args, Prefix: "!"})
		if err != nil || !strings.HasPrefix(reply.Text, "🏈 Usage: !ff ") {
			t.Errorf("%v: %q, %v", args, reply.Text, err)
		}
	}

	reply, _ := sl.Run(context.Background(), commands.Request{Args: []string{"nope"}, Prefix: "?"})
	if !strings.HasPrefix(reply.Text, "🏈 There's no ?ff nope. Try: team, matchup") {
		t.Errorf("unknown: %q", reply.Text)
	}
	reply, _ = sl.Run(context.Background(), commands.Request{Args: []string{"HELP"}, Prefix: "!"})
	if !strings.Contains(reply.Text, "!ff team, matchup") || !strings.Contains(reply.Text, "!ff team @name") {
		t.Errorf("help: %q", reply.Text)
	}
}

func TestTargetUser(t *testing.T) {
	e := commands.Request{}
	e.User.Name = "Lu"
	for _, tc := range []struct {
		args []string
		name string
		self bool
	}{
		{nil, "Lu", true},
		{[]string{"3"}, "Lu", true},
		{[]string{"@kyle"}, "kyle", false},
		{[]string{"3", "@LU"}, "LU", true},
		{[]string{"@"}, "Lu", true},
	} {
		if name, self := targetUser(e, tc.args); name != tc.name || self != tc.self {
			t.Errorf("%v: %q, %v", tc.args, name, self)
		}
	}
}
