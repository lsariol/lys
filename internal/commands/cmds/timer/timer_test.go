package timer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/scheduler"
)

type stubJobs struct {
	jobs   []scheduler.Job
	nextID int64
}

func (s *stubJobs) Schedule(ctx context.Context, j scheduler.NewJob) (int64, error) {
	s.nextID++
	s.jobs = append(s.jobs, scheduler.Job{ID: s.nextID, Kind: j.Kind, ConnectionID: j.ConnectionID, RunAt: j.RunAt, RepeatEvery: j.RepeatEvery})
	return s.nextID, nil
}

func (s *stubJobs) Pending(ctx context.Context, kind string, connectionID int64) ([]scheduler.Job, error) {
	var out []scheduler.Job
	for _, j := range s.jobs {
		if j.Kind == kind && j.ConnectionID == connectionID {
			out = append(out, j)
		}
	}
	return out, nil
}

func (s *stubJobs) Cancel(ctx context.Context, id, connectionID int64, kind string) error {
	for i, j := range s.jobs {
		if j.ID == id && j.ConnectionID == connectionID {
			s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
			return nil
		}
	}
	return scheduler.ErrNotFound
}

func TestTimer(t *testing.T) {
	jobs := &stubJobs{}
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	tm := New(jobs)
	tm.now = func() time.Time { return now }

	run := func(roles core.Roles, args ...string) string {
		reply, err := tm.Run(context.Background(), commands.Request{
			Message:    core.Message{Platform: "twitch", Channel: core.Channel{ID: "100"}, User: core.User{ID: "5", Roles: roles}},
			Connection: core.Connection{ID: 9},
			Prefix:     "!",
			Args:       args,
		})
		if err != nil {
			t.Fatal(err)
		}
		return reply.Text
	}

	if got := run(0, "add", "30m", "hi"); !strings.Contains(got, "owner and moderators") {
		t.Fatalf("a viewer added a timer: %q", got)
	}
	if got := run(core.RoleModerator, "add", "30m", "Follow", "the", "channel!"); got != "Timer 1 posts every 30 minutes, starting in 30 minutes." {
		t.Fatalf("add: %q", got)
	}
	if j := jobs.jobs[0]; j.RepeatEvery != 30*time.Minute || !j.RunAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("job: %+v", j)
	}
	if got := run(core.RoleBroadcaster, "add", "1m", "x"); !strings.Contains(got, "every 5 minutes") {
		t.Errorf("too often: %q", got)
	}
	if got := run(core.RoleBroadcaster, "list"); got != "Timers: 1 every 30 minutes, next in 30 minutes." {
		t.Errorf("list: %q", got)
	}
	if got := run(core.RoleBroadcaster, "remove", "2"); !strings.Contains(got, "no timer 2") {
		t.Errorf("remove missing: %q", got)
	}
	if got := run(core.RoleBroadcaster, "remove", "1"); got != "Timer 1 is removed." {
		t.Errorf("remove: %q", got)
	}
}
