// Package server is Lys's HTTP server: /health, and the inlet's endpoint.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Server serves /health from a health function, plus any extra routes.
type Server struct {
	srv *http.Server
	log *slog.Logger
}

// New returns a server on addr whose /health reports version and answers 503 while an adapter is down.
func New(addr, version string, health func() map[string]bool, routes map[string]http.Handler, log *slog.Logger) *Server {
	log = log.With("component", "server")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		adapters := health()
		healthy := true
		for _, ok := range adapters {
			healthy = healthy && ok
		}
		w.Header().Set("Content-Type", "application/json")
		if !healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(struct {
			Healthy  bool            `json:"healthy"`
			Version  string          `json:"version"`
			Adapters map[string]bool `json:"adapters"`
		}{healthy, version, adapters})
	})
	for pattern, h := range routes {
		mux.Handle(pattern, h)
	}

	return &Server{log: log, srv: &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		ErrorLog:     slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}}
}

// Run serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	errs := make(chan error, 1)
	go func() {
		s.log.Info("listening", "addr", s.srv.Addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
		close(errs)
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.srv.Shutdown(shutdownCtx)
}
