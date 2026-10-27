package timer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/scheduler"
)

// Kind is the scheduled_jobs kind of a channel timer.
const Kind = "timer"

const (
	minEvery   = 5 * time.Minute
	maxEvery   = 24 * time.Hour
	maxTimers  = 10 // per channel
	maxMessage = 450
)

// Jobs is the part of the scheduler's store timers use.
type Jobs interface {
	Schedule(ctx context.Context, j scheduler.NewJob) (int64, error)
	Pending(ctx context.Context, kind string, connectionID int64) ([]scheduler.Job, error)
	Cancel(ctx context.Context, id, connectionID int64, kind string) error
}

// Timer posts a channel's message on repeat. The channel's owner and
// moderators manage them.
type Timer struct {
	jobs Jobs
	now  func() time.Time
}

func New(jobs Jobs) *Timer { return &Timer{jobs: jobs, now: time.Now} }

func (*Timer) Spec() commands.Spec {
	return commands.Spec{
		Name:        "timer",
		Aliases:     []string{"timers"},
		Description: "Post a message in this channel on repeat (owner and moderators).",
		Usage:       "!timer add <every, like 30m or 2h> <message> | !timer list | !timer remove <id>",
		Timeout:     5 * time.Second,
	}
}

func (t *Timer) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	usage := fmt.Sprintf("Usage: %stimer add <every, like 30m> <message>, %stimer list, or %stimer remove <id>.", e.Prefix, e.Prefix, e.Prefix)
	if len(e.Args) == 0 {
		return commands.Reply{Text: usage}, nil
	}
	if !commands.HasPrivilege(e) && !e.User.Roles.Has(core.RoleModerator) {
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
	return commands.Reply{Text: fmt.Sprintf("Timer %d posts every %s, starting in %s.", id, commands.FormatDuration(d), commands.FormatDuration(d))}, nil
}

func (t *Timer) list(ctx context.Context, e commands.Request) (commands.Reply, error) {
	pending, err := t.jobs.Pending(ctx, Kind, e.Connection.ID)
	if err != nil {
		return commands.Reply{Text: "There was an error reading the timers."}, err
	}
	if len(pending) == 0 {
		return commands.Reply{Text: "This channel has no timers."}, nil
	}
	var parts []string
	for _, j := range pending {
		parts = append(parts, fmt.Sprintf("%d every %s, next in %s", j.ID, commands.FormatDuration(j.RepeatEvery), commands.FormatDuration(j.RunAt.Sub(t.now()))))
	}
	return commands.Reply{Text: "Timers: " + strings.Join(parts, " | ") + "."}, nil
}

func (t *Timer) remove(ctx context.Context, e commands.Request, arg string) (commands.Reply, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64)
	if err != nil {
		return commands.Reply{Text: fmt.Sprintf("Timer IDs are numbers; see %stimer list.", e.Prefix)}, nil
	}
	if err := t.jobs.Cancel(ctx, id, e.Connection.ID, Kind); err != nil {
		if errors.Is(err, scheduler.ErrNotFound) {
			return commands.Reply{Text: fmt.Sprintf("This channel has no timer %d.", id)}, nil
		}
		return commands.Reply{Text: "There was an error removing the timer."}, err
	}
	return commands.Reply{Text: fmt.Sprintf("Timer %d is removed.", id)}, nil
}
