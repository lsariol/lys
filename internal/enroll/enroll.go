// Package enroll is the API the website calls when a channel owner connects
// Lys or turns it off. The website verifies the owner (OAuth) first; Lys
// trusts a caller that has the shared enrollment token.
package enroll

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/lsariol/lys/internal/connections"
)

// Store is the part of the connections store the API uses.
type Store interface {
	Enroll(ctx context.Context, e connections.Enrollment) (connections.Connection, bool, error)
	Disable(ctx context.Context, platform, externalID string) error
}

// maxBody limits a request's size.
const maxBody = 16 << 10

// platformName is what a platform may be called: lower-case letters.
var platformName = regexp.MustCompile(`^[a-z]{2,20}$`)

// API serves POST /channels and DELETE /channels/{platform}/{id}.
type API struct {
	store Store
	token string
}

// New returns the API. With an empty token it refuses every request.
func New(store Store, token string) *API {
	if token == "" {
		log.Println("! [Enroll] SHARED_LYS_ENROLLMENT_TOKEN is empty: refusing every enrollment")
	}
	return &API{store: store, token: token}
}

// Routes are the API's handlers, by pattern, for the HTTP server.
func (a *API) Routes() map[string]http.Handler {
	return map[string]http.Handler{
		"POST /channels":                   a.authorized(http.HandlerFunc(a.enroll)),
		"DELETE /channels/{platform}/{id}": a.authorized(http.HandlerFunc(a.disable)),
	}
}

// enrollRequest is the body of POST /channels.
type enrollRequest struct {
	Platform  string `json:"platform"`
	ChannelID string `json:"channel_id"`
	Name      string `json:"name"`  // display name
	Login     string `json:"login"` // Twitch login; optional
}

// enrollResponse says what happened, so the website can tell the owner.
type enrollResponse struct {
	ConnectionID int64 `json:"connection_id"`
	Created      bool  `json:"created"` // false: it was known, and is on again
	Active       bool  `json:"active"`  // Lys joins it within seconds
	Blocked      bool  `json:"blocked"` // Lys refuses to join it, whatever the owner wants
}

func (a *API) enroll(w http.ResponseWriter, r *http.Request) {
	var req enrollRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.ChannelID = strings.TrimSpace(req.ChannelID)
	req.Name = strings.TrimSpace(req.Name)
	switch {
	case !platformName.MatchString(req.Platform):
		writeError(w, http.StatusBadRequest, "platform must be a name like \"twitch\"")
		return
	case req.ChannelID == "":
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	case req.Name == "":
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	c, created, err := a.store.Enroll(r.Context(), connections.Enrollment{
		Platform: req.Platform, ExternalID: req.ChannelID, Name: req.Name, Login: strings.TrimSpace(req.Login),
	})
	if err != nil {
		log.Printf("✗ [Enroll] %v", err)
		writeError(w, http.StatusInternalServerError, "couldn't save the channel; try again")
		return
	}
	log.Printf("[Enroll] %s channel %q (%s): created %v, blocked %v", c.Platform, c.Name, c.ExternalID, created, c.AdminBlocked)
	writeJSON(w, http.StatusOK, enrollResponse{ConnectionID: c.ID, Created: created, Active: c.Active(), Blocked: c.AdminBlocked})
}

func (a *API) disable(w http.ResponseWriter, r *http.Request) {
	platform, id := r.PathValue("platform"), r.PathValue("id")
	err := a.store.Disable(r.Context(), platform, id)
	switch {
	case errors.Is(err, connections.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such channel")
	case err != nil:
		log.Printf("✗ [Enroll] %v", err)
		writeError(w, http.StatusInternalServerError, "couldn't turn the channel off; try again")
	default:
		log.Printf("[Enroll] %s channel %s turned off by its owner", platform, id)
		writeJSON(w, http.StatusOK, map[string]bool{"disabled": true})
	}
}

// authorized refuses requests without "Authorization: Bearer <token>".
func (a *API) authorized(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if a.token == "" || !ok || subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
