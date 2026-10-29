package seventv

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// emoteFields is what Lys reads of an emote.
const emoteFields = `id defaultName deleted flags { nsfw publicListed private } owner { mainConnection { platformDisplayName } }`

// gqlEmote is an emote as 7TV sends it.
type gqlEmote struct {
	ID          string `json:"id"`
	DefaultName string `json:"defaultName"`
	Deleted     bool   `json:"deleted"`
	Flags       struct {
		NSFW         bool `json:"nsfw"`
		PublicListed bool `json:"publicListed"`
		Private      bool `json:"private"`
	} `json:"flags"`
	Owner *struct {
		MainConnection *struct {
			DisplayName string `json:"platformDisplayName"`
		} `json:"mainConnection"`
	} `json:"owner"`
}

func (g gqlEmote) emote() Emote {
	e := Emote{ID: g.ID, Name: g.DefaultName, NSFW: g.Flags.NSFW, Listed: g.Flags.PublicListed, Private: g.Flags.Private, Deleted: g.Deleted}
	if g.Owner != nil && g.Owner.MainConnection != nil {
		e.Owner = g.Owner.MainConnection.DisplayName
	}
	return e
}

// gqlConnection is a platform account linked to a 7TV user.
type gqlConnection struct {
	Platform string `json:"platform"`
	ID       string `json:"platformId"`
	Username string `json:"platformUsername"`
}

// twitch is the Twitch connection among cs, if there is one.
func twitch(cs []gqlConnection) gqlConnection {
	for _, c := range cs {
		if c.Platform == "TWITCH" {
			return c
		}
	}
	return gqlConnection{}
}

const selfQuery = `query Self { users { me { id connections { platform platformId platformUsername } } } }`

// Self is Lys's own 7TV account, for the current session.
func (c *Client) Self(ctx context.Context) (Account, error) {
	token, err := c.session(ctx)
	if err != nil {
		return Account{}, err
	}
	c.mu.Lock()
	if c.self != nil && c.token == token {
		a := *c.self
		c.mu.Unlock()
		return a, nil
	}
	c.mu.Unlock()

	var data struct {
		Users struct {
			Me *struct {
				ID          string          `json:"id"`
				Connections []gqlConnection `json:"connections"`
			} `json:"me"`
		} `json:"users"`
	}
	if err := c.authed(ctx, "read Lys's account", "Self", selfQuery, nil, &data); err != nil {
		return Account{}, err
	}
	if data.Users.Me == nil {
		return Account{}, ErrSession
	}
	tw := twitch(data.Users.Me.Connections)
	a := Account{ID: data.Users.Me.ID, TwitchID: tw.ID, Login: tw.Username}
	c.mu.Lock()
	if c.token == token {
		c.self = &a
	}
	c.mu.Unlock()
	return a, nil
}

const invitesQuery = `query Invites { users { me { editorFor { userId state user { connections { platform platformId platformUsername } } } } } }`

// Invites is every channel that made Lys an editor, in any state.
func (c *Client) Invites(ctx context.Context) ([]Invite, error) {
	var data struct {
		Users struct {
			Me *struct {
				EditorFor []struct {
					UserID string `json:"userId"`
					State  string `json:"state"`
					User   *struct {
						Connections []gqlConnection `json:"connections"`
					} `json:"user"`
				} `json:"editorFor"`
			} `json:"me"`
		} `json:"users"`
	}
	if err := c.authed(ctx, "list Lys's invites", "Invites", invitesQuery, nil, &data); err != nil {
		return nil, err
	}
	if data.Users.Me == nil {
		return nil, ErrSession
	}
	var out []Invite
	for _, e := range data.Users.Me.EditorFor {
		in := Invite{UserID: e.UserID, State: e.State}
		if e.User != nil {
			tw := twitch(e.User.Connections)
			in.TwitchID, in.Login = tw.ID, tw.Username
		}
		out = append(out, in)
	}
	return out, nil
}

const channelQuery = `query Channel($twitchId: String!) {
	users {
		userByConnection(platform: TWITCH, platformId: $twitchId) {
			id
			mainConnection { platformDisplayName }
			style { activeEmoteSetId }
			emoteSets { id name kind capacity }
			editors {
				editorId
				state
				permissions { emoteSet { manage } user { manageProfile } }
				editor { connections { platform platformId platformUsername } }
			}
		}
	}
}`

