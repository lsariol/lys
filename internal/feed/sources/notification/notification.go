package notification

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/lsariol/lys/internal/feed"
)

// inboundNotification mirrors the JSON body accepted by POST /notifications.
type inboundNotification struct {
	Message       string   `json:"message"`
	AlertChannels []string `json:"alert_channels"`
	UserID        string   `json:"user_id"`
	GUID          string   `json:"guid"`
}

// NotificationSource is both an http.Handler (POST /notifications) and a
// feed.Source (drains its inbox and pushes feed.Events to the router).
//
// Wiring order in main:
//  1. ns := notification.New()
//  2. feed.AddSource(ns)        ← Feed calls ns.SetOutbound automatically
//  3. notifserver.NewServer(addr, ns).Start()
type NotificationSource struct {
	inbox chan inboundNotification
	out   chan<- feed.Event
}

// New creates a NotificationSource. It has no outbound channel until
// Feed.AddSource calls SetOutbound.
func New() *NotificationSource {
	return &NotificationSource{
		inbox: make(chan inboundNotification, 256),
	}
}

// SetOutbound implements feed.Outbounder.
// Called automatically by Feed.AddSource.
func (ns *NotificationSource) SetOutbound(out chan<- feed.Event) {
	ns.out = out
}

// ServeHTTP implements http.Handler.
func (ns *NotificationSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var n inboundNotification
	if err := json.Unmarshal(body, &n); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if n.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if len(n.AlertChannels) == 0 {
		http.Error(w, "alert_channels is required", http.StatusBadRequest)
		return
	}

	select {
	case ns.inbox <- n:
	default:
		log.Println("[NotificationSource] inbox full, dropping notification:", n.GUID)
		http.Error(w, "server busy", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// PullFeed implements sources.Source.
// Runs until ctx is cancelled, converting inbox items into feed.Events and
// forwarding them to the router via the out channel.
func (ns *NotificationSource) PullFeed(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-ns.inbox:
			if !ok {
				return
			}
			event := ns.toEvent(n)
			select {
			case ns.out <- event:
			case <-ctx.Done():
				return
			}
		}
	}
}

// toEvent converts a raw inboundNotification into a feed.Event.
// AlertChannels entries are parsed as "platform:channelID" or
// "platform:channelID:channelName". Malformed entries are skipped with a log.
func (ns *NotificationSource) toEvent(n inboundNotification) feed.Event {
	subs := make([]feed.Subscriber, 0, len(n.AlertChannels))

	for _, ch := range n.AlertChannels {
		parts := strings.SplitN(ch, ":", 3)
		if len(parts) < 2 {
			log.Printf("[NotificationSource] skipping malformed alert_channel %q (expected platform:channelID)\n", ch)
			continue
		}
		sub := feed.Subscriber{
			Platform:  parts[0],
			ChannelID: parts[1],
		}
		if len(parts) == 3 {
			sub.ChannelName = parts[2]
		}
		subs = append(subs, sub)
	}

	return feed.Event{
		Type:        feed.EventNotification,
		Subscribers: subs,
		Payload: feed.NotificationPayload{
			Message: n.Message,
			UserID:  n.UserID,
			GUID:    n.GUID,
		},
	}
}
