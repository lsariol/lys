package seventv

import (
	"net/url"
	"regexp"
	"strings"
)

// Permission is what a change needs from an editor on 7TV.
type Permission int

const (
	ManageSets    Permission = iota // add, remove and rename emotes
	ManageProfile                   // switch the active set
)

// Denial is why someone may not make a change.
type Denial int

const (
	Allowed   Denial = iota
	NotEditor        // not on the channel's editor list
	NotYet           // invited, but the invite isn't accepted
	Lacking          // an editor without the permission
)

// EditorByTwitch finds an editor by their Twitch ID; nil if there's none.
func (ch *Channel) EditorByTwitch(twitchID string) *Editor {
	if twitchID == "" {
		return nil
	}
	for i := range ch.Editors {
		if ch.Editors[i].TwitchID == twitchID {
			return &ch.Editors[i]
		}
	}
	return nil
}

// EditorByID finds an editor by their 7TV ID; nil if there's none.
func (ch *Channel) EditorByID(id string) *Editor {
	for i := range ch.Editors {
		if ch.Editors[i].ID == id {
			return &ch.Editors[i]
		}
	}
	return nil
}

// May is whether the editor may make a change that needs p. A nil editor
// is no editor.
func (e *Editor) May(p Permission) Denial {
	switch {
	case e == nil:
		return NotEditor
	case e.State != Accepted:
		return NotYet
	case p == ManageSets && !e.ManageSets, p == ManageProfile && !e.ManageProfile:
		return Lacking
	}
	return Allowed
}

var (
	ulid     = regexp.MustCompile(`^01[0-9A-HJKMNP-TV-Z]{24}$`)
	objectID = regexp.MustCompile(`^[0-9a-f]{24}$`)
	// hosts are 7TV's own addresses an emote link may use.
	hosts = map[string]bool{"7tv.app": true, "www.7tv.app": true, "old.7tv.app": true, "cdn.7tv.app": true}
)

// ParseEmote reads what an add names: an emote ID from a 7TV link or a
// bare ID, or else a name. link is false for a name. A link anywhere but
// 7TV is refused with ok false; links are never fetched.
func ParseEmote(s string) (id string, link, ok bool) {
	if ulid.MatchString(s) || objectID.MatchString(s) {
		return s, true, true
	}
	if !strings.Contains(s, "/") && !strings.Contains(s, "://") {
		return "", false, true
	}
	raw := s
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || !hosts[strings.ToLower(u.Hostname())] {
		return "", true, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || (parts[0] != "emotes" && parts[0] != "emote") {
		return "", true, false
	}
	if ulid.MatchString(parts[1]) || objectID.MatchString(parts[1]) {
		return parts[1], true, true
	}
	return "", true, false
}