// Channel is the 7TV account linked to a Twitch channel, read fresh; nil if
// there's none.
func (c *Client) Channel(ctx context.Context, twitchID string) (*Channel, error) {
	var data struct {
		Users struct {
			User *struct {
				ID             string `json:"id"`
				MainConnection *struct {
					DisplayName string `json:"platformDisplayName"`
				} `json:"mainConnection"`
				Style struct {
					ActiveEmoteSetID *string `json:"activeEmoteSetId"`
				} `json:"style"`
				EmoteSets []struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Kind     string `json:"kind"`
					Capacity *int   `json:"capacity"`
				} `json:"emoteSets"`
				Editors []struct {
					EditorID    string `json:"editorId"`
					State       string `json:"state"`
					Permissions struct {
						EmoteSet struct {
							Manage bool `json:"manage"`
						} `json:"emoteSet"`
						User struct {
							ManageProfile bool `json:"manageProfile"`
						} `json:"user"`
					} `json:"permissions"`
					Editor *struct {
						Connections []gqlConnection `json:"connections"`
					} `json:"editor"`
				} `json:"editors"`
			} `json:"userByConnection"`
		} `json:"users"`
	}
	if err := c.read(ctx, "read the channel", "Channel", channelQuery, map[string]any{"twitchId": twitchID}, &data); err != nil {
		return nil, err
	}
	u := data.Users.User
	if u == nil {
		return nil, nil
	}
	ch := &Channel{ID: u.ID}
	if u.MainConnection != nil {
		ch.Name = u.MainConnection.DisplayName
	}
	if u.Style.ActiveEmoteSetID != nil {
		ch.ActiveSetID = *u.Style.ActiveEmoteSetID
	}
	for _, s := range u.EmoteSets {
		if s.Kind != "NORMAL" {
			continue
		}
		set := Set{ID: s.ID, Name: s.Name}
		if s.Capacity != nil {
			set.Capacity = *s.Capacity
		}
		ch.Sets = append(ch.Sets, set)
	}
	for _, e := range u.Editors {
		ed := Editor{ID: e.EditorID, State: e.State, ManageSets: e.Permissions.EmoteSet.Manage, ManageProfile: e.Permissions.User.ManageProfile}
		if e.Editor != nil {
			tw := twitch(e.Editor.Connections)
			ed.TwitchID, ed.Login = tw.ID, tw.Username
		}
		ch.Editors = append(ch.Editors, ed)
	}
	return ch, nil
}

// perPage is how many of a set's emotes one request asks for; 7TV gives up
// to 1000 at once, the most a set holds today.
const perPage = 1000

const contentsQuery = `query Contents($setId: Id!, $page: Int!, $perPage: Int!) {
	emoteSets {
		emoteSet(id: $setId) {
			id
			name
			capacity
			emotes(page: $page, perPage: $perPage) { totalCount items { alias emote { ` + emoteFields + ` } } }
		}
	}
}`

// Contents is a set with all its emotes, read fresh; nil if there's no such set.
func (c *Client) Contents(ctx context.Context, setID string) (*Contents, error) {
	var out *Contents
	for page := 1; ; page++ {
		var data struct {
			EmoteSets struct {
				EmoteSet *struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Capacity *int   `json:"capacity"`
					Emotes   struct {
						TotalCount int `json:"totalCount"`
						Items      []struct {
							Alias string   `json:"alias"`
							Emote gqlEmote `json:"emote"`
						} `json:"items"`
					} `json:"emotes"`
				} `json:"emoteSet"`
			} `json:"emoteSets"`
		}
		vars := map[string]any{"setId": setID, "page": page, "perPage": perPage}
		if err := c.read(ctx, "read the set", "Contents", contentsQuery, vars, &data); err != nil {
			return nil, err
		}
		s := data.EmoteSets.EmoteSet
		if s == nil {
			return nil, nil
		}
		if out == nil {
			out = &Contents{Set: Set{ID: s.ID, Name: s.Name}}
			if s.Capacity != nil {
				out.Capacity = *s.Capacity
			}
		}
		for _, it := range s.Emotes.Items {
			out.Emotes = append(out.Emotes, SetEmote{Alias: it.Alias, Emote: it.Emote.emote()})
		}
		if len(s.Emotes.Items) == 0 || len(out.Emotes) >= s.Emotes.TotalCount {
			return out, nil
		}
	}
}

const emoteQuery = `query Emote($id: Id!) { emotes { emote(id: $id) { ` + emoteFields + ` } } }`

// Emote is one emote by its ID; nil if there's none.
func (c *Client) Emote(ctx context.Context, id string) (*Emote, error) {
	var data struct {
		Emotes struct {
			Emote *gqlEmote `json:"emote"`
		} `json:"emotes"`
	}
	if err := c.read(ctx, "read the emote", "Emote", emoteQuery, map[string]any{"id": id}, &data); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "Failed to parse") {
			return nil, nil
		}
		return nil, err
	}
	if data.Emotes.Emote == nil {
		return nil, nil
	}
	e := data.Emotes.Emote.emote()
	return &e, nil
}

