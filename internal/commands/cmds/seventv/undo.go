package seventv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/seventv"
)

// undoWindow is how far back !7tv undo reaches.
const undoWindow = 30 * time.Minute

// Kinds of change.
const (
	added = iota
	removed
	renamed
	switched
)

// change is a channel's last change through Lys, for !7tv undo.
type change struct {
	kind      int
	by        string // who made it, by display name
	at        time.Time
	setID     string // the set changed, or switched to
	emoteID   string
	alias     string // the emote's name there (before a rename), or the set switched to
	newAlias  string // a rename's new name
	prevSetID string // the set active before a switch
	prevName  string
}

func (c change) permission() api.Permission {
	if c.kind == switched {
		return api.ManageProfile
	}
	return api.ManageSets
}

// String is what the change was, such as "add emote haha".
func (c change) String() string {
	switch c.kind {
	case added:
		return "add emote " + c.alias
	case removed:
		return "remove emote " + c.alias
	case renamed:
		return "rename emote " + c.alias + " to " + c.newAlias
	}
	return "set switch to " + c.alias
}

// undoLog keeps each channel's last change, in memory: a restart forgets it.
type undoLog struct {
	mu   sync.Mutex
	last map[string]change // by Twitch channel ID
}

func (u *undoLog) record(channelID string, c change) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.last == nil {
		u.last = make(map[string]change)
	}
	u.last[channelID] = c
}

// get is the channel's last change; false if there's none.
func (u *undoLog) get(channelID string) (change, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	c, ok := u.last[channelID]
	return c, ok
}

// clear forgets the channel's last change, if it's still c.
func (u *undoLog) clear(channelID string, c change) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.last[channelID] == c {
		delete(u.last, channelID)
	}
}

// undoLast reverts the channel's last change, if it's recent, for someone
// allowed to make that change now.
func (s *SevenTV) undoLast(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 1 {
		return usage(e, "undo"), nil
	}
	last, ok := s.undo.get(e.Channel.ID)
	if !ok || s.now().Sub(last.at) > undoWindow {
		return say("There's nothing to undo from the last 30 minutes."), nil
	}
	ch, reply, err := s.channel(ctx, e)
	if reply != nil {
		return *reply, err
	}
	if reply, err := s.authorize(ctx, e, ch, last.permission()); reply != nil {
		return *reply, err
	}
	done := say("Undo %s by @%s", last, last.by)

	if last.kind == switched {
		if ch.ActiveSetID != last.setID {
			s.undo.clear(e.Channel.ID, last)
			return say("The active set has changed since, so there's nothing to undo."), nil
		}
		var prev *api.Set
		for i := range ch.Sets {
			if ch.Sets[i].ID == last.prevSetID {
				prev = &ch.Sets[i]
			}
		}
		if prev == nil {
			s.undo.clear(e.Channel.ID, last)
			return say("%s isn't one of the channel's sets anymore, so the switch can't be undone.", last.prevName), nil
		}
		if r, err := s.switchTo(ctx, e, ch, *prev, true); r.Text != "" {
			return r, err
		}
		s.undo.clear(e.Channel.ID, last)
		return done, nil
	}

	owned := false
	for _, set := range ch.Sets {
		owned = owned || set.ID == last.setID
	}
	if !owned {
		s.undo.clear(e.Channel.ID, last)
		return say("That set isn't one of the channel's sets anymore, so there's nothing to undo."), nil
	}
	defer s.locks.lock(last.setID)()
	c, reply, err := s.contents(ctx, last.setID)
	if reply != nil {
		return *reply, err
	}

	var action string
	var check func(*api.Contents) bool
	switch last.kind {
	case added:
		in, ok := c.Has(last.emoteID)
		if !ok || in.Alias != last.alias {
			s.undo.clear(e.Channel.ID, last)
			return say("%s isn't in the set anymore, so there's nothing to undo.", last.alias), nil
		}
		action = fmt.Sprintf("undo: remove emote %s %q from set %s", last.emoteID, last.alias, last.setID)
		err = s.client.RemoveEmote(ctx, last.setID, last.emoteID, last.alias)
		check = func(c *api.Contents) bool { return !c.Taken(last.alias) }
	case removed:
		if c.Taken(last.alias) {
			s.undo.clear(e.Channel.ID, last)
			return say("There's an emote called %s in the set again, so the remove can't be undone.", last.alias), nil
		}
		if c.Capacity > 0 && len(c.Emotes) >= c.Capacity {
			return say("The set is full (%s slots), so %s can't go back in.", c.Slots(), last.alias), nil
		}
		action = fmt.Sprintf("undo: add emote %s as %q to set %s", last.emoteID, last.alias, last.setID)
		err = s.client.AddEmote(ctx, last.setID, last.emoteID, last.alias)
		check = func(c *api.Contents) bool { in, ok := c.Has(last.emoteID); return ok && in.Alias == last.alias }
	case renamed:
		in, ok := c.Has(last.emoteID)
		if !ok || in.Alias != last.newAlias {
			s.undo.clear(e.Channel.ID, last)
			return say("%s isn't in the set under that name anymore, so there's nothing to undo.", last.newAlias), nil
		}
		if c.Taken(last.alias) {
			s.undo.clear(e.Channel.ID, last)
			return say("There's an emote called %s in the set again, so the rename can't be undone.", last.alias), nil
		}
		action = fmt.Sprintf("undo: rename emote %s %q to %q in set %s", last.emoteID, last.newAlias, last.alias, last.setID)
		err = s.client.RenameEmote(ctx, last.setID, last.emoteID, last.newAlias, last.alias)
		check = func(c *api.Contents) bool { in, ok := c.Has(last.emoteID); return ok && in.Alias == last.alias }
	}
	if unknown(err) && s.landed(ctx, last.setID, check) {
		err = nil
	}
	logChange(e, ch, action, err)
	if err != nil {
		return failed(err), err
	}
	s.undo.clear(e.Channel.ID, last)
	return done, nil
}
