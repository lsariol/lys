package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
)

// fakeAdapter feeds messages into the router and records what it sends.
type fakeAdapter struct {
	in   chan core.Message
	sent chan core.Outbound
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{in: make(chan core.Message, 16), sent: make(chan core.Outbound, 16)}
}

func (f *fakeAdapter) Platform() string { return "fake" }
func (f *fakeAdapter) Healthy() bool    { return true }

func (f *fakeAdapter) Run(ctx context.Context, in chan<- core.Message) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m := <-f.in:
			in <- m
		}
	}
}

func (f *fakeAdapter) Send(ctx context.Context, out core.Outbound) error {
	f.sent <- out
	return nil
}

// say sends text as a chat message and returns the reply, or "" if none
// arrives within wait.
func (f *fakeAdapter) say(t *testing.T, channelID, text string, wait time.Duration) string {
	t.Helper()
	f.in <- core.Message{Platform: "fake", Channel: core.Channel{ID: channelID, Name: "chan" + channelID}, User: core.User{ID: "u1", Name: "viewer"}, Text: text}
	select {
	case out := <-f.sent:
		if out.Channel.ID != channelID || out.Platform != "fake" {
			t.Fatalf("reply went to %s %q, want fake %q", out.Platform, out.Channel.ID, channelID)
		}
		return out.Text
	case <-time.After(wait):
		return ""
	}
}

// fakeConfig gives channel "2" the prefix "?"; every other channel uses "!".
type fakeConfig struct{}

func (fakeConfig) Prefix(platform, channelID string) string {
	if channelID == "2" {
		return "?"
	}
	return core.DefaultPrefix
}
func (fakeConfig) SetPrefix(context.Context, string, string, string) error { return nil }
func (fakeConfig) Connection(platform, channelID string) (core.Connection, bool) {
	return core.Connection{ID: 7, CommunityID: 3}, true
}

// fakeIdentities gives user u1 platform user 11 and account 5; anyone else fails.
type fakeIdentities struct{}

func (fakeIdentities) Resolve(ctx context.Context, platform string, u core.User) (core.Identity, error) {
	if u.ID == "u1" {
		return core.Identity{PlatformUserID: 11, AccountID: 5}, nil
	}
	return core.Identity{}, errors.New("unknown user")
}

// cmd is a command built from a function.
type cmd struct {
	spec commands.Spec
	run  func(ctx context.Context, req commands.Request) (commands.Reply, error)
}

func (c cmd) Spec() commands.Spec { return c.spec }
func (c cmd) Run(ctx context.Context, req commands.Request) (commands.Reply, error) {
	return c.run(ctx, req)
}

func start(t *testing.T, cmds ...commands.Command) *fakeAdapter {
	t.Helper()
	reg := commands.NewRegistry()
	for _, c := range cmds {
		if err := reg.Register(c); err != nil {
			t.Fatal(err)
		}
	}
	r := New(reg, fakeConfig{}, fakeIdentities{}, 4)
	a := newFakeAdapter()
	r.Add(a)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	return a
}

func echo() commands.Command {
	return cmd{
		spec: commands.Spec{Name: "echo", Aliases: []string{"e"}},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			return commands.Reply{Text: req.Command + ":" + strings.Join(req.Args, ",") + ":" + req.Prefix}, nil
		},
	}
}

const wait = time.Second

func TestPrefixAndAliases(t *testing.T) {
	a := start(t, echo())

	for text, want := range map[string]string{
		"!echo a b":     "echo:a,b:!",
		"  !E a  ":      "echo:a:!",
		"! echo spaced": "echo:spaced:!",
	} {
		if got := a.say(t, "1", text, wait); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestChannelPrefix(t *testing.T) {
	a := start(t, echo())

	if got := a.say(t, "2", "?echo x", wait); got != "echo:x:?" {
		t.Fatalf("custom prefix: got %q", got)
	}
	if got := a.say(t, "2", "!echo x", 100*time.Millisecond); got != "" {
		t.Fatalf("the default prefix worked in a channel with its own: %q", got)
	}
}

func TestNoCommand(t *testing.T) {
	a := start(t, echo())

	for _, text := range []string{"hello", "!", "!unknown", "echo"} {
		if got := a.say(t, "1", text, 100*time.Millisecond); got != "" {
			t.Errorf("%q got a reply: %q", text, got)
		}
	}
}

func TestPattern(t *testing.T) {
	puzzle := cmd{
		spec: commands.Spec{Name: "puzzle", Patterns: []string{`finished puzzle #(\d+)`}},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			if !req.Pattern {
				return commands.Reply{Text: "called by name"}, nil
			}
			return commands.Reply{Text: "puzzle " + req.Args[0]}, nil
		},
	}
	a := start(t, echo(), puzzle)

	if got := a.say(t, "1", "Congratulations, you finished puzzle #616 in 3 minutes", wait); got != "puzzle 616" {
		t.Fatalf("pattern: got %q", got)
	}
	if got := a.say(t, "1", "!puzzle", wait); got != "called by name" {
		t.Fatalf("by name: got %q", got)
	}
	// A prefixed message never falls through to the patterns.
	if got := a.say(t, "1", "!nope finished puzzle #1", 100*time.Millisecond); got != "" {
		t.Fatalf("prefixed message matched a pattern: %q", got)
	}
}

func TestPanicAndError(t *testing.T) {
	boom := cmd{
		spec: commands.Spec{Name: "boom"},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			var args []string
			_ = args[1]
			return commands.Reply{}, nil
		},
	}
	fails := cmd{
		spec: commands.Spec{Name: "fails"},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			return commands.Reply{Text: "it failed"}, errors.New("broken")
		},
	}
	silent := cmd{
		spec: commands.Spec{Name: "silent"},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			return commands.Reply{Text: "  "}, nil
		},
	}
	a := start(t, boom, fails, silent, echo())

	if got := a.say(t, "1", "!boom", wait); got != panicReply {
		t.Fatalf("panic: got %q", got)
	}
	if got := a.say(t, "1", "!fails", wait); got != "it failed" {
		t.Fatalf("error with text: got %q", got)
	}
	if got := a.say(t, "1", "!silent", 100*time.Millisecond); got != "" {
		t.Fatalf("blank reply was sent: %q", got)
	}
	if got := a.say(t, "1", "!echo still", wait); got != "echo:still:!" {
		t.Fatalf("router stopped after a panic: got %q", got)
	}
}

