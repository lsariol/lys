package timer

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

// Kind is the scheduled_jobs kind of a channel timer.
const Kind = "timer"

// MinMessages is how many chat messages a channel needs since a timer's last
// post for it to post again, so quiet and offline channels get nothing.
const MinMessages = 5

const (
	minEvery   = 5 * time.Minute
	maxEvery   = 24 * time.Hour
	maxTimers  = 10 // per channel
	maxMessage = 450
	maxQuoted  = 40  // characters of a timer's message shown in replies
	maxReply   = 450 // Twitch refuses messages over 500
)

// Jobs is the part of the scheduler's store timers use.
type Jobs interface {
	Schedule(ctx context.Context, j scheduler.NewJob) (int64, error)
	Pending(ctx context.Context, kind string, connectionID int64) ([]scheduler.Job, error)
	Cancel(ctx context.Context, id, connectionID int64, kind string) error
}

// Activity tells how many chat messages a channel has had since a time.
type Activity interface {
	Since(platform, channelID string, after time.Time) int
}

// Post is the scheduler handler for timers: it posts the timer's message
// unless the channel had fewer than MinMessages since the last post.
func Post(chat Activity) scheduler.Handler {
	return func(ctx context.Context, j scheduler.Job, pub sources.Publisher) error {
		if chat.Since(j.Platform, j.Channel.ID, j.RunAt.Add(-j.RepeatEvery)) < MinMessages {
			return nil
		}
		return scheduler.SendMessage(ctx, j, pub)
	}
}

// Timer posts a channel's message on repeat. Only the channel's owner and
// moderators manage them; the bot's owner gets no say in other channels.
type Timer struct {
	jobs Jobs
	now  func() time.Time
}

func New(jobs Jobs) *Timer { return &Timer{jobs: jobs, now: time.Now} }

func (*Timer) Spec() commands.Spec {
	return commands.Spec{
		Name:        "timer",
		Aliases:     []string{"timers"},
		Description: "Post a message in this channel on repeat, while chat is active (owner and moderators).",
		Usage:       "!timer add <every, like 30m or 2h> <message> | !timer list | !timer remove <number>",
		Timeout:     5 * time.Second,
	}
}

func (t *Timer) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	usage := fmt.Sprintf("Usage: %stimer add <every, like 30m> <message>, %stimer list, or %stimer remove <number>.", e.Prefix, e.Prefix, e.Prefix)
	if len(e.Args) == 0 {
		return commands.Reply{Text: usage}, nil
	}
	if !commands.IsChannelOwner(e) && !e.User.Roles.Has(core.RoleModerator) {
		return commands.Reply{Text: "Only the channel's owner and moderators can manage timers."}, nil
	}
	if e.Connection.ID == 0 {
		return commands.Reply{Text: "Timers aren't available right now. Try again in a moment."},
			fmt.Errorf("timer: %q has no channel in Lys", e.Channel.Name)
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

func (t *Timer) add(ctx context.Context, e commands.Request, every, text string) (commands.Reply, error) {
	d, err := commands.ParseDuration(every)
	if err != nil {
		return commands.Reply{Text: fmt.Sprintf("%q isn't a time like 30m or 2h.", every)}, nil
	}
	if d < minEvery || d > maxEvery {
		return commands.Reply{Text: "Timers can repeat every 5 minutes up to every 24 hours."}, nil
	}
	if len(text) > maxMessage {
		return commands.Reply{Text: fmt.Sprintf("Timer messages can be up to %d characters.", maxMessage)}, nil
	}

	pending, err := t.jobs.Pending(ctx, Kind, e.Connection.ID)
	if err != nil {
		return commands.Reply{Text: "There was an error saving the timer. Please try again."}, err
	}
	if len(pending) >= maxTimers {
		return commands.Reply{Text: fmt.Sprintf("This channel already has %d timers. Remove one first (%stimer list).", len(pending), e.Prefix)}, nil
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
		return commands.Reply{Text: "There was an error saving the timer. Please try again."}, err
	}
	return commands.Reply{Text: fmt.Sprintf("Timer #%d added: %s every %s, while chat is active (%d+ messages since its last post).",
		id, quote(text), commands.FormatDuration(d), MinMessages)}, nil
}

func (t *Timer) list(ctx context.Context, e commands.Request) (commands.Reply, error) {
	pending, err := t.jobs.Pending(ctx, Kind, e.Connection.ID)
	if err != nil {
		return commands.Reply{Text: "There was an error reading the timers."}, err
	}
	if len(pending) == 0 {
		return commands.Reply{Text: fmt.Sprintf("This channel has no timers. Add one with %stimer add <every> <message>.", e.Prefix)}, nil
	}

	head := fmt.Sprintf("Timers (%d): ", len(pending))
	tail := fmt.Sprintf(" Remove one with %stimer remove <number>.", e.Prefix)
	var parts []string
	for i, j := range pending {
		var m scheduler.Message
		if err := json.Unmarshal(j.Payload, &m); err != nil {
			return commands.Reply{Text: "There was an error reading the timers."}, fmt.Errorf("timer %d: decode payload: %w", j.ID, err)
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

func (t *Timer) remove(ctx context.Context, e commands.Request, arg string) (commands.Reply, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64)
	if err != nil {
		return commands.Reply{Text: fmt.Sprintf("Timers have numbers like #3; see %stimer list.", e.Prefix)}, nil
	}
	if err := t.jobs.Cancel(ctx, id, e.Connection.ID, Kind); err != nil {
		if errors.Is(err, scheduler.ErrNotFound) {
			return commands.Reply{Text: fmt.Sprintf("This channel has no timer #%d; see %stimer list.", id, e.Prefix)}, nil
		}
		return commands.Reply{Text: "There was an error removing the timer."}, err
	}
	return commands.Reply{Text: fmt.Sprintf("Timer #%d removed.", id)}, nil
}

// quote shows a timer's message in quotes, cut short if it's long.
func quote(text string) string {
	if r := []rune(text); len(r) > maxQuoted {
		text = strings.TrimSpace(string(r[:maxQuoted-1])) + "…"
	}
	return `"` + text + `"`
}
