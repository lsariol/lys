package letterboxd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type Letterboxd struct{}

func (Letterboxd) Name() string      { return "letterboxd" }
func (Letterboxd) Aliases() []string { return []string{"lb"} }
func (Letterboxd) Regexes() []string { return nil }
func (Letterboxd) Description() string {
	return "Link your Letterboxd account so the channel sees your new reviews."
}
func (Letterboxd) Usage() string          { return "!letterboxd watch <username or url> | !letterboxd unwatch" }
func (Letterboxd) Timeout() time.Duration { return 10 * time.Second }

func (Letterboxd) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
	if len(e.Args) == 0 {
		return adapter.Response{Text: "Usage: !letterboxd watch <username or url> | !letterboxd unwatch"}, nil
	}

	switch strings.ToLower(e.Args[0]) {
	case "watch":
		if len(e.Args) < 2 {
			return adapter.Response{Text: "Usage: !letterboxd watch <username or url>"}, nil
		}
		return handleWatch(ctx, e, deps, e.Args[1])
	case "unwatch":
		return handleUnwatch(ctx, e, deps)
	default:
		return adapter.Response{Text: "Usage: !letterboxd watch <username or url> | !letterboxd unwatch"}, nil
	}
}

func handleWatch(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps, input string) (adapter.Response, error) {
	lbUsername, err := parseLetterboxdUsername(input)
	if err != nil || lbUsername == "" {
		return adapter.Response{Text: fmt.Sprintf("@%s that doesn't look like a valid Letterboxd username or URL.", e.Username)}, nil
	}

	watching, err := alreadyWatching(ctx, e.UserID, deps)
	if err != nil {
		return adapter.Response{SuppressReply: true, Error: true}, fmt.Errorf("letterboxd watch: check existing: %w", err)
	}
	if watching {
		return adapter.Response{Text: fmt.Sprintf("@%s you already have a Letterboxd account linked. Use !letterboxd unwatch first.", e.Username)}, nil
	}

	feedURL := fmt.Sprintf("https://letterboxd.com/%s/rss/", lbUsername)

	resp, err := http.Head(feedURL)
	if err != nil || resp.StatusCode == http.StatusNotFound {
		return adapter.Response{Text: fmt.Sprintf("@%s couldn't find a Letterboxd user named \"%s\".", e.Username, lbUsername)}, nil
	}

	alertChannel := fmt.Sprintf("%s:%s", e.Platform, e.ChannelID)

	if err := addSubscription(ctx, e.Username, e.UserID, lbUsername, feedURL, alertChannel, deps); err != nil {
		return adapter.Response{SuppressReply: true, Error: true}, fmt.Errorf("letterboxd watch: insert: %w", err)
	}

	return adapter.Response{Text: fmt.Sprintf("%s subscribed! New Letterboxd reviews from %s will appear in chat.", e.Username, lbUsername)}, nil
}

func handleUnwatch(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
	lbUsername, found, err := removeSubscription(ctx, e.UserID, deps)
	if err != nil {
		return adapter.Response{SuppressReply: true, Error: true}, fmt.Errorf("letterboxd unwatch: delete: %w", err)
	}
	if !found {
		return adapter.Response{Text: fmt.Sprintf("@%s you don't have a Letterboxd account linked.", e.Username)}, nil
	}

	return adapter.Response{Text: fmt.Sprintf("@%s removed. No longer watching %s on Letterboxd.", e.Username, lbUsername)}, nil
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
