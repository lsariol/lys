package sleeper

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRosterEntries(t *testing.T) {
	roster := SleeperRoster{Starters: []string{"1", "0"}, Players: []string{"1", "2", "3", "4", "2"}, Reserve: []string{"3"}, Taxi: []string{"4"}}
	players := map[string]SleeperPlayer{
		"1": {FullName: "Starter"}, "2": {FirstName: "Bench", LastName: "Player"},
		"3": {FullName: "Reserve"}, "4": {FullName: "Prospect"},
	}
	got := getRosterEntries(roster, []string{"QB", "FLEX", "BN"}, players)
	want := []string{"QB Starter", "FLEX Empty", "BN Bench Player", "IR Reserve", "Taxi Prospect"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	entry := formatRosterEntry("WR", strings.Repeat("界", 100))
	if len(entry) > 80 || !utf8.ValidString(entry) {
		t.Fatal("entry exceeds the byte limit or contains broken UTF-8")
	}
}

func TestPlayerCatalogLoadingAndRefreshFailure(t *testing.T) {
	reset := func() {
		playerCatalog.Lock()
		defer playerCatalog.Unlock()
		playerCatalog.players = nil
		playerCatalog.updatedAt = time.Time{}
		playerCatalog.retryAt = time.Time{}
		playerCatalog.loading = false
	}
	reset()
	t.Cleanup(reset)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	deps := &Sleeper{http: &http.Client{Transport: scoresTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"1":{"full_name":"Player One"}}`)), Header: make(http.Header)}, nil
	})}}
	go func() {
		_, err := getPlayerCatalog(context.Background(), deps)
		done <- err
	}()
	<-started
	_, loadingErr := getPlayerCatalog(context.Background(), deps)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if loadingErr == nil {
		t.Fatal("concurrent cold-cache request should report loading")
	}
	players, err := getPlayerCatalog(context.Background(), deps)
	if err != nil || players["1"].FullName != "Player One" {
		t.Fatalf("cached catalog unavailable: %v", err)
	}
	playerCatalog.Lock()
	playerCatalog.updatedAt = time.Now().Add(-25 * time.Hour)
	playerCatalog.Unlock()
	calls := 0
	deps.http = &http.Client{Transport: scoresTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	})}
	for i := 0; i < 2; i++ {
		players, err = getPlayerCatalog(context.Background(), deps)
		if err != nil || players["1"].FullName != "Player One" {
			t.Fatal("failed refresh discarded previous catalog")
		}
	}
	if calls != 1 {
		t.Fatalf("retry cooldown failed: %d requests", calls)
	}
}