func TestTimeout(t *testing.T) {
	slow := cmd{
		spec: commands.Spec{Name: "slow", Timeout: 50 * time.Millisecond},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			<-ctx.Done()
			return commands.Reply{Text: "timed out"}, ctx.Err()
		},
	}
	a := start(t, slow, echo())

	if got := a.say(t, "1", "!slow", wait); got != "timed out" {
		t.Fatalf("timeout: got %q", got)
	}
}

func TestSlowCommandDoesNotBlockOthers(t *testing.T) {
	release := make(chan struct{})
	stuck := cmd{
		spec: commands.Spec{Name: "stuck", Timeout: time.Minute},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			<-release
			return commands.Reply{}, nil
		},
	}
	a := start(t, stuck, echo())
	defer close(release)

	a.in <- core.Message{Platform: "fake", Channel: core.Channel{ID: "1"}, Text: "!stuck"}
	if got := a.say(t, "1", "!echo free", wait); got != "echo:free:!" {
		t.Fatalf("a stuck command blocked the others: got %q", got)
	}
}

func TestIdentity(t *testing.T) {
	who := cmd{
		spec: commands.Spec{Name: "who"},
		run: func(ctx context.Context, req commands.Request) (commands.Reply, error) {
			return commands.Reply{Text: fmt.Sprintf("%d/%d %d/%d", req.Identity.PlatformUserID, req.Identity.AccountID, req.Connection.ID, req.Connection.CommunityID)}, nil
		},
	}
	a := start(t, who)

	if got := a.say(t, "1", "!who", wait); got != "11/5 7/3" {
		t.Fatalf("identity: got %q", got)
	}

	// A failed lookup still runs the command, without an identity.
	a.in <- core.Message{Platform: "fake", Channel: core.Channel{ID: "1"}, User: core.User{ID: "u2"}, Text: "!who"}
	select {
	case out := <-a.sent:
		if out.Text != "0/0 7/3" {
			t.Fatalf("failed lookup: got %q", out.Text)
		}
	case <-time.After(wait):
		t.Fatal("a failed lookup stopped the command")
	}
}

func TestPublish(t *testing.T) {
	reg := commands.NewRegistry()
	r := New(reg, fakeConfig{}, fakeIdentities{}, 1)
	r.Add(newFakeAdapter())

	if err := r.Publish(context.Background(), core.Outbound{Platform: "nope", Text: "x"}); err == nil {
		t.Fatal("publishing to an unknown platform succeeded")
	}
	if err := r.Publish(context.Background(), core.Outbound{Platform: "fake", Text: ""}); err != nil {
		t.Fatalf("an empty message: %v", err)
	}
	if h := r.Health(); !h["fake"] {
		t.Fatalf("Health: %v", h)
	}
}

func TestAdapterErrorStopsRun(t *testing.T) {
	r := New(commands.NewRegistry(), fakeConfig{}, fakeIdentities{}, 1)
	r.Add(failing{})
	if err := r.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "lost") {
		t.Fatalf("Run: got %v, want the adapter's error", err)
	}
}

type failing struct{}

func (failing) Platform() string                               { return "failing" }
func (failing) Healthy() bool                                  { return false }
func (failing) Run(context.Context, chan<- core.Message) error { return errors.New("connection lost") }
func (failing) Send(context.Context, core.Outbound) error      { return nil }
