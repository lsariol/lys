package seventv

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
	api "github.com/lsariol/lys/internal/features/seventv"
	"github.com/lsariol/lys/internal/features/seventv/seventvtest"
)

// Twitch IDs in the tests.
const (
	streamer  = "100" // the channel
	editor    = "200" // 7TV editor with Manage emote sets
	manager   = "201" // 7TV editor with Manage emote sets and Manage profile
	invited   = "202" // invited, not accepted
	powerless = "203" // accepted editor with no permissions
	moderator = "300" // Twitch moderator, not a 7TV editor
	elsewhere = "500" // another channel, where editor isn't one
)

var (
	kekw    = api.Emote{ID: "01F61B1440000991F7SWQNMVX7", Name: "KEKW", Owner: "WH0LUV", Listed: true}
	kekw2   = api.Emote{ID: "01FCP0YPQ800037YGEKHNTNXY1", Name: "KEKW", Owner: "DerXiatos", Listed: true}
	nsfw    = api.Emote{ID: "01G00000000000000000000NSF", Name: "KEKW", Owner: "x", Listed: true, NSFW: true}
	haha    = api.Emote{ID: "01GNN1J9AR0000HECQDPP1DYF3", Name: "haha", Owner: "Panshui", Listed: true}
	private = api.Emote{ID: "01H00000000000000000000PRV", Name: "mine", Owner: "y", Listed: true, Private: true}
)

type fixture struct {
	fake *seventvtest.Fake
	cmd  *SevenTV
	now  time.Time
}

