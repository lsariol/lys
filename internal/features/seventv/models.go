package seventv

// Account is a 7TV account as Lys sees it.
type Account struct {
	ID       string // 7TV user ID
	TwitchID string // "" if it has no Twitch connection
	Login    string // its Twitch login
}

// Invite is one of Lys's editor records: a channel that invited it.
type Invite struct {
	UserID   string // the inviting 7TV user
	TwitchID string // their Twitch ID; "" if they have none
	Login    string
	State    string // PENDING, ACCEPTED or REJECTED
}

// Editor states.
const (
	Pending  = "PENDING"
	Accepted = "ACCEPTED"
)

// Channel is a streamer's 7TV account: its sets and editors.
type Channel struct {
	ID          string // 7TV user ID
	Name        string // display name
	ActiveSetID string // "" if no set is active
	Sets        []Set  // normal sets only, the ones a channel can make active
	Editors     []Editor
}

// Set is one emote set.
type Set struct {
	ID       string
	Name     string
	Capacity int // 0 if 7TV gives none
}

// Editor is someone on a channel's 7TV editor list.
type Editor struct {
	ID            string // 7TV user ID
	State         string
	ManageSets    bool // may add, remove and rename emotes
	ManageProfile bool // may switch the active set
	TwitchID      string
	Login         string
}

// Contents is a set and its emotes.
type Contents struct {
	Set
	Emotes []SetEmote
}

// SetEmote is an emote in a set, under its name there.
type SetEmote struct {
	Alias string
	Emote Emote
}

// Emote is a 7TV emote.
type Emote struct {
	ID      string
	Name    string // its own name
	Owner   string // the uploader's display name
	NSFW    bool
	Listed  bool // approved for public listing
	Private bool
	Deleted bool
}

// Link is where the emote's page is, short enough for chat.
func (e Emote) Link() string { return "7tv.app/emotes/" + e.ID }

// usable reports whether the emote may be picked from a search.
func (e Emote) usable() bool { return !e.Deleted && !e.NSFW && !e.Private && e.Listed }

// Active is the channel's active set; false if none is active or it isn't
// one of the channel's normal sets.
func (ch *Channel) Active() (Set, bool) {
	for _, s := range ch.Sets {
		if s.ID == ch.ActiveSetID {
			return s, true
		}
	}
	return Set{}, false
}

// Find is the emote with this name in the set: an exact match, or else the
// only one that matches ignoring case.
func (c *Contents) Find(alias string) (SetEmote, bool) {
	var folded []SetEmote
	for _, e := range c.Emotes {
		if e.Alias == alias {
			return e, true
		}
		if equalFold(e.Alias, alias) {
			folded = append(folded, e)
		}
	}
	if len(folded) == 1 {
		return folded[0], true
	}
	return SetEmote{}, false
}

// Taken reports whether a name is in use in the set; 7TV's names are case-sensitive.
func (c *Contents) Taken(alias string) bool {
	for _, e := range c.Emotes {
		if e.Alias == alias {
			return true
		}
	}
	return false
}

// Has finds the emote in the set by its ID, under any name.
func (c *Contents) Has(emoteID string) (SetEmote, bool) {
	for _, e := range c.Emotes {
		if e.Emote.ID == emoteID {
			return e, true
		}
	}
	return SetEmote{}, false
}

// Slots is "271/600", or just the count when the set has no capacity.
func (c *Contents) Slots() string {
	if c.Capacity == 0 {
		return itoa(len(c.Emotes))
	}
	return itoa(len(c.Emotes)) + "/" + itoa(c.Capacity)
}
