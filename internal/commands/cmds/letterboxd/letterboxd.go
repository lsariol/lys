package letterboxd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// Letterboxd links a viewer's Letterboxd account, so the channel sees their new reviews.
type Letterboxd struct {
	store Store
	http  *http.Client
}

func New(store Store, http *http.Client) *Letterboxd {
	return &Letterboxd{store: store, http: http}
}

func (*Letterboxd) Spec() commands.Spec {
	return commands.Spec{
		Name:        "letterboxd",
		Aliases:     []string{"lb"},
		Description: "Link your Letterboxd account so the channel sees your new reviews.",
		Usage:       "!letterboxd watch <username or url> | !letterboxd unwatch",
		Timeout:     10 * time.Second,
		Concurrency: 16,
	}
}

func (l *Letterboxd) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if len(e.Args) == 0 {
		return commands.Reply{Text: "Usage: !letterboxd watch <username or url> | !letterboxd unwatch"}, nil
	}

	switch strings.ToLower(e.Args[0]) {
	case "watch":
		if len(e.Args) < 2 {
			return commands.Reply{Text: "Usage: !letterboxd watch <username or url>"}, nil
		}
		return l.handleWatch(ctx, e, e.Args[1])
	case "unwatch":
		return l.handleUnwatch(ctx, e)
	default:
		return commands.Reply{Text: "Usage: !letterboxd watch <username or url> | !letterboxd unwatch"}, nil
	}
}

func (l *Letterboxd) handleWatch(ctx context.Context, e commands.Request, input string) (commands.Reply, error) {
	lbUsername, err := parseLetterboxdUsername(input)
	if err != nil || lbUsername == "" {
		return commands.Reply{Text: fmt.Sprintf("@%s that doesn't look like a valid Letterboxd username or URL.", e.User.Name)}, nil
	}

	if e.Identity.PlatformUserID == 0 || e.Connection.ID == 0 {
		return commands.Reply{}, fmt.Errorf("letterboxd watch: %q in %q has no identity or channel in Lys", e.User.Name, e.Channel.Name)
	}

	watching, err := l.store.AlreadyWatching(ctx, e.Identity.PlatformUserID)
	if err != nil {
		return commands.Reply{}, fmt.Errorf("letterboxd watch: check existing: %w", err)
	}
	if watching {
		return commands.Reply{Text: fmt.Sprintf("@%s you already have a Letterboxd account linked. Use !letterboxd unwatch first.", e.User.Name)}, nil
	}

	feedURL := fmt.Sprintf("https://letterboxd.com/%s/rss/", lbUsername)

	if !l.feedExists(ctx, feedURL) {
		return commands.Reply{Text: fmt.Sprintf("@%s couldn't find a Letterboxd user named \"%s\".", e.User.Name, lbUsername)}, nil
	}

	alertChannel := fmt.Sprintf("%s:%s", e.Platform, e.Channel.ID)

	if err := l.store.AddSubscription(ctx, Subscription{
		PlatformUserID: e.Identity.PlatformUserID,
		ConnectionID:   e.Connection.ID,
		FeedURL:        feedURL,
		LetterboxdName: lbUsername,
		Username:       e.User.Name,
		UserID:         e.User.ID,
		AlertChannel:   alertChannel,
	}); err != nil {
		return commands.Reply{}, fmt.Errorf("letterboxd watch: insert: %w", err)
	}

	return commands.Reply{Text: fmt.Sprintf("%s subscribed! New Letterboxd reviews from %s will appear in chat.", e.User.Name, lbUsername)}, nil
}

// feedExists reports whether Letterboxd answers for the feed (anything but a
// failed request or a 404).
func (l *Letterboxd) feedExists(ctx context.Context, feedURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, feedURL, nil)
	if err != nil {
		return false
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode != http.StatusNotFound
}

func (l *Letterboxd) handleUnwatch(ctx context.Context, e commands.Request) (commands.Reply, error) {
	lbUsername, found, err := l.store.RemoveSubscription(ctx, e.Identity.PlatformUserID, e.User.ID)
	if err != nil {
		return commands.Reply{}, fmt.Errorf("letterboxd unwatch: delete: %w", err)
	}
	if !found {
		return commands.Reply{Text: fmt.Sprintf("@%s you don't have a Letterboxd account linked.", e.User.Name)}, nil
	}

	return commands.Reply{Text: fmt.Sprintf("@%s removed. No longer watching %s on Letterboxd.", e.User.Name, lbUsername)}, nil
}

func parseLetterboxdUsername(input string) (string, error) {
	if strings.HasPrefix(input, "http") {
		u, err := url.Parse(input)
		if err != nil {
			return "", err
		}
		return strings.Trim(u.Path, "/"), nil
	}
	return input, nil
}
