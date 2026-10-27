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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/sources"
)

// Poller checks every feed once per window, spread evenly across it.
type Poller struct {
	pool *pgxpool.Pool
	http *http.Client

	window    time.Duration // how long a full round of feeds takes
	minGap    time.Duration // at least this long between two feeds
	notifyGap time.Duration // between two announcements
}

var _ sources.Source = (*Poller)(nil)

func New(pool *pgxpool.Pool, http *http.Client) *Poller {
	return &Poller{pool: pool, http: http, window: 25 * time.Minute, minGap: 5 * time.Second, notifyGap: 3 * time.Second}
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

		gap := max(p.window/time.Duration(len(subs)), p.minGap)
		for _, s := range subs {
			if err := p.check(ctx, s, pub); err != nil {
				log.Printf("[Letterboxd] %q for %q: %v", s.feedName, s.channel.Name, err)
			}
			if !sleep(ctx, gap) {
				return ctx.Err()
			}
		}
	}
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

// check announces a feed's new entries, then records the newest as seen.
func (p *Poller) check(ctx context.Context, s subscription, pub sources.Publisher) error {
	movies, err := p.fetch(ctx, s.feedURL)
	if err != nil {
		return err
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

func (p *Poller) fetch(ctx context.Context, url string) ([]movie, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Letterboxd: %w", err)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Letterboxd: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Letterboxd: %s: status %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("Letterboxd: read %s: %w", url, err)
	}
	var feed rss
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("Letterboxd: %s isn't an RSS feed: %w", url, err)
	}
	return parse(feed), nil
}

// markSeen records the newest entry, here and in the old table a rollback's
// LetterboxdWatcher reads, so neither announces it again.
func (p *Poller) markSeen(ctx context.Context, s subscription, guid string) error {
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE lys.feed_subscriptions SET last_seen_guid = $2, last_alerted_at = now() WHERE id = $1`, s.id, guid); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE lys.letterboxd_feed_subscriptions SET last_seen_guid = $2, last_alerted_at = now() WHERE feed_url = $1`, s.feedURL, guid)
		return err
	})
	if err != nil {
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
