package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	api "github.com/lsariol/lys/internal/features/seventv"
)

// sevenTV is a 7TV client with the session from Cove.
func sevenTV(ctx context.Context, env *Env) (*api.Client, error) {
	s, err := env.Services(ctx)
	if err != nil {
		return nil, err
	}
	return api.New(s.HTTP, api.Endpoint, api.FromCove(s.Cove)), nil
}

func runSevenTVStatus(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 0 {
		return usageError("7tv status takes no arguments.")
	}
	client, err := sevenTV(ctx, env)
	if err != nil {
		return err
	}
	self, err := client.Self(ctx)
	if err != nil {
		return err
	}
	invites, err := client.Invites(ctx)
	if err != nil {
		return err
	}
	var editor, pending []string
	for _, inv := range invites {
		switch inv.State {
		case api.Accepted:
			editor = append(editor, inv.Login)
		case api.Pending:
			pending = append(pending, inv.Login)
		}
	}

	expires := "unknown"
	exp, known := client.Expiry(ctx)
	if known {
		expires = exp.Local().Format(timeFormat)
	}
	rows := [][2]string{
		{"Account:", fmt.Sprintf("%s (7TV %s)", self.Login, self.ID)},
		{"Session expires:", expires},
		{"Editor in:", list(editor)},
		{"Invites pending:", list(pending)},
	}
	columns(env.Out, rows, 0)
	if known && time.Until(exp) <= api.WarnBefore {
		env.Warn("Needs attention: %s.", api.Renewal(exp.Local(), time.Now()))
	}
	return nil
}

// list is names joined with commas, or "none".
func list(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func runSevenTVCheck(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 1 {
		return usageError("7tv check takes at most one channel.")
	}
	client, err := sevenTV(ctx, env)
	if err != nil {
		return err
	}
	var failures []string
	step := func(name string, err error, format string, a ...any) bool {
		if err != nil {
			env.Error("%s: %v", name, err)
			failures = append(failures, name)
			return false
		}
		env.Success("%s: %s", name, fmt.Sprintf(format, a...))
		return true
	}

	if err := client.Reload(ctx); !step("Session", err, "read from Cove") {
		return errors.New("7TV: the check stopped: there's no session to check with")
	}
	if exp, ok := client.Expiry(ctx); ok && time.Until(exp) <= api.WarnBefore {
		env.Warn("%s.", api.Renewal(exp.Local(), time.Now()))
	}
	self, err := client.Self(ctx)
	step("Account", err, "%s (7TV %s)", self.Login, self.ID)
	invites, err := client.Invites(ctx)
	step("Invites", err, "%d", len(invites))

	twitchID := ""
	if len(in.Args) == 1 {
		if twitchID, err = channelID(ctx, env, in.Args[0]); err != nil {
			return err
		}
	} else {
		for _, inv := range invites {
			if inv.State == api.Accepted && inv.TwitchID != "" {
				twitchID = inv.TwitchID
				break
			}
		}
	}
	if twitchID == "" {
		env.Warn("No channel to check: Lys isn't an editor anywhere yet. Name one: lys 7tv check <channel>.")
	} else {
		checkChannel(ctx, client, env, twitchID, step)
	}
	found, err := client.Search(ctx, "KEKW", true, 3)
	if err == nil && len(found) == 0 {
		err = errors.New("no results for KEKW")
	}
	step("Search", err, "%d results for KEKW", len(found))

	if len(failures) > 0 {
		return fmt.Errorf("7TV: %s failed: the errors above say what 7TV answered", strings.Join(failures, ", "))
	}
	return nil
}

// channelID is a Twitch channel's ID: the argument if it's one, or else
// the channel Lys knows by that name.
func channelID(ctx context.Context, env *Env, arg string) (string, error) {
	if strings.Trim(arg, "0123456789") == "" {
		return arg, nil
	}
	_, conns, err := connectionsStore(ctx, env)
	if err != nil {
		return "", err
	}
	c, err := conns.Find(ctx, "twitch", arg)
	if err != nil {
		return "", fmt.Errorf("find %q: %w", arg, err)
	}
	return c.ExternalID, nil
}

// checkChannel reads a channel's 7TV account and its active set.
func checkChannel(ctx context.Context, client *api.Client, env *Env, twitchID string, step func(string, error, string, ...any) bool) {
	ch, err := client.Channel(ctx, twitchID)
	if err == nil && ch == nil {
		err = fmt.Errorf("Twitch %s has no 7TV account", twitchID)
	}
	if err != nil {
		step("Channel", err, "")
		return
	}
	step("Channel", nil, "%s: %d %s, %d %s", ch.Name, len(ch.Sets), plural(len(ch.Sets), "set", "sets"), len(ch.Editors), plural(len(ch.Editors), "editor", "editors"))
	set, ok := ch.Active()
	if !ok {
		env.Warn("Active set: none")
		return
	}
	c, err := client.Contents(ctx, set.ID)
	if err == nil && c == nil {
		err = fmt.Errorf("set %s isn't there", set.ID)
	}
	if err != nil {
		step("Active set", err, "")
		return
	}
	step("Active set", nil, "%s, %s emotes", set.Name, c.Slots())
}
