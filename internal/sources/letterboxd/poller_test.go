package letterboxd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/database"
)

type recorder []core.Outbound

func (r *recorder) Publish(ctx context.Context, out core.Outbound) error {
	*r = append(*r, out)
	return nil
}

// TestPoller needs a Postgres set up by scripts/test-db.sh.
func TestPoller(t *testing.T) {
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

	// The feed starts with only Alien and Jaws; Heat is added later.
	var withHeat atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		feed := sampleFeed
		if !withHeat.Load() {
			start := strings.Index(feed, "<item>")
			end := strings.Index(feed, "</item>") + len("</item>")
			feed = feed[:start] + feed[end:]
		}
		w.Write([]byte(feed))
	}))
	t.Cleanup(srv.Close)
	feedURL := srv.URL + "/pltest/rss/"

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DELETE FROM lys.letterboxd_feed_subscriptions WHERE user_id = 'pltest-u'`)
	exec(`DELETE FROM lys.communities WHERE name = 'PollerTest'`)
	exec(`DELETE FROM lys.platform_users WHERE external_id = 'pltest-u'`)
	var community, conn, user int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('PollerTest') RETURNING id`).Scan(&community); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_connections (community_id, platform, external_id, name) VALUES ($1, 'twitch', 'pltest-ch', 'PollerTest') RETURNING id`, community).Scan(&conn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_users (platform, external_id, display_name) VALUES ('twitch', 'pltest-u', 'Viewer') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO lys.feed_subscriptions (kind, feed_url, feed_name, connection_id, platform_user_id) VALUES ('letterboxd', $1, 'pltest', $2, $3)`, feedURL, conn, user)
	exec(`INSERT INTO lys.letterboxd_feed_subscriptions (username, user_id, letterboxd_username, feed_url, alert_channels) VALUES ('Viewer', 'pltest-u', 'pltest', $1, ARRAY['twitch:pltest-ch'])`, feedURL)

	p := New(pool, srv.Client())
	p.notifyGap = 0
	pub := &recorder{}
	poll := func() {
		t.Helper()
		subs, err := p.subscriptions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range subs {
			if s.feedURL == feedURL {
				if err := p.check(ctx, s, pub); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	poll()
	if len(*pub) != 1 || !strings.HasPrefix((*pub)[0].Text, "🎬 Viewer watched Alien") || (*pub)[0].Channel.ID != "pltest-ch" {
		t.Fatalf("first check: %+v", *pub)
	}
	var newSeen, oldSeen string
	if err := pool.QueryRow(ctx, `SELECT f.last_seen_guid, l.last_seen_guid FROM lys.feed_subscriptions f JOIN lys.letterboxd_feed_subscriptions l USING (feed_url) WHERE f.feed_url = $1`, feedURL).Scan(&newSeen, &oldSeen); err != nil {
		t.Fatal(err)
	}
	if newSeen != "letterboxd-review-2" || oldSeen != newSeen {
		t.Fatalf("seen: new table %q, old table %q", newSeen, oldSeen)
	}

	poll()
	if len(*pub) != 1 {
		t.Fatalf("announced again: %+v", *pub)
	}

	withHeat.Store(true)
	poll()
	if len(*pub) != 2 || !strings.HasPrefix((*pub)[1].Text, "🎬 Viewer rewatched Heat") {
		t.Fatalf("new entry: %+v", *pub)
	}
}
