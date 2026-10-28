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

	// The feed starts with only Alien and Jaws; Heat is added later. It answers
	// a request for the version the poller already has with 304.
	var withHeat atomic.Bool
	var fetches, unchanged atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		feed, etag := sampleFeed, `"with-heat"`
		if !withHeat.Load() {
			start := strings.Index(feed, "<item>")
			end := strings.Index(feed, "</item>") + len("</item>")
			feed, etag = feed[:start]+feed[end:], `"without-heat"`
		}
		if r.Header.Get("If-None-Match") == etag {
			unchanged.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
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
	exec(`DELETE FROM lys.communities WHERE name IN ('PollerTest', 'PollerTest2')`)
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
	// The same feed, followed in a second channel.
	var community2, conn2 int64
	if err := pool.QueryRow(ctx, `INSERT INTO lys.communities (name) VALUES ('PollerTest2') RETURNING id`).Scan(&community2); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lys.platform_connections (community_id, platform, external_id, name) VALUES ($1, 'twitch', 'pltest-ch2', 'PollerTest2') RETURNING id`, community2).Scan(&conn2); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO lys.feed_subscriptions (kind, feed_url, feed_name, connection_id, platform_user_id) VALUES ('letterboxd', $1, 'pltest', $2, $3)`, feedURL, conn2, user)

	p := New(pool, srv.Client())
	p.notifyGap = 0
	pub := &recorder{}
	poll := func() {
		t.Helper()
		subs, err := p.subscriptions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range group(subs) {
			if f.url == feedURL {
				if len(f.subs) != 2 {
					t.Fatalf("the feed's subscriptions weren't grouped: %d", len(f.subs))
				}
				p.checkFeed(ctx, f, pub)
			}
		}
	}

	poll()
	if len(*pub) != 2 || !strings.HasPrefix((*pub)[0].Text, "🎬 Viewer watched Alien") || (*pub)[0].Channel.ID == (*pub)[1].Channel.ID {
		t.Fatalf("first check: %+v", *pub)
	}
	if fetches.Load() != 1 {
		t.Fatalf("a feed two channels follow was fetched %d times", fetches.Load())
	}
	var seen string
	if err := pool.QueryRow(ctx, `SELECT last_seen_guid FROM lys.feed_subscriptions WHERE feed_url = $1`, feedURL).Scan(&seen); err != nil || seen != "letterboxd-review-2" {
		t.Fatalf("seen: %q, %v", seen, err)
	}

	poll()
	if len(*pub) != 2 || unchanged.Load() != 1 {
		t.Fatalf("an unchanged feed: announced %d, %d not-modified answers", len(*pub), unchanged.Load())
	}

	withHeat.Store(true)
	poll()
	if len(*pub) != 4 || !strings.HasPrefix((*pub)[2].Text, "🎬 Viewer rewatched Heat") {
		t.Fatalf("new entry: %+v", *pub)
	}
}