func setup(t *testing.T) *fixture {
	f := seventvtest.New(t)
	f.Token = "session"
	f.Self = api.Account{ID: "lys", TwitchID: "999", Login: "lyslink"}
	f.Catalog = []api.Emote{nsfw, kekw, kekw2, haha, private}
	f.Channels[streamer] = &seventvtest.Channel{
		ID: "streamer7tv", Name: "Streamer", Active: "main",
		Sets: []*seventvtest.Set{
			{ID: "main", Name: "Main", Capacity: 5, Emotes: []api.SetEmote{{Alias: "haha", Emote: haha}}},
			{ID: "halloween", Name: "Halloween", Capacity: 5},
		},
		Editors: []api.Editor{
			{ID: "lys", State: api.Accepted, ManageSets: true, ManageProfile: true, TwitchID: "999", Login: "lyslink"},
			{ID: "e200", State: api.Accepted, ManageSets: true, TwitchID: editor, Login: "editor"},
			{ID: "e201", State: api.Accepted, ManageSets: true, ManageProfile: true, TwitchID: manager, Login: "manager"},
			{ID: "e202", State: api.Pending, ManageSets: true, ManageProfile: true, TwitchID: invited, Login: "invited"},
			{ID: "e203", State: api.Accepted, TwitchID: powerless, Login: "powerless"},
		},
	}
	f.Channels[elsewhere] = &seventvtest.Channel{
		ID: "other7tv", Name: "Other", Active: "other",
		Sets:    []*seventvtest.Set{{ID: "other", Name: "Other", Capacity: 5}},
		Editors: []api.Editor{{ID: "lys", State: api.Accepted, ManageSets: true, ManageProfile: true}},
	}
	fx := &fixture{fake: f, now: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	fx.cmd = New(f.Client())
	fx.cmd.now = func() time.Time { return fx.now }
	return fx
}

// run sends !7tv args in the streamer's channel as the Twitch user.
func (fx *fixture) run(t *testing.T, user string, args ...string) string {
	t.Helper()
	return fx.runIn(t, streamer, user, 0, args...)
}

func (fx *fixture) runIn(t *testing.T, channel, user string, roles core.Roles, args ...string) string {
	t.Helper()
	req := commands.Request{Prefix: "!", Args: args}
	req.Platform = "twitch"
	req.Channel = core.Channel{ID: channel, Name: "chan" + channel}
	req.User = core.User{ID: user, Name: "u" + user, Roles: roles}
	reply, _ := fx.cmd.Run(context.Background(), req)
	return reply.Text
}

// aliases is the set's emote names.
func (fx *fixture) aliases(setID string) []string {
	fx.fake.Mu.Lock()
	defer fx.fake.Mu.Unlock()
	var out []string
	for _, ch := range fx.fake.Channels {
		for _, s := range ch.Sets {
			if s.ID == setID {
				for _, e := range s.Emotes {
					out = append(out, e.Alias+"="+e.Emote.ID)
				}
			}
		}
	}
	return out
}

func (fx *fixture) writes() int {
	fx.fake.Mu.Lock()
	defer fx.fake.Mu.Unlock()
	n := 0
	for _, op := range []string{"AddEmote", "RemoveEmote", "RenameEmote", "SwitchSet", "AcceptInvite"} {
		n += fx.fake.Calls[op]
	}
	return n
}

func expect(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
}

func TestArguments(t *testing.T) {
	fx := setup(t)
	for _, args := range [][]string{
		{"add"}, {"add", "a", "b", "c"}, {"remove"}, {"remove", "a", "b"}, {"rename", "a"},
		{"undo", "x"}, {"sets", "x"}, {"set"}, {"search"}, {"editors", "x"}, {"emote"}, {"status", "x"},
	} {
		if got := fx.run(t, streamer, args...); !strings.HasPrefix(got, "Usage: !7tv ") {
			t.Errorf("%v: %q", args, got)
		}
	}
	expect(t, fx.run(t, streamer, "nope"), "There's no !7tv nope. Try: add, remove")
	expect(t, fx.run(t, streamer), "!7tv add, remove, rename")
	if fx.writes() != 0 {
		t.Error("wrong arguments changed something")
	}
}

// 1, 15: an editor with Manage emote sets, not a Twitch moderator, can add
// by name (the most used exact match, never an NSFW one) and remove.
func TestEditorAddsAndRemoves(t *testing.T) {
	fx := setup(t)
	expect(t, fx.run(t, editor, "add", "KEKW"), "Added KEKW (7tv.app/emotes/"+kekw.ID+") · 2/5 slots")
	expect(t, strings.Join(fx.aliases("main"), " "), "KEKW="+kekw.ID)
	expect(t, fx.run(t, editor, "remove", "KEKW"), "Removed KEKW · 1/5 slots")
	if got := fx.aliases("main"); len(got) != 1 {
		t.Errorf("set after remove: %v", got)
	}
	expect(t, fx.run(t, editor, "add", "nothing"), "No 7TV emote is called nothing")
}

// 2, 3, 4, 7: only accepted 7TV editors with the permission, in their own
// channel, can change anything; Twitch roles don't count.
func TestRefusals(t *testing.T) {
	fx := setup(t)
	expect(t, fx.runIn(t, streamer, moderator, core.RoleModerator, "add", "KEKW"), "only the channel's 7TV editors can change emotes")
	expect(t, fx.runIn(t, streamer, moderator, core.RoleBroadcaster, "add", "KEKW"), "only the channel's 7TV editors")
	expect(t, fx.run(t, invited, "add", "KEKW"), "accept the channel's 7TV editor invite first")
	expect(t, fx.run(t, powerless, "remove", "haha"), "you need 7TV's Manage emote sets permission")
	expect(t, fx.runIn(t, elsewhere, editor, core.RoleModerator, "add", "KEKW"), "only the channel's 7TV editors")
	expect(t, fx.run(t, "", "add", "KEKW"), "only the channel's 7TV editors")
	if fx.writes() != 0 {
		t.Errorf("a refused change reached 7TV (%d writes)", fx.writes())
	}
	expect(t, fx.run(t, streamer, "add", "KEKW"), "Added KEKW")
}

// 5, 6: switching sets needs Manage profile, from the sender and from Lys.
func TestSwitchSet(t *testing.T) {
	fx := setup(t)
	expect(t, fx.run(t, editor, "set", "halloween"), "you need 7TV's Manage profile permission to switch sets")
	expect(t, fx.run(t, manager, "set", "Nope"), "There's no set called Nope. The sets: Main, Halloween.")
	expect(t, fx.run(t, manager, "set", "Main"), "Main is already the active set")
	expect(t, fx.run(t, manager, "set", "halloween"), "Switched the active set to Halloween")
	if fx.fake.Channels[streamer].Active != "halloween" {
		t.Error("the set didn't switch")
	}

	fx.fake.Channels[streamer].Editors[0].ManageProfile = false
	expect(t, fx.run(t, manager, "set", "Main"), "Lys needs 7TV's Manage profile permission to switch sets")
	expect(t, fx.run(t, manager, "add", "KEKW"), "Added KEKW")
}

// 8: a revoked editor is stopped on their next change.
func TestRevoked(t *testing.T) {
	fx := setup(t)
	expect(t, fx.run(t, editor, "add", "KEKW"), "Added")
	fx.fake.Mu.Lock()
	fx.fake.Channels[streamer].Editors = fx.fake.Channels[streamer].Editors[:1]
	fx.fake.Mu.Unlock()
	expect(t, fx.run(t, editor, "remove", "KEKW"), "only the channel's 7TV editors")
}

// Lys must be an accepted editor with the permission too.
func TestLysPermissions(t *testing.T) {
	fx := setup(t)
	ch := fx.fake.Channels[streamer]
	ch.Editors[0].State = api.Pending
	expect(t, fx.run(t, editor, "add", "KEKW"), "Lys hasn't accepted the channel's 7TV invite yet")
	ch.Editors[0] = api.Editor{ID: "lys", State: api.Accepted}
	expect(t, fx.run(t, editor, "add", "KEKW"), "Lys needs 7TV's Manage emote sets permission to change emotes. The streamer can give it to lyslink")
	ch.Editors = ch.Editors[1:]
	expect(t, fx.run(t, editor, "add", "KEKW"), "Lys isn't a 7TV editor here yet. The streamer can add lyslink")
	if fx.writes() != 0 {
		t.Error("a change reached 7TV without Lys's permission")
	}
}

// 9, 16: duplicates, name collisions, a full set, links and local names.
func TestAddChecks(t *testing.T) {
	fx := setup(t)
	expect(t, fx.run(t, editor, "add", "7tv.app/emotes/"+kekw2.ID, "kek"), "Added KEKW as kek (7tv.app/emotes/"+kekw2.ID+")")
	expect(t, fx.run(t, editor, "add", "https://7tv.app/emotes/"+kekw2.ID), "KEKW is already in the set, as kek")
	expect(t, fx.run(t, editor, "add", kekw.ID, "haha"), "There's already an emote called haha in the set")
	expect(t, fx.run(t, editor, "add", "https://evil.example/emotes/"+kekw.ID), "Only 7TV emote links work")
	expect(t, fx.run(t, editor, "add", "https://7tv.app/emotes/01H00000000000000000000000"), "There's no 7TV emote")
	expect(t, fx.run(t, editor, "add", private.ID), "That emote is private")

	fx.fake.Channels[streamer].Sets[0].Capacity = 2
	expect(t, fx.run(t, editor, "add", "KEKW"), "The set is full (2/2 slots)")
	expect(t, fx.run(t, editor, "rename", "kek", "haha"), "There's already an emote called haha")
	expect(t, fx.run(t, editor, "rename", "kek", "lol"), "Renamed kek to lol")
	expect(t, fx.run(t, editor, "remove", "kek"), "There's no emote called kek in the set")
}

// 9: when 7TV's answer is lost, Lys checks the set instead of trying again.
func TestLostAnswer(t *testing.T) {
	fx := setup(t)
	fx.fake.Lose["AddEmote"] = true
	expect(t, fx.run(t, editor, "add", "KEKW"), "Added KEKW")
	if fx.fake.Calls["AddEmote"] != 1 {
		t.Errorf("the add was sent %d times", fx.fake.Calls["AddEmote"])
	}
}

// 9: an expired session stops changes with a clear answer.
func TestExpiredSession(t *testing.T) {
	fx := setup(t)
	fx.cmd.client = api.New(http.DefaultClient, fx.fake.URL, func(context.Context) (string, error) { return "expired", nil })
	expect(t, fx.run(t, editor, "add", "KEKW"), "Lys's 7TV login needs renewing")
	expect(t, fx.run(t, moderator, "status"), "Lys's 7TV login needs renewing")
	expect(t, fx.run(t, moderator, "sets"), "Main (active)")
}

// 10: when 7TV is down, the command answers instead of failing Lys.
func TestUnreachable(t *testing.T) {
	fx := setup(t)
	fx.cmd.client = api.New(http.DefaultClient, "http://127.0.0.1:1", func(context.Context) (string, error) { return "t", nil })
	expect(t, fx.run(t, editor, "add", "KEKW"), "Couldn't reach 7TV")
	expect(t, fx.run(t, editor, "editors"), "Couldn't reach 7TV")
}

// 13: two adds of one emote at the same moment add it once.
func TestConcurrentAdds(t *testing.T) {
	fx := setup(t)
	var wg sync.WaitGroup
	replies := make([]string, 4)
	for i := range replies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replies[i] = fx.run(t, editor, "add", "KEKW")
		}()
	}
	wg.Wait()
	added := 0
	for _, r := range replies {
		if strings.HasPrefix(r, "Added") {
			added++
		}
	}
	if added != 1 || len(fx.aliases("main")) != 2 {
		t.Errorf("added %d times; set %v; replies %q", added, fx.aliases("main"), replies)
	}
}

