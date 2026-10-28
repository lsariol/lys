package sleeper

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestPlayerCatalogLoadingAndRefreshFailure(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	c := New(&http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"1":{"full_name":"Player One"}}`)), Header: make(http.Header)}, nil
	})})
	go func() {
		_, err := c.Players(context.Background())
		done <- err
	}()
	<-started
	_, loadingErr := c.Players(context.Background())
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if loadingErr == nil {
		t.Fatal("concurrent cold-cache request should report loading")
	}
	players, err := c.Players(context.Background())
	if err != nil || players["1"].FullName != "Player One" {
		t.Fatalf("cached catalog unavailable: %v", err)
	}
	c.catalog.Lock()
	c.catalog.updatedAt = time.Now().Add(-25 * time.Hour)
	c.catalog.Unlock()
	calls := 0
	c.http = &http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	})}
	for i := 0; i < 2; i++ {
		players, err = c.Players(context.Background())
		if err != nil || players["1"].FullName != "Player One" {
			t.Fatal("failed refresh discarded previous catalog")
		}
	}
	if calls != 1 {
		t.Fatalf("retry cooldown failed: %d requests", calls)
	}
}
