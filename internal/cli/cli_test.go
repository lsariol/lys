package cli

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
)

// testEnv has no terminal and no services: commands that need them fail the test.
func testEnv(t *testing.T) (*Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	return &Env{
		Out: &out, Err: &errOut, In: bufio.NewReader(strings.NewReader("")),
		Services: func(context.Context) (*Services, error) {
			t.Fatal("a command loaded services it doesn't need")
			return nil, nil
		},
	}, &out, &errOut
}

func TestLookup(t *testing.T) {
	for line, want := range map[string]string{
		"channel list":          "channel list",
		"channels":              "channel list",
		"migrate":               "migrate status",
		"migrate up":            "migrate up",
		"channel block x --yes": "channel block",
		"help channel":          "help",
	} {
		cmd, _ := lookup(strings.Fields(line))
		if cmd == nil || cmd.Names[0] != want {
			t.Errorf("%q: got %v, want %q", line, cmd, want)
		}
	}
	if cmd, _ := lookup([]string{"channel"}); cmd != nil {
		t.Errorf("\"channel\" alone matched %q", cmd.Names[0])
	}
}

func TestParse(t *testing.T) {
	cmd, _ := lookup([]string{"channel", "block"})

	in, err := parse(cmd, []string{"--reason", "spam bot", "spambot", "-y"})
	if err != nil || in.Flags["reason"] != "spam bot" || !in.Has("yes") || len(in.Args) != 1 || in.Args[0] != "spambot" {
		t.Fatalf("flags anywhere: %+v, %v", in, err)
	}
	if in, err := parse(cmd, []string{"--reason=x", "--", "--not-a-flag"}); err != nil || in.Flags["reason"] != "x" || in.Args[0] != "--not-a-flag" {
		t.Fatalf("= and --: %+v, %v", in, err)
	}
	for _, args := range [][]string{{"--nope"}, {"--reason"}, {"--yes=1"}, {"-z"}} {
		if _, err := parse(cmd, args); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

func TestRunUsageAndUnknown(t *testing.T) {
	env, _, errOut := testEnv(t)
	if code := Run(context.Background(), env, []string{"channel", "block"}); code != 1 {
		t.Fatalf("missing argument: exit %d", code)
	}
	if got := errOut.String(); !strings.HasPrefix(got, "! ") || !strings.Contains(got, "Usage: lys channel block <channel>") {
		t.Fatalf("missing argument: %q", got)
	}

	errOut.Reset()
	if code := Run(context.Background(), env, []string{"nope"}); code != 1 || !strings.Contains(errOut.String(), `Unknown command "nope"`) {
		t.Fatalf("unknown command: exit %d, %q", code, errOut.String())
	}
}

func TestConfirmWithoutTerminal(t *testing.T) {
	env, _, errOut := testEnv(t)
	if err := env.Confirm(Input{Flags: map[string]string{}}, "Block", "Block it?"); err != errCancelled {
		t.Fatalf("got %v", err)
	}
	if want := "! Block cancelled: no answer to the confirmation. Use --yes to block without asking.\n"; errOut.String() != want {
		t.Fatalf("got %q, want %q", errOut.String(), want)
	}
	if err := env.Confirm(Input{Flags: map[string]string{"yes": "true"}}, "Block", "Block it?"); err != nil {
		t.Fatalf("--yes: %v", err)
	}
}

func TestConfirmOnTerminal(t *testing.T) {
	for answer, wantErr := range map[string]bool{"y\n": false, "yes\n": false, "\n": true, "n\n": true} {
		env, _, _ := testEnv(t)
		env.Terminal = true
		env.In = bufio.NewReader(strings.NewReader(answer))
		if err := env.Confirm(Input{Flags: map[string]string{}}, "Block", "Block it?"); (err != nil) != wantErr {
			t.Errorf("answer %q: err %v", answer, err)
		}
	}
}

func TestHelp(t *testing.T) {
	pages := [][]string{{"help"}}
	for _, c := range table {
		pages = append(pages, append([]string{"help"}, strings.Fields(c.Names[0])...))
	}
	for _, g := range guides {
		pages = append(pages, []string{"help", g.Name})
	}

	for _, args := range pages {
		env, out, errOut := testEnv(t)
		if code := Run(context.Background(), env, args); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errOut.String())
		}
		for _, line := range strings.Split(out.String(), "\n") {
			if len(line) > width {
				t.Errorf("%v: line longer than %d: %q", args, width, line)
			}
		}
	}

	env, _, errOut := testEnv(t)
	if code := Run(context.Background(), env, []string{"help", "nope"}); code != 1 || !strings.Contains(errOut.String(), "no command or guide") {
		t.Fatalf("help nope: exit %d, %q", code, errOut.String())
	}
}

func TestWrap(t *testing.T) {
	if got := wrap("one two three four", 9); got != "one two\nthree\nfour" {
		t.Fatalf("got %q", got)
	}
}