// 17: undo reverts each kind of change, the last one only, within 30
// minutes, for someone allowed to make it.
func TestUndo(t *testing.T) {
	fx := setup(t)
	expect(t, fx.run(t, editor, "undo"), "There's nothing to undo")

	fx.run(t, editor, "add", "KEKW")
	expect(t, fx.run(t, moderator, "undo"), "only the channel's 7TV editors")
	expect(t, fx.run(t, manager, "undo"), "Undo add emote KEKW by @u200")
	if got := fx.aliases("main"); len(got) != 1 {
		t.Errorf("after undoing the add: %v", got)
	}
	expect(t, fx.run(t, manager, "undo"), "There's nothing to undo")

	fx.run(t, editor, "remove", "haha")
	expect(t, fx.run(t, editor, "undo"), "Undo remove emote haha by @u200")
	expect(t, strings.Join(fx.aliases("main"), " "), "haha="+haha.ID)

	fx.run(t, editor, "rename", "haha", "hehe")
	expect(t, fx.run(t, editor, "undo"), "Undo rename emote haha to hehe by @u200")
	expect(t, strings.Join(fx.aliases("main"), " "), "haha="+haha.ID)

	fx.run(t, manager, "set", "Halloween")
	expect(t, fx.run(t, editor, "undo"), "you need 7TV's Manage profile permission")
	expect(t, fx.run(t, manager, "undo"), "Undo set switch to Halloween by @u201")
	if fx.fake.Channels[streamer].Active != "main" {
		t.Error("the switch wasn't undone")
	}

	fx.run(t, editor, "add", "KEKW")
	fx.now = fx.now.Add(31 * time.Minute)
	expect(t, fx.run(t, editor, "undo"), "There's nothing to undo from the last 30 minutes")

	fx.now = fx.now.Add(time.Minute)
	fx.run(t, editor, "add", "7tv.app/emotes/"+kekw2.ID, "kek")
	fx.run(t, editor, "remove", "kek")
	fx.run(t, editor, "undo")
	expect(t, fx.run(t, editor, "undo"), "There's nothing to undo")
	expect(t, strings.Join(fx.aliases("main"), " "), "kek="+kekw2.ID)
}

