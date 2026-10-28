package sleeper

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
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

func TestGames(t *testing.T) {
	fixture, err := os.ReadFile("testdata/espn-week9.json")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := New(&http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		calls++
		if q := req.URL.Query(); req.URL.Host != "site.api.espn.com" || q.Get("week") != "9" || q.Get("dates") != "2026" || q.Get("seasontype") != "2" {
			t.Errorf("unexpected request %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(fixture)), Header: make(http.Header)}, nil
	})})

	games, err := c.Games(context.Background(), "2026", 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 15 {
		t.Fatalf("%d games, want 15", len(games))
	}
	first := games[0]
	if first.Name() != "JAX @ BAL" || !first.Kickoff.Equal(time.Date(2026, 11, 6, 1, 15, 0, 0, time.UTC)) {
		t.Fatalf("first game: %s at %s", first.Name(), first.Kickoff)
	}
	if got := first.Kickoff.In(Eastern).Format("Mon 3:04 PM"); got != "Thu 8:15 PM" {
		t.Fatalf("in Eastern time: %s", got)
	}
	for i := 1; i < len(games); i++ {
		if games[i].Kickoff.Before(games[i-1].Kickoff) {
			t.Fatal("games aren't in kickoff order")
		}
	}
	if _, err := c.Games(context.Background(), "2026", 9); err != nil || calls != 1 {
		t.Fatalf("the week wasn't cached: %d calls, %v", calls, err)
	}

	// A different week in the answer, or one without teams, is an error, never a guess.
	if _, err := New(&http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(fixture)), Header: make(http.Header)}, nil
	})}).Games(context.Background(), "2026", 10); err == nil {
		t.Fatal("week 9's games were accepted for week 10")
	}
	for _, body := range []string{`{"week":{"number":9},"events":[]}`, `{"week":{"number":9},"events":[{"date":"soon"}]}`, `{"week":{"number":9},"events":[{"date":"2026-11-06T01:15Z","competitions":[{"competitors":[]}]}]}`} {
		_, err := New(&http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}).Games(context.Background(), "2026", 9)
		if err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}
