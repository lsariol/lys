package seventv

import (
	"context"
	"errors"
	"strings"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/seventv"
)

// sets lists the channel's normal sets, the active one marked.
func (s *SevenTV) sets(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 1 {
		return usage(e, "sets"), nil
	}
	ch, reply, err := s.channel(ctx, e)
	if reply != nil {
		return *reply, err
	}
	if len(ch.Sets) == 0 {
		return say("This channel has no 7TV sets."), nil
	}
	names := make([]string, len(ch.Sets))
	for i, set := range ch.Sets {
		names[i] = set.Name
		if set.ID == ch.ActiveSetID {
			names[i] += " (active)"
		}
	}
	return say("7TV sets: %s", strings.Join(names, ", ")), nil
}

// search shows the most used emotes matching a name, with their links.
func (s *SevenTV) search(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 2 {
		return usage(e, "search <name>"), nil
	}
	found, err := s.client.Search(ctx, e.Args[1], false, 4)
	if err != nil {
		return failed(err), err
	}
	if len(found) == 0 {
		return say("No 7TV emotes match %s.", e.Args[1]), nil
	}
	parts := make([]string, len(found))
	for i, em := range found {
		parts[i] = describe(em) + " " + em.Link()
	}
	return say("%s", strings.Join(parts, " · ")), nil
}

// editors lists the channel's accepted 7TV editors and what each may do.
func (s *SevenTV) editors(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 1 {
		return usage(e, "editors"), nil
	}
	ch, reply, err := s.channel(ctx, e)
	if reply != nil {
		return *reply, err
	}
	var parts []string
	for _, ed := range ch.Editors {
		if ed.State != api.Accepted {
			continue
		}
		name := ed.Login
		if name == "" {
			name = ed.ID
		}
		var may []string
		if ed.ManageSets {
			may = append(may, "emotes")
		}
		if ed.ManageProfile {
			may = append(may, "sets")
		}
		if len(may) == 0 {
			may = append(may, "neither emotes nor sets")
		}
		parts = append(parts, name+" ("+strings.Join(may, ", ")+")")
	}
	if len(parts) == 0 {
		return say("This channel has no 7TV editors."), nil
	}
	return say("7TV editors: %s", strings.Join(parts, ", ")), nil
}

// emote says what an emote in the active set is.
func (s *SevenTV) emote(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 2 {
		return usage(e, "emote <name>"), nil
	}
	ch, reply, err := s.channel(ctx, e)
	if reply != nil {
		return *reply, err
	}
	set, ok := ch.Active()
	if !ok {
		return say("This channel has no active 7TV set."), nil
	}
	c, reply, err := s.contents(ctx, set.ID)
	if reply != nil {
		return *reply, err
	}
	in, ok := c.Find(e.Args[1])
	if !ok {
		return say("There's no emote called %s in the active set.", e.Args[1]), nil
	}
	if in.Alias == in.Emote.Name {
		return say("%s: %s", describe(in.Emote), in.Emote.Link()), nil
	}
	return say("%s is %s: %s", in.Alias, describe(in.Emote), in.Emote.Link()), nil
}

// status is the active set, its slots, and what Lys may do here.
func (s *SevenTV) status(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) != 1 {
		return usage(e, "status"), nil
	}
	ch, reply, err := s.channel(ctx, e)
	if reply != nil {
		return *reply, err
	}
	parts := []string{"No active 7TV set"}
	if set, ok := ch.Active(); ok {
		c, reply, err := s.contents(ctx, set.ID)
		if reply != nil {
			return *reply, err
		}
		parts = []string{"Active set: " + set.Name, c.Slots() + " slots"}
	}

	self, err := s.client.Self(ctx)
	switch {
	case errors.Is(err, api.ErrNoToken), errors.Is(err, api.ErrSession):
		parts = append(parts, "Lys's 7TV login needs renewing")
		err = nil
	case err != nil:
		parts = append(parts, "couldn't check Lys's permissions")
	default:
		parts = append(parts, lysMay(ch.EditorByID(self.ID)))
	}
	return say("%s", strings.Join(parts, " · ")), err
}

// lysMay says what Lys's editor record lets it do.
func lysMay(ed *api.Editor) string {
	switch {
	case ed == nil:
		return "Lys isn't a 7TV editor here"
	case ed.State != api.Accepted:
		return "Lys's editor invite is waiting to be accepted"
	case ed.ManageSets && ed.ManageProfile:
		return "Lys can change emotes and switch sets"
	case ed.ManageSets:
		return "Lys can change emotes, not switch sets"
	case ed.ManageProfile:
		return "Lys can switch sets, not change emotes"
	}
	return "Lys is an editor without Manage emote sets"
}

// describe is an emote's name and author, such as "KEKW by WH0LUV".
func describe(em api.Emote) string {
	if em.Owner == "" {
		return em.Name
	}
	return em.Name + " by " + em.Owner
}
