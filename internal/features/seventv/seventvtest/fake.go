// Package seventvtest is a fake 7TV GraphQL API for tests: it answers the
// operations Lys sends, from state the test sets up.
package seventvtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	api "github.com/lsariol/lys/internal/features/seventv"
)

// Fake is the fake API's state. Lock Mu to change it while the server runs.
type Fake struct {
	Mu       sync.Mutex
	Token    string // the session it accepts
	Self     api.Account
	Channels map[string]*Channel // by Twitch ID
	Catalog  []api.Emote         // every emote, most used first
	Invites  []api.Invite
	Calls    map[string]int  // requests by operation name
	Lose     map[string]bool // operations whose answer is lost after they take effect

	URL string
}

// Channel is a streamer's 7TV account.
type Channel struct {
	ID      string
	Name    string
	Active  string
	Sets    []*Set
	Editors []api.Editor
}

// Set is an emote set and its emotes.
type Set struct {
	ID       string
	Name     string
	Capacity int
	Emotes   []api.SetEmote
}

// New starts a fake API, closed when the test ends.
func New(t *testing.T) *Fake {
	f := &Fake{Channels: map[string]*Channel{}, Calls: map[string]int{}, Lose: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// Client is a client of the fake, with its session.
func (f *Fake) Client() *api.Client {
	return api.New(http.DefaultClient, f.URL, func(context.Context) (string, error) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		return f.Token, nil
	})
}

// Emote finds an emote in the catalog.
func (f *Fake) Emote(id string) (api.Emote, bool) {
	for _, e := range f.Catalog {
		if e.ID == id {
			return e, true
		}
	}
	return api.Emote{}, false
}

type body struct {
	OperationName string                     `json:"operationName"`
	Variables     map[string]json.RawMessage `json:"variables"`
}

type gqlError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	var b body
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Calls[b.OperationName]++

	authed := r.Header.Get("Authorization") == "Bearer "+f.Token && f.Token != ""
	if r.Header.Get("Authorization") != "" && !authed {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status":"Unauthorized","error_code":1000,"error":"invalid session"}`))
		return
	}

	data, gerr := f.answer(b, authed)
	if gerr == nil && f.Lose[b.OperationName] {
		http.Error(w, "lost", http.StatusBadGateway)
		return
	}
	out := map[string]any{"data": data}
	if gerr != nil {
		out = map[string]any{"data": nil, "errors": []gqlError{*gerr}}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func fail(code, message string) *gqlError {
	return &gqlError{Message: message, Extensions: map[string]any{"code": code}}
}

func str(b body, name string) string {
	var s string
	json.Unmarshal(b.Variables[name], &s)
	return s
}

func connections(twitchID, login string) []map[string]any {
	if twitchID == "" {
		return []map[string]any{}
	}
	return []map[string]any{{"platform": "TWITCH", "platformId": twitchID, "platformUsername": login}}
}

func emote(e api.Emote) map[string]any {
	return map[string]any{
		"id": e.ID, "defaultName": e.Name, "deleted": e.Deleted,
		"flags": map[string]any{"nsfw": e.NSFW, "publicListed": e.Listed, "private": e.Private},
		"owner": map[string]any{"mainConnection": map[string]any{"platformDisplayName": e.Owner}},
	}
}

// answer runs one operation against the state.
func (f *Fake) answer(b body, authed bool) (any, *gqlError) {
	switch b.OperationName {
	case "Self":
		if !authed {
			return map[string]any{"users": map[string]any{"me": nil}}, nil
		}
		return map[string]any{"users": map[string]any{"me": map[string]any{"id": f.Self.ID, "connections": connections(f.Self.TwitchID, f.Self.Login)}}}, nil

	case "Invites":
		if !authed {
			return map[string]any{"users": map[string]any{"me": nil}}, nil
		}
		var list []map[string]any
		for _, in := range f.Invites {
			list = append(list, map[string]any{"userId": in.UserID, "state": in.State, "user": map[string]any{"connections": connections(in.TwitchID, in.Login)}})
		}
		return map[string]any{"users": map[string]any{"me": map[string]any{"editorFor": list}}}, nil

	case "Channel":
		ch := f.Channels[str(b, "twitchId")]
		if ch == nil {
			return map[string]any{"users": map[string]any{"userByConnection": nil}}, nil
		}
		var sets, editors []map[string]any
		for _, s := range ch.Sets {
			sets = append(sets, map[string]any{"id": s.ID, "name": s.Name, "kind": "NORMAL", "capacity": s.Capacity})
		}
		sets = append(sets, map[string]any{"id": "personal-" + ch.ID, "name": "Personal", "kind": "PERSONAL", "capacity": 5})
		for _, e := range ch.Editors {
			editors = append(editors, map[string]any{
				"editorId": e.ID, "state": e.State,
				"permissions": map[string]any{"emoteSet": map[string]any{"manage": e.ManageSets}, "user": map[string]any{"manageProfile": e.ManageProfile}},
				"editor":      map[string]any{"connections": connections(e.TwitchID, e.Login)},
			})
		}
		var active any
		if ch.Active != "" {
			active = ch.Active
		}
		return map[string]any{"users": map[string]any{"userByConnection": map[string]any{
			"id": ch.ID, "mainConnection": map[string]any{"platformDisplayName": ch.Name},
			"style": map[string]any{"activeEmoteSetId": active}, "emoteSets": sets, "editors": editors,
		}}}, nil

	case "Contents":
		_, s := f.set(str(b, "setId"))
		if s == nil {
			return map[string]any{"emoteSets": map[string]any{"emoteSet": nil}}, nil
		}
		var page, perPage int
		json.Unmarshal(b.Variables["page"], &page)
		json.Unmarshal(b.Variables["perPage"], &perPage)
		items := []map[string]any{}
		for i, e := range s.Emotes {
			if i >= (page-1)*perPage && i < page*perPage {
				items = append(items, map[string]any{"alias": e.Alias, "emote": emote(e.Emote)})
			}
		}
		return map[string]any{"emoteSets": map[string]any{"emoteSet": map[string]any{
			"id": s.ID, "name": s.Name, "capacity": s.Capacity,
			"emotes": map[string]any{"totalCount": len(s.Emotes), "items": items},
		}}}, nil

	case "Emote":
		e, ok := f.Emote(str(b, "id"))
		if !ok {
			return map[string]any{"emotes": map[string]any{"emote": nil}}, nil
		}
		return map[string]any{"emotes": map[string]any{"emote": emote(e)}}, nil

	case "Search":
		var exact bool
		json.Unmarshal(b.Variables["exact"], &exact)
		q := strings.ToLower(str(b, "query"))
		items := []map[string]any{}
		for _, e := range f.Catalog {
			name := strings.ToLower(e.Name)
			if (exact && name == q) || (!exact && strings.Contains(name, q)) {
				items = append(items, emote(e))
			}
		}
		return map[string]any{"emotes": map[string]any{"search": map[string]any{"items": items}}}, nil
	}

	if !authed {
		return nil, fail("LOGIN_REQUIRED", "LOGIN_REQUIRED you are not logged in")
	}
	switch b.OperationName {
	case "AddEmote", "RemoveEmote", "RenameEmote":
		var id struct {
			EmoteID string `json:"emoteId"`
			Alias   string `json:"alias"`
		}
		json.Unmarshal(b.Variables["emote"], &id)
		ch, s := f.set(str(b, "setId"))
		if s == nil {
			return nil, fail("BAD_REQUEST", "emote set not found")
		}
		if !f.may(ch, api.ManageSets) {
			return nil, fail("LACKING_PRIVILEGES", "you are not an editor for this user")
		}
		taken := func(alias string) bool {
			return slices.ContainsFunc(s.Emotes, func(e api.SetEmote) bool { return e.Alias == alias })
		}
		at := slices.IndexFunc(s.Emotes, func(e api.SetEmote) bool { return e.Emote.ID == id.EmoteID && (id.Alias == "" || e.Alias == id.Alias) })
		switch b.OperationName {
		case "AddEmote":
			e, ok := f.Emote(id.EmoteID)
			if !ok {
				return nil, fail("BAD_REQUEST", "emote not found")
			}
			alias := id.Alias
			if alias == "" {
				alias = e.Name
			}
			if taken(alias) {
				return nil, fail("BAD_REQUEST", "this emote has a conflicting name")
			}
			if s.Capacity > 0 && len(s.Emotes) >= s.Capacity {
				return nil, fail("BAD_REQUEST", "emote set is at capacity")
			}
			s.Emotes = append(s.Emotes, api.SetEmote{Alias: alias, Emote: e})
		case "RemoveEmote":
			if at < 0 {
				return nil, fail("BAD_REQUEST", "emote not found in set")
			}
			s.Emotes = slices.DeleteFunc(s.Emotes, func(e api.SetEmote) bool {
				return e.Emote.ID == id.EmoteID && (id.Alias == "" || e.Alias == id.Alias)
			})
		case "RenameEmote":
			alias := str(b, "alias")
			if at < 0 {
				return nil, fail("BAD_REQUEST", "emote not found in set")
			}
			if taken(alias) {
				return nil, fail("BAD_REQUEST", "this emote has a conflicting name")
			}
			s.Emotes[at].Alias = alias
		}
		return map[string]any{"emoteSets": map[string]any{"emoteSet": map[string]any{"id": s.ID}}}, nil

	case "SwitchSet":
		var ch *Channel
		for _, c := range f.Channels {
			if c.ID == str(b, "userId") {
				ch = c
			}
		}
		if ch == nil || !slices.ContainsFunc(ch.Sets, func(s *Set) bool { return s.ID == str(b, "setId") }) {
			return nil, fail("BAD_REQUEST", "emote set not found")
		}
		if !f.may(ch, api.ManageProfile) {
			return nil, fail("LACKING_PRIVILEGES", "you do not have permission to change this user's active emote set")
		}
		ch.Active = str(b, "setId")
		return map[string]any{"users": map[string]any{"user": map[string]any{"activeEmoteSet": map[string]any{"id": ch.ID}}}}, nil

	case "AcceptInvite":
		for i := range f.Invites {
			if f.Invites[i].UserID == str(b, "userId") && str(b, "editorId") == f.Self.ID {
				f.Invites[i].State = api.Accepted
				for _, ch := range f.Channels {
					for j := range ch.Editors {
						if ch.ID == f.Invites[i].UserID && ch.Editors[j].ID == f.Self.ID {
							ch.Editors[j].State = api.Accepted
						}
					}
				}
				return map[string]any{"userEditors": map[string]any{"editor": map[string]any{"updateState": map[string]any{"state": "ACCEPTED"}}}}, nil
			}
		}
		return nil, fail("BAD_REQUEST", "editor not found")
	}
	return nil, fail("BAD_REQUEST", "unknown operation "+b.OperationName)
}

// set finds a set and the channel that has it.
func (f *Fake) set(id string) (*Channel, *Set) {
	for _, ch := range f.Channels {
		for _, s := range ch.Sets {
			if s.ID == id {
				return ch, s
			}
		}
	}
	return nil, nil
}

// may is whether Lys's account may make a change needing p on ch.
func (f *Fake) may(ch *Channel, p api.Permission) bool {
	for i := range ch.Editors {
		if ch.Editors[i].ID == f.Self.ID {
			return ch.Editors[i].May(p) == api.Allowed
		}
	}
	return false
}
