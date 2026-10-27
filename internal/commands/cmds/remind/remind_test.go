package remind

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/scheduler"
)

// stubJobs keeps jobs in memory.
type stubJobs struct {
	jobs   []scheduler.Job
	nextID int64
}

func (s *stubJobs) Schedule(ctx context.Context, j scheduler.NewJob) (int64, error) {
	s.nextID++
	s.jobs = append(s.jobs, scheduler.Job{ID: s.nextID, Kind: j.Kind, ConnectionID: j.ConnectionID, CreatedBy: j.CreatedBy, RunAt: j.RunAt,
		Payload: []byte(j.Payload.(scheduler.Message).Text)})
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

func (s *stubJobs) CountByCreator(ctx context.Context, kind string, createdBy int64) (int, error) {
	n := 0
	for _, j := range s.jobs {
		if j.Kind == kind && j.CreatedBy == createdBy {
			n++
		}
	}
	return n, nil
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

func TestRemind(t *testing.T) {
	jobs := &stubJobs{}
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	r := New(jobs)
	r.now = func() time.Time { return now }

	run := func(user int64, args ...string) string {
		reply, err := r.Run(context.Background(), commands.Request{
			Message:    core.Message{Platform: "twitch", User: core.User{Name: "Viewer"}},
			Identity:   core.Identity{PlatformUserID: user},
			Connection: core.Connection{ID: 9},
			Prefix:     "!",
			Args:       args,
		})
		if err != nil {
			t.Fatal(err)
		}
		return reply.Text
	}

	if got := run(1, "in", "1h30m", "stretch", "now"); got != "@Viewer I'll remind you in 1 hour 30 minutes (reminder 1)." {
		t.Fatalf("set: %q", got)
	}
	if j := jobs.jobs[0]; !j.RunAt.Equal(now.Add(90*time.Minute)) || string(j.Payload) != "⏰ @Viewer reminder: stretch now" || j.CreatedBy != 1 || j.ConnectionID != 9 {
		t.Fatalf("job: %+v %q", j, j.Payload)
	}
	for _, args := range [][]string{{"10s", "x"}, {"31d", "x"}} {
		if got := run(1, args...); !strings.Contains(got, "1 minute to 30 days") {
			t.Errorf("%v: %q", args, got)
		}
	}
	if got := run(1, "soon", "x"); !strings.Contains(got, "Usage") {
		t.Errorf("bad time: %q", got)
	}
	if got := run(1, "list"); got != "@Viewer your reminders: 1 in 1 hour 30 minutes." {
		t.Errorf("list: %q", got)
	}
	if got := run(2, "cancel", "1"); !strings.Contains(got, "no reminder 1") {
		t.Errorf("someone else cancelled it: %q", got)
	}
	if got := run(1, "cancel", "#1"); !strings.Contains(got, "cancelled") || len(jobs.jobs) != 0 {
		t.Errorf("cancel: %q", got)
	}

	for range maxEach {
		run(3, "5m", "x")
	}
	if got := run(3, "5m", "x"); !strings.Contains(got, "already have 10") {
		t.Errorf("limit: %q", got)
	}
}
