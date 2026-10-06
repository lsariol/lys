package request

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type Request struct{}

func (Request) Name() string           { return "request" }
func (Request) Aliases() []string      { return nil }
func (Request) Regexes() []string      { return nil }
func (Request) Description() string    { return "Logs a request for things to be added to the bot." }
func (Request) Usage() string          { return "!request <feature or idea you want added>" }
func (Request) Timeout() time.Duration { return 3 * time.Second }

// Users can request things added to the bot. Will log it into the database.
func (Request) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	// We dont care if there is no arguments / body is empty
	if len(e.Args) == 0 {
		return adapter.Response{Text: "Usage: !request <feature or idea>"}, fmt.Errorf("incorrect command usage")
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
				return adapter.Response{Text: "Usage !request get <new/recent> < # to return (optional)>"}, err
			}
			//Until more is figurred out, we need to limit to 1 - CHANGE TO N WHEN READY
			limit = n
		}
		if strings.ToLower(e.Username) != "mobasity" {
			return adapter.Response{SuppressReply: true, Error: true}, fmt.Errorf("insufficient permissions")
		}
		return getHandler(ctx, mode, limit, deps)
	}

	if m := reSet.FindStringSubmatch(full); m != nil {
		idStr := m[1]
		status := m[2]

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return adapter.Response{Text: "Invalid request ID."}, err
		}
		if strings.ToLower(e.Username) != "mobasity" {
			return adapter.Response{SuppressReply: true, Error: true}, fmt.Errorf("insufficient permissions")
		}
		return setHandler(ctx, id, status, deps)

	}

	return createHandler(ctx, e, deps)
}

var (
	reHelp = regexp.MustCompile(`(?i)^help$`)
	reGet  = regexp.MustCompile(`(?i)^get\s+(new|recent)(?:\s+(\d+))?$`)
	reSet  = regexp.MustCompile(`(?i)^set\s+(\d+)\s+(new|in_progress|completed|rejected)$`)
)

func helpHandler() (adapter.Response, error) {
	return adapter.Response{Text: "help"}, nil
}

func getHandler(ctx context.Context, mode string, limit int, deps *dependencies.Deps) (adapter.Response, error) {

	switch mode {
	case "new":
		reqs, err := getNewRequests(ctx, limit, deps)
		if err != nil {
			return adapter.Response{Text: "Error getting new requests. Its possible there are none.", Error: true}, err
		}

		var requests string
		for _, i := range reqs {
			requests += fmt.Sprintf("[%d] %s: %s\n", i.ID, i.Username, i.Body)
		}

		if requests == "" {
			requests = "There are no pending requests to review."
		}

		return adapter.Response{Text: requests}, nil

	case "recent":
		reqs, err := getRecentRequests(ctx, limit, deps)
		if err != nil {
			return adapter.Response{Text: "Error getting recent requests. Its possible there are none.", Error: true}, err
		}

		var requests string
		for _, i := range reqs {
			requests += fmt.Sprintf("[%d] %s: %s\n", i.ID, i.Username, i.Body)
		}

		if requests == "" {
			requests = "There are no pending requests to review."
		}

		return adapter.Response{Text: requests}, nil
	}

	return adapter.Response{Text: "get"}, nil
}

func setHandler(ctx context.Context, id int64, status string, deps *dependencies.Deps) (adapter.Response, error) {

	if err := updateStatus(ctx, status, id, deps); err != nil {
		return adapter.Response{Text: "Error updating", Error: true}, err
	}

	return adapter.Response{Text: fmt.Sprintf("Status has been updated for [%d]", id)}, nil
}

func createHandler(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
	featureRequest := newFeatureRequest(e)
	id, err := storeRequest(ctx, featureRequest, deps)
	if err != nil {
		return adapter.Response{Text: "An error occured while storing your request. Please try again."}, err
	}

	return adapter.Response{Text: fmt.Sprintf("Your request has been saved (ID %d)", id)}, nil
}

func newFeatureRequest(e adapter.Envelope) FeatureRequest {

	b := strings.Join(e.Args, " ")

	fR := FeatureRequest{
		UserID:      e.UserID,
		Username:    e.Username,
		ChannelID:   e.ChannelID,
		ChannelName: e.ChannelName,
		Platform:    e.Platform,
		Body:        b,
	}

	return fR
}