// 18: the read-only subcommands work for everyone and change nothing.
func TestReads(t *testing.T) {
	fx := setup(t)
	expect(t, fx.run(t, moderator, "sets"), "7TV sets: Main (active), Halloween")
	expect(t, fx.run(t, moderator, "editors"), "7TV editors: lyslink (emotes, sets), editor (emotes), manager (emotes, sets), powerless (neither emotes nor sets)")
	expect(t, fx.run(t, moderator, "search", "kek"), "KEKW by WH0LUV 7tv.app/emotes/"+kekw.ID+" · KEKW by DerXiatos")
	expect(t, fx.run(t, moderator, "emote", "HAHA"), "haha by Panshui: 7tv.app/emotes/"+haha.ID)
	expect(t, fx.run(t, moderator, "emote", "nope"), "There's no emote called nope")
	expect(t, fx.run(t, moderator, "status"), "Active set: Main · 1/5 slots · Lys can change emotes and switch sets")
	expect(t, fx.runIn(t, "404", moderator, 0, "status"), "This channel has no 7TV account")
	if fx.writes() != 0 {
		t.Error("a read changed something")
	}
	if strings.Contains(fx.run(t, moderator, "search", "kek"), nsfw.ID) {
		t.Error("search showed an NSFW emote")
	}
}
