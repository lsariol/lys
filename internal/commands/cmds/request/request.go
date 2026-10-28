package request

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// Request lets viewers suggest features, and the bot's owner review them.
type Request struct {
	store Store
}

func New(store Store) *Request { return &Request{store: store} }

func (*Request) Spec() commands.Spec {
	return commands.Spec{
		Name:        "request",
		Description: "Logs a request for things to be added to the bot.",
		Usage:       "!request <feature or idea you want added>",
		Timeout:     3 * time.Second,
	}
}

// Run saves a request, or lists and updates them for the bot's owner.
func (r *Request) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	// We dont care if there is no arguments / body is empty
	if len(e.Args) == 0 {
		return commands.Reply{Text: "Usage: !request <feature or idea>"}, fmt.Errorf("incorrect command usage")
	}

	full := strings.Join(e.Args, " ")

	if reHelp.MatchString(full) {
		return helpHandler()
	}

	if m := reGet.FindStringSubmatch(full); m != nil {
		mode := m[1]
		limitStr := m[2]
		limit := 3
		if limitStr != "" {
			n, err := strconv.Atoi(limitStr)
			if err != nil || n <= 0 {
				return commands.Reply{Text: "Usage !request get <new/recent> < # to return (optional)>"}, err
			}
			//Until more is figurred out, we need to limit to 1 - CHANGE TO N WHEN READY
			limit = n
		}
		if !commands.IsBotOwner(e) {
			return commands.Reply{}, fmt.Errorf("request get: %q isn't the bot's owner", e.User.Name)
		}
		return r.getHandler(ctx, mode, limit)
	}

	if m := reSet.FindStringSubmatch(full); m != nil {
		idStr := m[1]
		status := m[2]

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return commands.Reply{Text: "Invalid request ID."}, err
		}
		if !commands.IsBotOwner(e) {
			return commands.Reply{}, fmt.Errorf("request set: %q isn't the bot's owner", e.User.Name)
		}
		return r.setHandler(ctx, id, status)

	}

	return r.createHandler(ctx, e)
}

var (
	reHelp = regexp.MustCompile(`(?i)^help$`)
	reGet  = regexp.MustCompile(`(?i)^get\s+(new|recent)(?:\s+(\d+))?$`)
	reSet  = regexp.MustCompile(`(?i)^set\s+(\d+)\s+(new|in_progress|completed|rejected)$`)
)

func helpHandler() (commands.Reply, error) {
	return commands.Reply{Text: "help"}, nil
}

func (r *Request) getHandler(ctx context.Context, mode string, limit int) (commands.Reply, error) {

	switch mode {
	case "new":
		reqs, err := r.store.NewRequests(ctx, limit)
		if err != nil {
			return commands.Reply{Text: "Error getting new requests. Its possible there are none."}, err
		}

		var requests string
		for _, i := range reqs {
			requests += fmt.Sprintf("[%d] %s: %s\n", i.ID, i.Username, i.Body)
		}

		if requests == "" {
			requests = "There are no pending requests to review."
		}

		return commands.Reply{Text: requests}, nil

	case "recent":
		reqs, err := r.store.RecentRequests(ctx, limit)
		if err != nil {
			return commands.Reply{Text: "Error getting recent requests. Its possible there are none."}, err
		}

		var requests string
		for _, i := range reqs {
			requests += fmt.Sprintf("[%d] %s: %s\n", i.ID, i.Username, i.Body)
		}

		if requests == "" {
			requests = "There are no pending requests to review."
		}

		return commands.Reply{Text: requests}, nil
	}

	return commands.Reply{Text: "get"}, nil
}

func (r *Request) setHandler(ctx context.Context, id int64, status string) (commands.Reply, error) {

	if err := r.store.UpdateStatus(ctx, status, id); err != nil {
		return commands.Reply{Text: "Error updating"}, err
	}

	return commands.Reply{Text: fmt.Sprintf("Status has been updated for [%d]", id)}, nil
}

func (r *Request) createHandler(ctx context.Context, e commands.Request) (commands.Reply, error) {
	if e.Identity.PlatformUserID == 0 {
		return commands.Reply{Text: "An error occured while storing your request. Please try again."},
			fmt.Errorf("request: %q's identity couldn't be looked up, so the request can't be saved", e.User.Name)
	}
	featureRequest := newFeatureRequest(e)
	id, err := r.store.StoreRequest(ctx, featureRequest)
	if err != nil {
		return commands.Reply{Text: "An error occured while storing your request. Please try again."}, err
	}

	return commands.Reply{Text: fmt.Sprintf("Your request has been saved (ID %d)", id)}, nil
}

func newFeatureRequest(e commands.Request) FeatureRequest {

	b := strings.Join(e.Args, " ")

	fR := FeatureRequest{
		Platform: e.Platform,
		Body:     b,

		ConnectionID:   e.Connection.ID,
		PlatformUserID: e.Identity.PlatformUserID,
	}

	return fR
}
