// Package letterboxd is the source that announces new Letterboxd diary
// entries from the feeds viewers subscribed with !letterboxd watch. It
// replaces LetterboxdWatcher.
package letterboxd

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/sources"
)

// Poller checks every feed once per window, spread evenly across it. A feed
// several channels follow is fetched once for all of them.
type Poller struct {
	pool *pgxpool.Pool
	http *http.Client

	window    time.Duration // how long a full round of feeds takes
	minGap    time.Duration // at least this long between two fetches starting
	notifyGap time.Duration // between two announcements

	mu         sync.Mutex
	validators map[string]validators // by feed URL, for conditional requests
}

// fetchAtOnce is how many feeds may be fetched at the same time.
const fetchAtOnce = 4

// validators are what Letterboxd sent to tell an unchanged feed by.
type validators struct{ etag, lastModified string }

var _ sources.Source = (*Poller)(nil)

func New(pool *pgxpool.Pool, http *http.Client) *Poller {
	return &Poller{pool: pool, http: http, window: 25 * time.Minute, minGap: 1250 * time.Millisecond, notifyGap: 3 * time.Second,
		validators: make(map[string]validators)}
}

func (p *Poller) Name() string { return "letterboxd" }

// subscription is one feed and the channel it's announced in.
type subscription struct {
	id          int64
	feedURL     string
	feedName    string
	lastSeen    string
	lastAlerted *time.Time
	subscriber  string
	platform    string
	channel     core.Channel
}

func (p *Poller) Run(ctx context.Context, pub sources.Publisher) error {
	for {
		subs, err := p.subscriptions(ctx)
		if err != nil {
			log.Printf("[Letterboxd] %v; trying again in %s", err, p.window)
		}
		if err != nil || len(subs) == 0 {
			if !sleep(ctx, p.window) {
				return ctx.Err()
			}
			continue
		}

		feeds := group(subs)
		gap := max(p.window/time.Duration(len(feeds)), p.minGap)
		slots := make(chan struct{}, fetchAtOnce)
		var wg sync.WaitGroup
		for _, f := range feeds {
			slots <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-slots }()
				p.checkFeed(ctx, f, pub)
			}()
			if !sleep(ctx, gap) {
				wg.Wait()
				return ctx.Err()
			}
		}
		wg.Wait()
	}
}

// feed is one feed URL and every subscription to it.
type feed struct {
	url  string
	subs []subscription
}

// group puts subscriptions to the same feed together, in the order they came.
func group(subs []subscription) []feed {
	var feeds []feed
	at := make(map[string]int)
	for _, s := range subs {
		i, ok := at[s.feedURL]
		if !ok {
			i = len(feeds)
			at[s.feedURL] = i
			feeds = append(feeds, feed{url: s.feedURL})
		}
		feeds[i].subs = append(feeds[i].subs, s)
	}
	return feeds
}

// checkFeed fetches a feed once and announces its new entries to each
// subscriber. The feed counts as unchanged next time only if all of that worked,
// so a failed announcement is tried again.
func (p *Poller) checkFeed(ctx context.Context, f feed, pub sources.Publisher) {
	movies, v, err := p.fetch(ctx, f.url)
	if err != nil {
		log.Printf("[Letterboxd] %q: %v", f.subs[0].feedName, err)
		return
	}
	ok := true
	for _, s := range f.subs {
		if err := p.announce(ctx, s, movies, pub); err != nil {
			log.Printf("[Letterboxd] %q for %q: %v", s.feedName, s.channel.Name, err)
			ok = false
		}
	}
	p.mu.Lock()
	if ok && movies != nil {
		p.validators[f.url] = v
	} else if !ok {
		delete(p.validators, f.url)
	}
	p.mu.Unlock()
}

func (p *Poller) subscriptions(ctx context.Context) ([]subscription, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT f.id, f.feed_url, f.feed_name, COALESCE(f.last_seen_guid, ''), f.last_alerted_at,
		       COALESCE(pu.display_name, f.feed_name), pc.platform, pc.external_id, pc.name
		FROM lys.feed_subscriptions f
		JOIN lys.platform_connections pc ON pc.id = f.connection_id
		LEFT JOIN lys.platform_users pu ON pu.id = f.platform_user_id
		WHERE f.kind = 'letterboxd' AND pc.authorized AND pc.enabled AND NOT pc.admin_blocked
		ORDER BY f.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("database: read Letterboxd subscriptions: %w", err)
	}
	subs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (subscription, error) {
		var s subscription
		err := r.Scan(&s.id, &s.feedURL, &s.feedName, &s.lastSeen, &s.lastAlerted, &s.subscriber, &s.platform, &s.channel.ID, &s.channel.Name)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("database: read Letterboxd subscriptions: %w", err)
	}
	return subs, nil
}

// announce posts the feed's entries this subscription hasn't seen, then
// records the newest as seen. No movies (an unchanged feed) announces nothing.
func (p *Poller) announce(ctx context.Context, s subscription, movies []movie, pub sources.Publisher) error {
	if len(movies) == 0 {
		return nil
	}
	fresh := newMovies(movies, s.lastSeen, s.lastAlerted)
	if len(fresh) == 0 {
		return nil
	}

	profile := strings.TrimSuffix(s.feedURL, "rss/")
	if err := pub.Publish(ctx, core.Outbound{Platform: s.platform, Channel: s.channel, Text: message(s.subscriber, profile, fresh)}); err != nil {
		return err
	}
	if !sleep(ctx, p.notifyGap) {
		return ctx.Err()
	}
	return p.markSeen(ctx, s, movies[0].GUID)
}

// fetch reads a feed's entries, newest first, and what tells this version of
// it apart. It asks only for a feed that changed since the last fetch; an
// unchanged one returns no movies.
func (p *Poller) fetch(ctx context.Context, url string) ([]movie, validators, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, validators{}, fmt.Errorf("Letterboxd: %w", err)
	}
	p.mu.Lock()
	v := p.validators[url]
	p.mu.Unlock()
	if v.etag != "" {
		req.Header.Set("If-None-Match", v.etag)
	}
	if v.lastModified != "" {
		req.Header.Set("If-Modified-Since", v.lastModified)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, validators{}, fmt.Errorf("Letterboxd: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, v, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, validators{}, fmt.Errorf("Letterboxd: %s: status %s", url, resp.Status)
	}
	v = validators{etag: resp.Header.Get("ETag"), lastModified: resp.Header.Get("Last-Modified")}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, validators{}, fmt.Errorf("Letterboxd: read %s: %w", url, err)
	}
	var feed rss
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, validators{}, fmt.Errorf("Letterboxd: %s isn't an RSS feed: %w", url, err)
	}
	return parse(feed), v, nil
}

// markSeen records the newest entry, so it isn't announced again.
func (p *Poller) markSeen(ctx context.Context, s subscription, guid string) error {
	if _, err := p.pool.Exec(ctx, `UPDATE lys.feed_subscriptions SET last_seen_guid = $2, last_alerted_at = now() WHERE id = $1`, s.id, guid); err != nil {
		return fmt.Errorf("database: record what was announced: %w", err)
	}
	return nil
}

// sleep waits d, or returns false if ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
