package seventv

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/seventv"
)

// target is the channel's active set, for a change to it.
func (s *SevenTV) target(ctx context.Context, e commands.Request) (*api.Channel, api.Set, *commands.Reply, error) {
	ch, reply, err := s.channel(ctx, e)
	if reply != nil {
		return nil, api.Set{}, reply, err
	}
	if reply, err := s.authorize(ctx, e, ch, api.ManageSets); reply != nil {
		return nil, api.Set{}, reply, err
	}
	set, ok := ch.Active()
	if !ok {
		r := say("This channel has no active 7TV set.")
		return nil, api.Set{}, &r, nil
	}
	return ch, set, nil, nil
}

// contents reads the set; reply is set when it can't.
func (s *SevenTV) contents(ctx context.Context, setID string) (*api.Contents, *commands.Reply, error) {
	c, err := s.client.Contents(ctx, setID)
	if err != nil {
		r := failed(err)
		return nil, &r, err
	}
	if c == nil {
		r := say("7TV couldn't find the set, try again in a moment.")
		return nil, &r, fmt.Errorf("7TV: set %s is gone", setID)
	}
	return c, nil, nil
}

// landed checks, after a change whose answer was lost, whether it took effect.
func (s *SevenTV) landed(ctx context.Context, setID string, check func(*api.Contents) bool) bool {
	ctx, cancel := settled(ctx)
	defer cancel()
	c, err := s.client.Contents(ctx, setID)
	return err == nil && c != nil && check(c)
}

// add puts an emote in the active set: a link adds that emote, a name the
// most used emote with exactly that name.
func (s *SevenTV) add(ctx context.Context, e commands.Request) (commands.Reply, error) {
	args := e.Args[1:]
	if len(args) < 1 || len(args) > 2 {
		return usage(e, "add <name or link> [local name]"), nil
	}
	id, link, ok := api.ParseEmote(args[0])
	if !ok {
		return say("Only 7TV emote links work, like 7tv.app/emotes/<ID>."), nil
	}

	ch, set, reply, err := s.target(ctx, e)
	if reply != nil {
		return *reply, err
	}
	defer s.locks.lock(set.ID)()

	var emote api.Emote
	if link {
		found, err := s.client.Emote(ctx, id)
		if err != nil {
			return failed(err), err
		}
		if found == nil || found.Deleted {
			return say("There's no 7TV emote %s.", id), nil
		}
		if found.Private {
			return say("That emote is private on 7TV."), nil
		}
		emote = *found
	} else {
		found, err := s.client.Search(ctx, args[0], true, 5)
		if err != nil {
			return failed(err), err
		}
		if emote, ok = api.Pick(found, args[0]); !ok {
			return say("No 7TV emote is called %s. Try %s7tv search %s.", args[0], e.Prefix, args[0]), nil
		}
	}
	alias := emote.Name
	if len(args) == 2 {
		alias = args[1]
	}

	c, reply, err := s.contents(ctx, set.ID)
	if reply != nil {
		return *reply, err
	}
	if in, ok := c.Has(emote.ID); ok {
		return say("%s is already in the set, as %s.", emote.Name, in.Alias), nil
	}
	if c.Taken(alias) {
		return say("There's already an emote called %s in the set.", alias), nil
	}
	if c.Capacity > 0 && len(c.Emotes) >= c.Capacity {
		return say("The set is full (%s slots).", c.Slots()), nil
	}

	action := fmt.Sprintf("add emote %s as %q to set %s", emote.ID, alias, set.ID)
	err = s.client.AddEmote(ctx, set.ID, emote.ID, alias)
	if unknown(err) && s.landed(ctx, set.ID, func(c *api.Contents) bool { in, ok := c.Has(emote.ID); return ok && in.Alias == alias }) {
		err = nil
	}
	logChange(e, ch, action, err)
	if err != nil {
		return failed(err), err
	}
	s.undo.record(e.Channel.ID, change{kind: added, by: e.User.Name, at: s.now(), setID: set.ID, emoteID: emote.ID, alias: alias})

	c.Emotes = append(c.Emotes, api.SetEmote{Alias: alias, Emote: emote})
	name := emote.Name
	if alias != emote.Name {
		name = emote.Name + " as " + alias
	}
	return say("Added %s (%s) · %s slots", name, emote.Link(), c.Slots()), nil
}

