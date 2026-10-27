package timer

import (
	"context"
	"encoding/json"
	"fmt"
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
	payload, err := json.Marshal(j.Payload)
	if err != nil {
		return 0, err
	}
	s.nextID++
	s.jobs = append(s.jobs, scheduler.Job{ID: s.nextID, Kind: j.Kind, ConnectionID: j.ConnectionID, Payload: payload, RunAt: j.RunAt, RepeatEvery: j.RepeatEvery})
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
	if reply, _ := tm.Run(context.Background(), commands.Request{
		Message:    core.Message{Platform: "twitch", Channel: core.Channel{ID: "100"}, User: core.User{ID: "42217464"}},
		Connection: core.Connection{ID: 9}, Prefix: "!", Args: []string{"add", "30m", "hi"},
	}); !strings.Contains(reply.Text, "owner and moderators") {
		t.Fatalf("the bot's owner added a timer in someone else's channel: %q", reply.Text)
	}
	if got := run(core.RoleModerator, "list"); got != "This channel has no timers. Add one with !timer add <every> <message>." {
		t.Errorf("empty list: %q", got)
	}
	if got, want := run(core.RoleModerator, "add", "30m", "Follow", "the", "channel!"), `Timer #1 added: "Follow the channel!" every 30 minutes, while chat is active (5+ messages since its last post).`; got != want {
		t.Fatalf("add: %q", got)
	}
	if j := jobs.jobs[0]; j.RepeatEvery != 30*time.Minute || !j.RunAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("job: %+v", j)
	}
	if got := run(core.RoleBroadcaster, "add", "1m", "x"); !strings.Contains(got, "every 5 minutes") {
		t.Errorf("too often: %q", got)
	}
	if got, want := run(core.RoleBroadcaster, "list"), `Timers (1): #1 "Follow the channel!" every 30 minutes, next in 30 minutes. Remove one with !timer remove <number>.`; got != want {
		t.Errorf("list: %q", got)
	}
	if got := run(core.RoleBroadcaster, "remove", "2"); got != "This channel has no timer #2; see !timer list." {
		t.Errorf("remove missing: %q", got)
	}
	if got := run(core.RoleBroadcaster, "remove", "#1"); got != "Timer #1 removed." {
		t.Errorf("remove: %q", got)
	}
}

func TestListFitsInOneMessage(t *testing.T) {
	jobs := &stubJobs{}
	tm := New(jobs)
	req := commands.Request{
		Message:    core.Message{Channel: core.Channel{ID: "100"}, User: core.User{Roles: core.RoleBroadcaster}},
		Connection: core.Connection{ID: 9},
		Prefix:     "!",
	}
	for i := range maxTimers {
		req.Args = []string{"add", "2h", fmt.Sprintf("Timer number %d has a long message that goes on and on", i)}
		if _, err := tm.Run(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	req.Args = []string{"list"}
	reply, err := tm.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Text) > maxReply || !strings.Contains(reply.Text, "more.") || !strings.HasPrefix(reply.Text, "Timers (10): #1 ") {
		t.Fatalf("%d characters: %q", len(reply.Text), reply.Text)
	}
}

type stubActivity int

func (s stubActivity) Since(platform, channelID string, after time.Time) int { return int(s) }

type recorder []core.Outbound

func (r *recorder) Publish(ctx context.Context, out core.Outbound) error {
	*r = append(*r, out)
	return nil
}

func TestPostNeedsActiveChat(t *testing.T) {
	job := scheduler.Job{Kind: Kind, Platform: "twitch", Channel: core.Channel{ID: "100"}, Payload: json.RawMessage(`{"text": "hello"}`),
		RunAt: time.Now(), RepeatEvery: 30 * time.Minute}

	var quiet recorder
	if err := Post(stubActivity(MinMessages-1))(context.Background(), job, &quiet); err != nil || len(quiet) != 0 {
		t.Fatalf("quiet chat: %v %v", quiet, err)
	}
	var active recorder
	if err := Post(stubActivity(MinMessages))(context.Background(), job, &active); err != nil || len(active) != 1 || active[0].Text != "hello" {
		t.Fatalf("active chat: %v %v", active, err)
	}
}
