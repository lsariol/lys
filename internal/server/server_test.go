package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	for name, tc := range map[string]struct {
		adapters map[string]bool
		want     int
	}{
		"connected":    {map[string]bool{"twitch": true}, http.StatusOK},
		"disconnected": {map[string]bool{"twitch": false}, http.StatusServiceUnavailable},
	} {
		s := New(":0", "v1.2.3", func() map[string]bool { return tc.adapters }, nil)
		w := httptest.NewRecorder()
		s.srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
		if w.Code != tc.want {
			t.Errorf("%s: got %d, want %d", name, w.Code, tc.want)
		}
		if !strings.Contains(w.Body.String(), `"version":"v1.2.3"`) {
			t.Errorf("%s: no version in %s", name, w.Body.String())
		}
	}
}
