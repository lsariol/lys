package settings

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
)

type stubConfig struct {
	set map[string]string
	err error
}

func (c *stubConfig) Prefix(platform, channelID string) string { return core.DefaultPrefix }
func (c *stubConfig) Connection(platform, channelID string) (core.Connection, bool) {
	return core.Connection{}, false
}
func (c *stubConfig) SetPrefix(ctx context.Context, platform, channelID, prefix string) error {
	if c.err != nil {
		return c.err
	}
	c.set[platform+":"+channelID] = prefix
	return nil
}

func request(userID string, args ...string) commands.Request {
	return commands.Request{
		Message: core.Message{Platform: "twitch", Channel: core.Channel{ID: "100"}, User: core.User{ID: userID}},
		Prefix:  "!",
		Args:    args,
	}
}

func TestPrefix(t *testing.T) {
	cfg := &stubConfig{set: map[string]string{}}
	s := New(cfg)
	run := func(req commands.Request) (string, error) {
		r, err := s.Run(context.Background(), req)
		return r.Text, err
	}

	if got, _ := run(request("5", "prefix")); !strings.Contains(got, "'!' prefix") {
		t.Errorf("show: %q", got)
	}
	if got, _ := run(request("5", "prefix", "set", "?")); got != "Only the channel's owner can change the prefix." {
		t.Errorf("a viewer changed the prefix: %q", got)
	}
	if got, _ := run(request("100", "prefix", "set", "???")); !strings.Contains(got, "longer than 2") {
		t.Errorf("long prefix: %q", got)
	}
	if got, _ := run(request("100", "prefix", "set", "?")); !strings.Contains(got, "now set to '?'") || cfg.set["twitch:100"] != "?" {
		t.Errorf("broadcaster: %q, stored %v", got, cfg.set)
	}
	if got, _ := run(request("42217464", "prefix", "set", "$")); !strings.HasPrefix(got, "Only the channel's owner") || cfg.set["twitch:100"] != "?" {
		t.Errorf("the bot's owner changed someone else's prefix: %q, stored %v", got, cfg.set)
	}
	broadcaster := request("other-platform-id", "prefix", "set", "%")
	broadcaster.User.Roles = core.RoleBroadcaster
	if _, err := run(broadcaster); err != nil || cfg.set["twitch:100"] != "%" {
		t.Errorf("broadcaster role: err %v, stored %v", err, cfg.set)
	}
	if got, _ := run(request("100")); got != "" {
		t.Errorf("no arguments replied: %q", got)
	}

	cfg.err = errors.New("database down")
	if got, err := run(request("100", "prefix", "set", "?")); err == nil || !strings.Contains(got, "error storing") {
		t.Errorf("store failure: %q, %v", got, err)
	}
}
