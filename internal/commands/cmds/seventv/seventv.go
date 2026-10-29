// Package seventv is the !7tv command: a channel's 7TV editors manage its
// emotes from chat. The channel's 7TV editor list decides who may change
// what; Twitch roles don't count.
package seventv

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lsariol/lys/internal/commands"
	api "github.com/lsariol/lys/internal/features/seventv"
)

// SevenTV runs !7tv. Changes go through Lys's own 7TV account, so it checks
// the sender against the channel's editor list before every one.
type SevenTV struct {
	client *api.Client
	locks  locks
	undo   undoLog
	now    func() time.Time
}

func New(client *api.Client) *SevenTV {
	return &SevenTV{client: client, now: time.Now}
}

// subcommands is what !7tv does, for !7tv help and unknown words.
const subcommands = "add, remove, rename, undo, sets, set, search, editors, emote, status"

func (*SevenTV) Spec() commands.Spec {
	return commands.Spec{
		Name:        "7tv",
		Description: "The channel's 7TV emotes; the channel's 7TV editors can change them.",
		Usage:       "!7tv <" + strings.ReplaceAll(subcommands, ", ", "|") + ">",
		Timeout:     20 * time.Second,
		Concurrency: 16,
	}
}

func (s *SevenTV) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) == 0 {
		return help(e), nil
	}
	switch strings.ToLower(e.Args[0]) {
	case "help":
		return help(e), nil
	case "add":
		return s.add(ctx, e)
	case "remove":
		return s.remove(ctx, e)
	case "rename":
		return s.rename(ctx, e)
	case "undo":
		return s.undoLast(ctx, e)
	case "sets":
		return s.sets(ctx, e)
	case "set":
		return s.switchSet(ctx, e)
	case "search":
		return s.search(ctx, e)
	case "editors":
		return s.editors(ctx, e)
	case "emote":
		return s.emote(ctx, e)
	case "status":
		return s.status(ctx, e)
	}
	return say("There's no %s7tv %s. Try: %s.", e.Prefix, e.Args[0], subcommands), nil
}

func help(e commands.Request) commands.Reply {
	return say("%s7tv %s. Changes are for the channel's 7TV editors.", e.Prefix, subcommands)
}

func say(format string, args ...any) commands.Reply {
	return commands.Reply{Text: fmt.Sprintf(format, args...)}
}

// usage answers wrong arguments with the subcommand's form, such as "remove <name>".
func usage(e commands.Request, form string) commands.Reply {
	return say("Usage: %s7tv %s", e.Prefix, form)
}

// failed is the reply when 7TV fails or refuses.
func failed(err error) commands.Reply {
	var apiErr *api.APIError
	switch {
	case errors.Is(err, api.ErrNoToken), errors.Is(err, api.ErrSession):
		return say("Lys's 7TV login needs renewing, so it can't change emotes right now.")
	case errors.Is(err, api.ErrRateLimited):
		return say("7TV is busy, try again in a moment.")
	case errors.As(err, &apiErr) && apiErr.Code == "LACKING_PRIVILEGES":
		return say("7TV refused: Lys doesn't have the permission it needs here.")
	case errors.As(err, &apiErr) && apiErr.Code == "BAD_REQUEST":
		return say("7TV refused: %s.", apiErr.Message)
	}
	return say("Couldn't reach 7TV, try again in a moment.")
}

// unknown reports whether a change may have happened despite err: the
// request failed in transit, so 7TV's answer was lost.
func unknown(err error) bool {
	var apiErr *api.APIError
	return err != nil && !errors.As(err, &apiErr) && !errors.Is(err, api.ErrSession) &&
		!errors.Is(err, api.ErrNoToken) && !errors.Is(err, api.ErrRateLimited)
}

// channel reads the channel's 7TV account. When it can't, reply says why.
func (s *SevenTV) channel(ctx context.Context, e commands.Request) (*api.Channel, *commands.Reply, error) {
	if e.Platform != "twitch" {
		r := say("7TV commands only work on Twitch.")
		return nil, &r, nil
	}
	ch, err := s.client.Channel(ctx, e.Channel.ID)
	if err != nil {
		r := failed(err)
		return nil, &r, err
	}
	if ch == nil {
		r := say("This channel has no 7TV account.")
		return nil, &r, nil
	}
	return ch, nil, nil
}

// authorize checks that the sender may make a change that needs p, then
// that Lys's 7TV account may. When either may not, reply says why.
func (s *SevenTV) authorize(ctx context.Context, e commands.Request, ch *api.Channel, p api.Permission) (*commands.Reply, error) {
	what := "change emotes"
	permission := "Manage emote sets"
	if p == api.ManageProfile {
		what, permission = "switch sets", "Manage profile"
	}

	if e.User.ID == "" || e.User.ID != e.Channel.ID {
		var r commands.Reply
		switch ch.EditorByTwitch(e.User.ID).May(p) {
		case api.NotEditor:
			r = say("@%s, only the channel's 7TV editors can %s.", e.User.Name, what)
		case api.NotYet:
			r = say("@%s, accept the channel's 7TV editor invite first.", e.User.Name)
		case api.Lacking:
			r = say("@%s, you need 7TV's %s permission to %s.", e.User.Name, permission, what)
		}
		if r.Text != "" {
			return &r, nil
		}
	}

	self, err := s.client.Self(ctx)
	if err != nil {
		r := failed(err)
		return &r, err
	}
	login := self.Login
	if login == "" {
		login = "Lys's 7TV account"
	}
	var r commands.Reply
	switch ch.EditorByID(self.ID).May(p) {
	case api.NotEditor:
		r = say("Lys isn't a 7TV editor here yet. The streamer can add %s as an editor on 7tv.app, with %s.", login, permission)
	case api.NotYet:
		r = say("Lys hasn't accepted the channel's 7TV invite yet; it does within a few minutes.")
	case api.Lacking:
		r = say("Lys needs 7TV's %s permission to %s. The streamer can give it to %s on 7tv.app.", permission, what, login)
	}
	if r.Text != "" {
		return &r, nil
	}
	return nil, nil
}

// logChange records one change, done or not, for the central logs.
func logChange(e commands.Request, ch *api.Channel, action string, err error) {
	result := "done"
	if err != nil {
		result = "failed: " + err.Error()
	}
	log.Printf("[7TV] %s in %q (Twitch %s, 7TV %s) by %q (Twitch %s): %s",
		action, e.Channel.Name, e.Channel.ID, ch.ID, e.User.Name, e.User.ID, result)
}

// locks makes changes to one set, or to one account's active set, take turns.
type locks struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func (l *locks) lock(key string) (unlock func()) {
	l.mu.Lock()
	if l.m == nil {
		l.m = make(map[string]*sync.Mutex)
	}
	m, ok := l.m[key]
	if !ok {
		m = &sync.Mutex{}
		l.m[key] = m
	}
	l.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// settled is a context to check a change's outcome in, after ctx may have run out.
func settled(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}
