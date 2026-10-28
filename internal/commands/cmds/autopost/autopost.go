package autopost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/scheduler"
	"github.com/lsariol/lys/internal/sources"
)

// Kind is the scheduled_jobs kind of a channel autopost.
const Kind = "autopost"

// MinMessages is how many chat messages a channel needs since an autopost's last
// post for it to post again, so quiet and offline channels get nothing.
const MinMessages = 5

const (
	minEvery     = 5 * time.Minute
	maxEvery     = 24 * time.Hour
	maxAutoposts = 10 // per channel
	maxMessage   = 450
	maxQuoted    = 40  // characters of an autopost's message shown in replies
	maxReply     = 450 // Twitch refuses messages over 500
)

// Jobs is the part of the scheduler's store autoposts use.
type Jobs interface {
	Schedule(ctx context.Context, j scheduler.NewJob) (int64, error)
	Pending(ctx context.Context, kind string, connectionID int64) ([]scheduler.Job, error)
	Cancel(ctx context.Context, id, connectionID int64, kind string) error
}

// Activity tells how many chat messages a channel has had since a time.
type Activity interface {
	Since(platform, channelID string, after time.Time) int
}

// Post is the scheduler handler for autoposts: it posts the autopost's message
// unless the channel had fewer than MinMessages since the last post.
func Post(chat Activity) scheduler.Handler {
	return func(ctx context.Context, j scheduler.Job, pub sources.Publisher) error {
		if chat.Since(j.Platform, j.Channel.ID, j.RunAt.Add(-j.RepeatEvery)) < MinMessages {
			return nil
		}
		return scheduler.SendMessage(ctx, j, pub)
	}
}

// Autopost posts a channel's message on repeat. Only the channel's owner and
// moderators manage them; the bot's owner gets no say in other channels.
type Autopost struct {
	jobs Jobs
	now  func() time.Time
}

func New(jobs Jobs) *Autopost { return &Autopost{jobs: jobs, now: time.Now} }

func (*Autopost) Spec() commands.Spec {
	return commands.Spec{
		Name:        "autopost",
		Aliases:     []string{"autoposts"},
		Description: "Post a message in this channel on repeat, while chat is active (owner and moderators).",
		Usage:       "!autopost add <how often, e.g. 30m> <message> | !autopost list | !autopost remove <number>",
		Timeout:     5 * time.Second,
	}
}

func (t *Autopost) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	usage := fmt.Sprintf("Usage: %sautopost add 30m <message> (repeats every 30 minutes; 5m to 24h), %sautopost list, or %sautopost remove <number>.", e.Prefix, e.Prefix, e.Prefix)
	if len(e.Args) == 0 {
		return commands.Reply{Text: usage}, nil
	}
	if !commands.IsChannelOwner(e) && !e.User.Roles.Has(core.RoleModerator) {
		return commands.Reply{Text: "Only the channel's owner and moderators can manage autoposts."}, nil
	}
	if e.Connection.ID == 0 {
		return commands.Reply{Text: "Autoposts aren't available right now. Try again in a moment."},
			fmt.Errorf("autopost: %q has no channel in Lys", e.Channel.Name)
	}

	switch strings.ToLower(e.Args[0]) {
	case "add":
		if len(e.Args) < 3 {
			return commands.Reply{Text: usage}, nil
		}
		return t.add(ctx, e, e.Args[1], strings.Join(e.Args[2:], " "))
	case "list":
		return t.list(ctx, e)
	case "remove", "delete":
		if len(e.Args) != 2 {
			return commands.Reply{Text: usage}, nil
		}
		return t.remove(ctx, e, e.Args[1])
	default:
		return commands.Reply{Text: usage}, nil
	}
}