// remove takes one emote out of the active set, by its name there.
func (s *SevenTV) remove(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 2 {
		return usage(e, "remove <name>"), nil
	}
	ch, set, reply, err := s.target(ctx, e)
	if reply != nil {
		return *reply, err
	}
	defer s.locks.lock(set.ID)()

	c, reply, err := s.contents(ctx, set.ID)
	if reply != nil {
		return *reply, err
	}
	in, ok := c.Find(e.Args[1])
	if !ok {
		return say("There's no emote called %s in the set.", e.Args[1]), nil
	}

	action := fmt.Sprintf("remove emote %s %q from set %s", in.Emote.ID, in.Alias, set.ID)
	err = s.client.RemoveEmote(ctx, set.ID, in.Emote.ID, in.Alias)
	if unknown(err) && s.landed(ctx, set.ID, func(c *api.Contents) bool { return !c.Taken(in.Alias) }) {
		err = nil
	}
	logChange(e, ch, action, err)
	if err != nil {
		return failed(err), err
	}
	s.undo.record(e.Channel.ID, change{kind: removed, by: e.User.Name, at: s.now(), setID: set.ID, emoteID: in.Emote.ID, alias: in.Alias})

	c.Emotes = slices.DeleteFunc(c.Emotes, func(x api.SetEmote) bool { return x.Alias == in.Alias })
	return say("Removed %s · %s slots", in.Alias, c.Slots()), nil
}

// rename changes an emote's name in the active set; its own name stays.
func (s *SevenTV) rename(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 3 {
		return usage(e, "rename <name> <new name>"), nil
	}
	ch, set, reply, err := s.target(ctx, e)
	if reply != nil {
		return *reply, err
	}
	defer s.locks.lock(set.ID)()

	c, reply, err := s.contents(ctx, set.ID)
	if reply != nil {
		return *reply, err
	}
	in, ok := c.Find(e.Args[1])
	if !ok {
		return say("There's no emote called %s in the set.", e.Args[1]), nil
	}
	to := e.Args[2]
	if to == in.Alias {
		return say("It's already called %s.", to), nil
	}
	if c.Taken(to) {
		return say("There's already an emote called %s in the set.", to), nil
	}

	action := fmt.Sprintf("rename emote %s %q to %q in set %s", in.Emote.ID, in.Alias, to, set.ID)
	err = s.client.RenameEmote(ctx, set.ID, in.Emote.ID, in.Alias, to)
	if unknown(err) && s.landed(ctx, set.ID, func(c *api.Contents) bool { got, ok := c.Find(to); return ok && got.Emote.ID == in.Emote.ID }) {
		err = nil
	}
	logChange(e, ch, action, err)
	if err != nil {
		return failed(err), err
	}
	s.undo.record(e.Channel.ID, change{kind: renamed, by: e.User.Name, at: s.now(), setID: set.ID, emoteID: in.Emote.ID, alias: in.Alias, newAlias: to})
	return say("Renamed %s to %s", in.Alias, to), nil
}

// switchSet makes another of the channel's sets the active one.
func (s *SevenTV) switchSet(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) < 2 {
		return usage(e, "set <set name>"), nil
	}
	name := strings.Join(e.Args[1:], " ")
	ch, reply, err := s.channel(ctx, e)
	if reply != nil {
		return *reply, err
	}
	if reply, err := s.authorize(ctx, e, ch, api.ManageProfile); reply != nil {
		return *reply, err
	}

	var matches []api.Set
	for _, set := range ch.Sets {
		if set.ID == name {
			matches = []api.Set{set}
			break
		}
		if strings.EqualFold(set.Name, name) {
			matches = append(matches, set)
		}
	}
	switch {
	case len(matches) == 0:
		return say("There's no set called %s. The sets: %s.", name, setNames(ch)), nil
	case len(matches) > 1:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return say("Several sets are called %s; use one's ID instead: %s.", name, strings.Join(ids, ", ")), nil
	}
	to := matches[0]
	if to.ID == ch.ActiveSetID {
		return say("%s is already the active set.", to.Name), nil
	}
	return s.switchTo(ctx, e, ch, to, false)
}

// switchTo switches the channel to set, for a set switch or its undo.
func (s *SevenTV) switchTo(ctx context.Context, e commands.Request, ch *api.Channel, to api.Set, undo bool) (commands.Reply, error) {
	defer s.locks.lock("user:" + ch.ID)()
	from := ch.ActiveSetID
	action := fmt.Sprintf("switch set from %s to %s", from, to.ID)
	err := s.client.SwitchSet(ctx, ch.ID, to.ID)
	if unknown(err) {
		ctx, cancel := settled(ctx)
		now, cerr := s.client.Channel(ctx, e.Channel.ID)
		cancel()
		if cerr == nil && now != nil && now.ActiveSetID == to.ID {
			err = nil
		}
	}
	if undo {
		action = "undo: " + action
	}
	logChange(e, ch, action, err)
	if err != nil {
		return failed(err), err
	}
	if undo {
		return commands.Reply{}, nil
	}
	fromName := from
	for _, set := range ch.Sets {
		if set.ID == from {
			fromName = set.Name
		}
	}
	s.undo.record(e.Channel.ID, change{kind: switched, by: e.User.Name, at: s.now(), setID: to.ID, alias: to.Name, prevSetID: from, prevName: fromName})
	return say("Switched the active set to %s", to.Name), nil
}

func setNames(ch *api.Channel) string {
	names := make([]string, len(ch.Sets))
	for i, set := range ch.Sets {
		names[i] = set.Name
	}
	return strings.Join(names, ", ")
}
