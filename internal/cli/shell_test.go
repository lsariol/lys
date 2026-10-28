package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestSplitLine(t *testing.T) {
	for line, want := range map[string][]string{
		`channel block spambot --reason "spam bot"`: {"channel", "block", "spambot", "--reason", "spam bot"},
		`  help   channel  `:                        {"help", "channel"},
		`channel prefix x ""`:                       {"channel", "prefix", "x", ""},
		``:                                          nil,
	} {
		got, err := splitLine(line)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q, %v; want %q", line, got, err, want)
		}
	}
	if _, err := splitLine(`channel block "x`); err == nil {
		t.Error("an open quote was accepted")
	}
}

func TestComplete(t *testing.T) {
	env, _, _ := testEnv(t)
	for line, want := range map[string]string{
		"chan":           "channel",
		"channel bl":     "channel block ",
		"  migrate s":    "  migrate status ",
		"help channel b": "help channel block ",
		"help mig":       "help migrate ", // both choices continue with a space
	} {
		var listed bytes.Buffer
		got, pos, ok := complete(env, line, len(line), &listed)
		if !ok || got != want || pos != len(want) {
			t.Errorf("%q: got %q at %d (%v), want %q", line, got, pos, ok, want)
		}
	}
	// No match changes nothing; Tab on a common prefix lists the choices.
	if _, _, ok := complete(env, "nothing", 7, &bytes.Buffer{}); ok {
		t.Error("completed a word nothing starts with")
	}
	var listed bytes.Buffer
	if _, _, ok := complete(env, "channel", 7, &listed); ok || !strings.Contains(listed.String(), "channel block") {
		t.Errorf("choices: %v, %q", ok, listed.String())
	}
}

func TestPrompt(t *testing.T) {
	env, _, _ := testEnv(t)
	t.Setenv("APP_ENV", "PROD")
	env.Color = true
	if got := prompt(env); got != "lys ("+red+"prod"+reset+")> " {
		t.Errorf("prod: %q", got)
	}
	t.Setenv("APP_ENV", "")
	if got := prompt(env); got != "lys (dev)> " {
		t.Errorf("dev: %q", got)
	}
}

func TestConfirmInTheShell(t *testing.T) {
	env, _, _ := testEnv(t)
	env.Terminal = true
	var asked string
	env.readLine = func(prompt string) (string, error) {
		asked = prompt
		return "y", nil
	}
	if err := env.Confirm(Input{}, "Block", "Block it?"); err != nil || !strings.Contains(asked, "Block it? (y/N)") {
		t.Fatalf("confirm: %v, asked %q", err, asked)
	}
}
