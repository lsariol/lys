package chatgpt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
	"github.com/lsariol/lys/internal/config"
)

func testDeps() *dependencies.Deps {
	return &dependencies.Deps{Config: &config.Config{Commands: &config.CommandsConfig{ChatGPTKey: "test"}}}
}

func serve(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := responsesURL
	responsesURL = srv.URL
	t.Cleanup(func() { responsesURL = old })
}

func TestCallChatGPT(t *testing.T) {
	e := adapter.Envelope{Username: "viewer", Args: []string{"hi"}}

	t.Run("answer", func(t *testing.T) {
		serve(t, http.StatusOK, `{"output":[{"type":"web_search_call"},{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`)
		got, err := callChatGPT(context.Background(), e, testDeps())
		if err != nil || got != "hello" {
			t.Fatalf("got %q, %v; want \"hello\", nil", got, err)
		}
	})

	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"error status":      {http.StatusUnauthorized, `{"error":{"message":"bad key"}}`},
		"not json":          {http.StatusBadGateway, `<html>`},
		"empty content":     {http.StatusOK, `{"output":[{"type":"message","content":[]}]}`},
		"no message at all": {http.StatusOK, `{"output":[]}`},
	} {
		t.Run(name, func(t *testing.T) {
			serve(t, tc.status, tc.body)
			if _, err := callChatGPT(context.Background(), e, testDeps()); err == nil {
				t.Fatal("got no error")
			}
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		old := responsesURL
		responsesURL = "http://127.0.0.1:1"
		t.Cleanup(func() { responsesURL = old })
		if _, err := callChatGPT(context.Background(), e, testDeps()); err == nil {
			t.Fatal("got no error")
		}
	})
}
