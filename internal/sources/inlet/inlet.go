// Package inlet is the source other services post messages to.
package inlet

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/sources"
)

// maxBody limits a notification's size.
const maxBody = 64 << 10

// notification is the JSON body of POST /notifications.
type notification struct {
	Message       string   `json:"message"`
	AlertChannels []string `json:"alert_channels"` // "platform:channelID" or "platform:channelID:channelName"
	UserID        string   `json:"user_id"`
	GUID          string   `json:"guid"`
}

// Inlet is the http.Handler for POST /notifications and the Source that publishes what it receives.
type Inlet struct {
	token string
	inbox chan notification
	log   *slog.Logger
}

var _ sources.Source = (*Inlet)(nil)

// New returns an inlet that needs "Authorization: Bearer <token>"; with an empty token it refuses everything.
func New(token string, log *slog.Logger) *Inlet {
	return &Inlet{token: token, inbox: make(chan notification, 256), log: log.With("component", "inlet")}
}

func (in *Inlet) Name() string { return "inlet" }

func (in *Inlet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !in.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var n notification
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&n); err != nil {
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
	case in.inbox <- n:
	default:
		in.log.Warn("inbox full: dropped a notification", "guid", n.GUID, "dropped", 1)
		http.Error(w, "server busy", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (in *Inlet) authorized(r *http.Request) bool {
	if in.token == "" {
		return false
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(got), []byte(in.token)) == 1
}

// Run publishes each notification to every channel it names.
func (in *Inlet) Run(ctx context.Context, pub sources.Publisher) error {
	if in.token == "" {
		in.log.Warn("SHARED_LYS_NOTIFICATION_TOKEN is empty: refusing every notification")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case n := <-in.inbox:
			for _, out := range in.outbounds(n) {
				if err := pub.Publish(ctx, out); err != nil {
					in.log.Warn("notification not sent", "guid", n.GUID, "channel_id", out.Channel.ID, "err", err)
				}
			}
		}
	}
}

// outbounds turns a notification into one message per alert channel, skipping and logging malformed ones.
func (in *Inlet) outbounds(n notification) []core.Outbound {
	out := make([]core.Outbound, 0, len(n.AlertChannels))
	for _, ch := range n.AlertChannels {
		parts := strings.SplitN(ch, ":", 3)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			in.log.Warn("skipping a malformed alert channel: want platform:channelID", "guid", n.GUID, "alert_channel", ch)
			continue
		}
		o := core.Outbound{Platform: parts[0], Channel: core.Channel{ID: parts[1]}, Text: n.Message}
		if len(parts) == 3 {
			o.Channel.Name = parts[2]
		}
		out = append(out, o)
	}
	return out
}