func (t *Autopost) add(ctx context.Context, e commands.Request, every, text string) (commands.Reply, error) {
	d, err := commands.ParseDuration(every)
	if err != nil {
		return commands.Reply{Text: fmt.Sprintf("%q isn't a time like 30m or 2h.", every)}, nil
	}
	if d < minEvery || d > maxEvery {
		return commands.Reply{Text: "Autoposts can repeat every 5 minutes up to every 24 hours."}, nil
	}
	if len(text) > maxMessage {
		return commands.Reply{Text: fmt.Sprintf("Autopost messages can be up to %d characters.", maxMessage)}, nil
	}

	pending, err := t.jobs.Pending(ctx, Kind, e.Connection.ID)
	if err != nil {
		return commands.Reply{Text: "There was an error saving the autopost. Please try again."}, err
	}
	if len(pending) >= maxAutoposts {
		return commands.Reply{Text: fmt.Sprintf("This channel already has %d autoposts. Remove one first (%sautopost list).", len(pending), e.Prefix)}, nil
	}

	id, err := t.jobs.Schedule(ctx, scheduler.NewJob{
		Kind:         Kind,
		ConnectionID: e.Connection.ID,
		CreatedBy:    e.Identity.PlatformUserID,
		Payload:      scheduler.Message{Text: text},
		RunAt:        t.now().Add(d),
		RepeatEvery:  d,
	})
	if err != nil {
		return commands.Reply{Text: "There was an error saving the autopost. Please try again."}, err
	}
	return commands.Reply{Text: fmt.Sprintf("Autopost #%d added: %s every %s, while chat is active (%d+ messages since its last post).",
		id, quote(text), commands.FormatDuration(d), MinMessages)}, nil
}

func (t *Autopost) list(ctx context.Context, e commands.Request) (commands.Reply, error) {
	pending, err := t.jobs.Pending(ctx, Kind, e.Connection.ID)
	if err != nil {
		return commands.Reply{Text: "There was an error reading the autoposts."}, err
	}
	if len(pending) == 0 {
		return commands.Reply{Text: fmt.Sprintf("This channel has no autoposts. Add one with %sautopost add 30m <message>.", e.Prefix)}, nil
	}

	head := fmt.Sprintf("Autoposts (%d): ", len(pending))
	tail := fmt.Sprintf(" Remove one with %sautopost remove <number>.", e.Prefix)
	var parts []string
	for i, j := range pending {
		var m scheduler.Message
		if err := json.Unmarshal(j.Payload, &m); err != nil {
			return commands.Reply{Text: "There was an error reading the autoposts."}, fmt.Errorf("autopost %d: decode payload: %w", j.ID, err)
		}
		part := fmt.Sprintf("#%d %s every %s, next in %s", j.ID, quote(m.Text), commands.FormatDuration(j.RepeatEvery), commands.FormatDuration(j.RunAt.Sub(t.now())))
		more := fmt.Sprintf(" | and %d more", len(pending)-i)
		if len(head)+len(strings.Join(append(parts, part), " | "))+len(more)+len(tail) > maxReply {
			parts = append(parts, strings.TrimPrefix(more, " | "))
			break
		}
		parts = append(parts, part)
	}
	return commands.Reply{Text: head + strings.Join(parts, " | ") + "." + tail}, nil
}

func (t *Autopost) remove(ctx context.Context, e commands.Request, arg string) (commands.Reply, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64)
	if err != nil {
		return commands.Reply{Text: fmt.Sprintf("Autoposts have numbers like #3; see %sautopost list.", e.Prefix)}, nil
	}
	if err := t.jobs.Cancel(ctx, id, e.Connection.ID, Kind); err != nil {
		if errors.Is(err, scheduler.ErrNotFound) {
			return commands.Reply{Text: fmt.Sprintf("This channel has no autopost #%d; see %sautopost list.", id, e.Prefix)}, nil
		}
		return commands.Reply{Text: "There was an error removing the autopost."}, err
	}
	return commands.Reply{Text: fmt.Sprintf("Autopost #%d removed.", id)}, nil
}

// quote shows an autopost's message in quotes, cut short if it's long.
func quote(text string) string {
	if r := []rune(text); len(r) > maxQuoted {
		text = strings.TrimSpace(string(r[:maxQuoted-1])) + "…"
	}
	return `"` + text + `"`
}
