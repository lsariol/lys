package notification

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	notificationsource "github.com/lsariol/lys/internal/feed/sources/notification"
)

// Server is the HTTP server that accepts inbound notifications.
// All business logic lives in the NotificationSource; this struct only owns
// the net/http plumbing.
type Server struct {
	addr   string
	source *notificationsource.NotificationSource
	srv    *http.Server
}

// NewServer creates a Server.
//
//	addr   — listen address, e.g. ":8080"
//	source — a NotificationSource already registered with the Feed via
//	         feed.AddSource so its outbound channel is wired up
func NewServer(addr string, source *notificationsource.NotificationSource) *Server {
	return &Server{
		addr:   addr,
		source: source,
	}
}

// Start registers routes and begins listening in a background goroutine.
// Returns immediately. Use Shutdown to stop.
func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.healthHandler)
	mux.Handle("POST /notifications", s.source)

	s.srv = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("[NotificationServer] listening on %s", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[NotificationServer] fatal: %v", err)
		}
	}()
}

// Shutdown gracefully drains in-flight requests. Safe to call even if Start
// was never called.
func (s *Server) Shutdown(ctx context.Context) {
	if s.srv == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[NotificationServer] shutdown error: %v", err)
	}
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"healthy":true}`))
}