const searchQuery = `query Search($query: String!, $exact: Boolean!, $perPage: Int!) {
	emotes {
		search(query: $query, sort: { sortBy: TOP_ALL_TIME, order: DESCENDING }, filters: { exactMatch: $exact }, page: 1, perPage: $perPage) {
			items { ` + emoteFields + ` }
		}
	}
}`

// Search is the most used emotes matching a name, most used first, without
// NSFW, unlisted, private or deleted ones. exact asks for that name only,
// ignoring case.
func (c *Client) Search(ctx context.Context, query string, exact bool, n int) ([]Emote, error) {
	var data struct {
		Emotes struct {
			Search struct {
				Items []gqlEmote `json:"items"`
			} `json:"search"`
		} `json:"emotes"`
	}
	vars := map[string]any{"query": query, "exact": exact, "perPage": 3 * n}
	if err := c.read(ctx, "search", "Search", searchQuery, vars, &data); err != nil {
		return nil, err
	}
	var out []Emote
	for _, g := range data.Emotes.Search.Items {
		if e := g.emote(); e.usable() && len(out) < n {
			out = append(out, e)
		}
	}
	return out, nil
}

// Pick is the emote to add for a name: the most used one with exactly that
// name, preferring one that matches its case.
func Pick(found []Emote, name string) (Emote, bool) {
	for _, e := range found {
		if e.Name == name {
			return e, true
		}
	}
	for _, e := range found {
		if equalFold(e.Name, name) {
			return e, true
		}
	}
	return Emote{}, false
}

const addMutation = `mutation AddEmote($setId: Id!, $emote: EmoteSetEmoteId!) { emoteSets { emoteSet(id: $setId) { addEmote(id: $emote) { id } } } }`

// AddEmote adds an emote to a set under alias.
func (c *Client) AddEmote(ctx context.Context, setID, emoteID, alias string) error {
	var data ignored
	vars := map[string]any{"setId": setID, "emote": map[string]any{"emoteId": emoteID, "alias": alias}}
	return c.authed(ctx, "add emote", "AddEmote", addMutation, vars, &data)
}

const removeMutation = `mutation RemoveEmote($setId: Id!, $emote: EmoteSetEmoteId!) { emoteSets { emoteSet(id: $setId) { removeEmote(id: $emote) { id } } } }`

// RemoveEmote takes the emote under alias out of a set. The alias is
// required: without one, 7TV removes the emote under every name.
func (c *Client) RemoveEmote(ctx context.Context, setID, emoteID, alias string) error {
	if alias == "" {
		return fmt.Errorf("7TV: remove emote %s: no name given", emoteID)
	}
	var data ignored
	vars := map[string]any{"setId": setID, "emote": map[string]any{"emoteId": emoteID, "alias": alias}}
	return c.authed(ctx, "remove emote", "RemoveEmote", removeMutation, vars, &data)
}

const renameMutation = `mutation RenameEmote($setId: Id!, $emote: EmoteSetEmoteId!, $alias: String!) { emoteSets { emoteSet(id: $setId) { updateEmoteAlias(id: $emote, alias: $alias) { id } } } }`

// RenameEmote changes the emote's name in a set from alias to newAlias.
func (c *Client) RenameEmote(ctx context.Context, setID, emoteID, alias, newAlias string) error {
	var data ignored
	vars := map[string]any{"setId": setID, "emote": map[string]any{"emoteId": emoteID, "alias": alias}, "alias": newAlias}
	return c.authed(ctx, "rename emote", "RenameEmote", renameMutation, vars, &data)
}

const switchMutation = `mutation SwitchSet($userId: Id!, $setId: Id!) { users { user(id: $userId) { activeEmoteSet(emoteSetId: $setId) { id } } } }`

// SwitchSet makes setID the user's active set.
func (c *Client) SwitchSet(ctx context.Context, userID, setID string) error {
	var data ignored
	return c.authed(ctx, "switch set", "SwitchSet", switchMutation, map[string]any{"userId": userID, "setId": setID}, &data)
}

const acceptMutation = `mutation AcceptInvite($editorId: Id!, $userId: Id!) { userEditors { editor(editorId: $editorId, userId: $userId) { updateState(state: ACCEPT) { state } } } }`

// AcceptInvite accepts the user's invite to make Lys an editor.
func (c *Client) AcceptInvite(ctx context.Context, userID string) error {
	self, err := c.Self(ctx)
	if err != nil {
		return err
	}
	var data ignored
	return c.authed(ctx, "accept invite", "AcceptInvite", acceptMutation, map[string]any{"editorId": self.ID, "userId": userID}, &data)
}

// ignored is an answer Lys doesn't read beyond its errors.
type ignored = map[string]any

func itoa(n int) string { return strconv.Itoa(n) }

func equalFold(a, b string) bool { return strings.EqualFold(a, b) }
