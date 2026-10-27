package remind

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/scheduler"
)

// Kind is the scheduled_jobs kind of a reminder.
const Kind = "reminder"

const (
	minDelay = time.Minute
	maxDelay = 30 * 24 * time.Hour
	maxEach  = 10 // pending reminders per person
)

// Jobs is the part of the scheduler's store reminders use.
type Jobs interface {
	Schedule(ctx context.Context, j scheduler.NewJob) (int64, error)
	Pending(ctx context.Context, kind string, connectionID int64) ([]scheduler.Job, error)
	CountByCreator(ctx context.Context, kind string, createdBy int64) (int, error)
	Cancel(ctx context.Context, id, connectionID int64, kind string) error
}

// Remind posts a message for someone in the same channel later.
type Remind struct {
	jobs Jobs
	now  func() time.Time
}

func New(jobs Jobs) *Remind { return &Remind{jobs: jobs, now: time.Now} }

func (*Remind) Spec() commands.Spec {
	return commands.Spec{
		Name:        "remind",
		Description: "Get a reminder in this channel later.",
		Usage:       "!remind <time like 10m, 1h30m or 2d> <message> | !remind list | !remind cancel <id>",
		Timeout:     5 * time.Second,
	}
}

func (r *Remind) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	usage := fmt.Sprintf("Usage: %sremind <time like 10m or 1h30m> <message>, %sremind list, or %sremind cancel <id>.", e.Prefix, e.Prefix, e.Prefix)
	if len(e.Args) == 0 {
		return commands.Reply{Text: usage}, nil
	}
	if e.Identity.PlatformUserID == 0 || e.Connection.ID == 0 {
		return commands.Reply{Text: "Reminders aren't available right now. Try again in a moment."},
			fmt.Errorf("remind: %q in %q has no identity or channel in Lys", e.User.Name, e.Channel.Name)
	}

	switch strings.ToLower(e.Args[0]) {
	case "list":
		return r.list(ctx, e)
	case "cancel":
		if len(e.Args) != 2 {
			return commands.Reply{Text: usage}, nil
		}
		return r.cancel(ctx, e, e.Args[1])
	}

	args := e.Args
	if strings.EqualFold(args[0], "in") {
		args = args[1:]
	}
	if len(args) < 2 {
		return commands.Reply{Text: usage}, nil
	}
	delay, err := commands.ParseDuration(args[0])
	if err != nil {
		return commands.Reply{Text: usage}, nil
	}
	if delay < minDelay || delay > maxDelay {
		return commands.Reply{Text: "Reminders can be set from 1 minute to 30 days ahead."}, nil
	}

	n, err := r.jobs.CountByCreator(ctx, Kind, e.Identity.PlatformUserID)
	if err != nil {
		return commands.Reply{Text: "There was an error saving your reminder. Please try again."}, err
	}
	if n >= maxEach {
		return commands.Reply{Text: fmt.Sprintf("@%s you already have %d reminders waiting. Cancel one first (%sremind list).", e.User.Name, n, e.Prefix)}, nil
	}

	note := strings.Join(args[1:], " ")
	id, err := r.jobs.Schedule(ctx, scheduler.NewJob{
		Kind:         Kind,
		ConnectionID: e.Connection.ID,
		CreatedBy:    e.Identity.PlatformUserID,
		Payload:      scheduler.Message{Text: fmt.Sprintf("⏰ @%s reminder: %s", e.User.Name, note)},
		RunAt:        r.now().Add(delay),
	})
	if err != nil {
		return commands.Reply{Text: "There was an error saving your reminder. Please try again."}, err
	}
	return commands.Reply{Text: fmt.Sprintf("@%s I'll remind you in %s (reminder %d).", e.User.Name, commands.FormatDuration(delay), id)}, nil
}

func (r *Remind) list(ctx context.Context, e commands.Request) (commands.Reply, error) {
	pending, err := r.jobs.Pending(ctx, Kind, e.Connection.ID)
	if err != nil {
		return commands.Reply{Text: "There was an error reading your reminders."}, err
	}
	var mine []string
	for _, j := range pending {
		if j.CreatedBy == e.Identity.PlatformUserID {
			mine = append(mine, fmt.Sprintf("%d in %s", j.ID, commands.FormatDuration(j.RunAt.Sub(r.now()))))
		}
	}
	if len(mine) == 0 {
		return commands.Reply{Text: fmt.Sprintf("@%s you have no reminders waiting in this channel.", e.User.Name)}, nil
	}
	return commands.Reply{Text: fmt.Sprintf("@%s your reminders: %s.", e.User.Name, strings.Join(mine, ", "))}, nil
}

func (r *Remind) cancel(ctx context.Context, e commands.Request, arg string) (commands.Reply, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64)
	if err != nil {
		return commands.Reply{Text: fmt.Sprintf("Reminder IDs are numbers; see %sremind list.", e.Prefix)}, nil
	}

	pending, err := r.jobs.Pending(ctx, Kind, e.Connection.ID)
	if err != nil {
		return commands.Reply{Text: "There was an error cancelling your reminder."}, err
	}
	for _, j := range pending {
		if j.ID == id && j.CreatedBy == e.Identity.PlatformUserID {
			if err := r.jobs.Cancel(ctx, id, e.Connection.ID, Kind); err != nil && !errors.Is(err, scheduler.ErrNotFound) {
				return commands.Reply{Text: "There was an error cancelling your reminder."}, err
			}
			return commands.Reply{Text: fmt.Sprintf("@%s reminder %d is cancelled.", e.User.Name, id)}, nil
		}
	}
	return commands.Reply{Text: fmt.Sprintf("@%s you have no reminder %d in this channel.", e.User.Name, id)}, nil
}
