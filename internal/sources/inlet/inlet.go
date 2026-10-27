// Package inlet is the source other services post messages to, such as
// LetterboxdWatcher's new reviews.
package inlet

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
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

// Inlet is an http.Handler for POST /notifications and the Source that
// publishes what it receives.
type Inlet struct {
	token string
	inbox chan notification
}

var _ sources.Source = (*Inlet)(nil)

// New returns an inlet that accepts requests carrying "Authorization: Bearer
// <token>". With an empty token it refuses every request.
func New(token string) *Inlet {
	return &Inlet{token: token, inbox: make(chan notification, 256)}
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
		log.Printf("[Inlet] inbox full, dropping notification %q", n.GUID)
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
		log.Println("! [Inlet] SHARED_LYS_NOTIFICATION_TOKEN is empty: refusing every notification")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case n := <-in.inbox:
			for _, out := range outbounds(n) {
				if err := pub.Publish(ctx, out); err != nil {
					log.Printf("[Inlet] notification %q: %v", n.GUID, err)
				}
			}
		}
	}
}

// outbounds turns a notification into one message per alert channel,
// skipping (and logging) malformed entries.
func outbounds(n notification) []core.Outbound {
	out := make([]core.Outbound, 0, len(n.AlertChannels))
	for _, ch := range n.AlertChannels {
		parts := strings.SplitN(ch, ":", 3)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			log.Printf("[Inlet] skipping malformed alert channel %q (want platform:channelID)", ch)
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
