package scheduler

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/database"
	"github.com/lsariol/lys/internal/sources"
)

type recorder struct {
	mu   sync.Mutex
	sent []core.Outbound
}

func (r *recorder) Publish(ctx context.Context, out core.Outbound) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, out)
	return nil
}

func (r *recorder) texts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, o := range r.sent {
		out = append(out, o.Text)
	}
	return out
}

// open needs a Postgres set up by scripts/test-db.sh. It returns a store and
// two fresh connections (channels), the second blocked.
func open(t *testing.T) (*Store, *pgxpool.Pool, int64, int64) {
	t.Helper()
	migratorURL, appURL := os.Getenv("LYS_TEST_MIGRATOR_DATABASE_URL"), os.Getenv("LYS_TEST_DATABASE_URL")
	if migratorURL == "" || appURL == "" {
		t.Skip("LYS_TEST_MIGRATOR_DATABASE_URL and LYS_TEST_DATABASE_URL aren't set (see scripts/test-db.sh)")
	}
	ctx := context.Background()
	if err := database.Migrate(ctx, migratorURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DELETE FROM lys.communities WHERE name LIKE 'SchedTest%'`); err != nil {
		t.Fatal(err)
	}

	conn := func(name string, blocked bool) int64 {
		var community, id int64
		if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ($1) RETURNING id`, name).Scan(&community); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_connections (community_id, platform, external_id, name, admin_blocked)
			VALUES ($1, 'fake', $2, $2, $3) RETURNING id`, community, name, blocked).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	return NewStore(pool), pool, conn("SchedTest-A", false), conn("SchedTest-B", true)
}

func TestScheduler(t *testing.T) {
	store, pool, active, blocked := open(t)
	ctx := context.Background()

	schedule := func(kind string, conn int64, text string, at time.Time, repeat time.Duration) int64 {
		t.Helper()
		id, err := store.Schedule(ctx, NewJob{Kind: kind, ConnectionID: conn, Payload: Message{Text: text}, RunAt: at, RepeatEvery: repeat})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	past, future := time.Now().Add(-time.Minute), time.Now().Add(time.Hour)
	once := schedule("message", active, "once", past, 0)
	repeating := schedule("message", active, "every 10m", past.Add(-25*time.Minute), 10*time.Minute)
	later := schedule("message", active, "later", future, 0)
	skipped := schedule("message", blocked, "blocked channel", past, 0)
	failing := schedule("fails", active, "", past, 0)

	pub := &recorder{}
	s := New(store, map[string]Handler{
		"message": SendMessage,
		"fails":   func(context.Context, Job, sources.Publisher) error { return errors.New("broken") },
	})
	s.runDue(ctx, pub)

	got := map[string]int{}
	for _, text := range pub.texts() {
		got[text]++
	}
	if got["once"] != 1 || got["every 10m"] != 1 || got["later"] != 0 || got["blocked channel"] != 0 {
		t.Fatalf("sent: %v", got)
	}

	state := func(id int64) (done bool, runAt time.Time, attempts int, lastErr string) {
		t.Helper()
		var errText *string
		if err := pool.QueryRow(ctx, `SELECT done_at IS NOT NULL, run_at, attempts, last_error FROM lys.scheduled_jobs WHERE id = $1`, id).
			Scan(&done, &runAt, &attempts, &errText); err != nil {
			t.Fatal(err)
		}
		if errText != nil {
			lastErr = *errText
		}
		return
	}
	if done, _, _, _ := state(once); !done {
		t.Error("the one-off job isn't done")
	}
	if done, runAt, _, _ := state(repeating); done || !runAt.After(time.Now()) || runAt.After(time.Now().Add(10*time.Minute)) {
		t.Errorf("repeating job: done %v, next run %s", done, runAt)
	}
	if done, _, _, lastErr := state(skipped); !done || lastErr == "" {
		t.Errorf("blocked channel's job: done %v, error %q", done, lastErr)
	}
	if done, runAt, attempts, lastErr := state(failing); done || attempts != 1 || lastErr != "broken" || !runAt.After(time.Now()) {
		t.Errorf("failing job: done %v attempts %d error %q next %s", done, attempts, lastErr, runAt)
	}

	// A second pass finds nothing due.
	s.runDue(ctx, pub)
	if n := len(pub.texts()); n != 2 {
		t.Fatalf("second pass sent more: %v", pub.texts())
	}

	pending, err := store.Pending(ctx, "message", active)
	if err != nil || len(pending) != 2 || pending[0].ID != repeating || pending[1].ID != later {
		t.Fatalf("pending: %+v, %v", pending, err)
	}
	if err := store.Cancel(ctx, later, blocked, "message"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel from another channel: %v", err)
	}
	if err := store.Cancel(ctx, later, active, "message"); err != nil {
		t.Fatal(err)
	}
}

func TestClaimIsExclusive(t *testing.T) {
	store, _, active, _ := open(t)
	ctx := context.Background()
	for range 10 {
		if _, err := store.Schedule(ctx, NewJob{Kind: "claimtest", ConnectionID: active, Payload: Message{}, RunAt: time.Now().Add(-time.Second)}); err != nil {
			t.Fatal(err)
		}
	}

	// Two claimers at once never get the same job.
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[int64]int{}
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				jobs, err := store.Claim(ctx, 3, time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				for _, j := range jobs {
					if j.Kind == "claimtest" {
						seen[j.ID]++
					}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != 10 {
		t.Fatalf("claimed %d of 10 jobs", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("job %d was claimed %d times", id, n)
		}
	}
}
