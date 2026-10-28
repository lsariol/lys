// Package sleeper is Lys's client for the Sleeper fantasy football API and the
// types it returns. The !sleeper command and the Sleeper source share one.
package sleeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

const baseURL = "https://api.sleeper.app/v1"

// ErrNotFound means Sleeper has no such league (or other object).
var ErrNotFound = errors.New("Sleeper: not found")

// Client calls the Sleeper API. Share one: it keeps the player catalog.
type Client struct {
	http    *http.Client
	catalog catalog
}

func New(http *http.Client) *Client { return &Client{http: http} }

// get decodes the JSON answer for path into v.
func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Sleeper: %s: %w", path, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("Sleeper: %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("Sleeper: %s: %w", path, err)
	}
	return nil
}

// NFLState is the current NFL season and week.
func (c *Client) NFLState(ctx context.Context) (*NFLState, error) {
	var state NFLState
	if err := c.get(ctx, "/state/nfl", &state); err != nil {
		return nil, err
	}
	if state.Season == "" {
		return nil, errors.New("Sleeper: the NFL state has no season")
	}
	return &state, nil
}

func (c *Client) League(ctx context.Context, leagueID string) (*League, error) {
	var league League
	if err := c.get(ctx, "/league/"+leagueID, &league); err != nil {
		return nil, err
	}
	return &league, nil
}

func (c *Client) LeagueUsers(ctx context.Context, leagueID string) (*[]LeagueUser, error) {
	var users []LeagueUser
	if err := c.get(ctx, "/league/"+leagueID+"/users", &users); err != nil {
		return nil, err
	}
	return &users, nil
}

func (c *Client) Rosters(ctx context.Context, leagueID string) (*[]Roster, error) {
	var rosters []Roster
	if err := c.get(ctx, "/league/"+leagueID+"/rosters", &rosters); err != nil {
		return nil, err
	}
	return &rosters, nil
}

func (c *Client) Matchups(ctx context.Context, leagueID string, week int) (*[]Matchup, error) {
	if week < 1 {
		return nil, fmt.Errorf("Sleeper: invalid matchup week %d", week)
	}
	var matchups []Matchup
	if err := c.get(ctx, fmt.Sprintf("/league/%s/matchups/%d", leagueID, week), &matchups); err != nil {
		return nil, err
	}
	return &matchups, nil
}

func (c *Client) PlayoffBracket(ctx context.Context, leagueID string) (*[]PlayoffMatchup, error) {
	var bracket []PlayoffMatchup
	if err := c.get(ctx, "/league/"+leagueID+"/winners_bracket", &bracket); err != nil {
		return nil, err
	}
	return &bracket, nil
}

// Transactions are the league's trades, waiver claims and free-agent moves in a week.
func (c *Client) Transactions(ctx context.Context, leagueID string, week int) ([]Transaction, error) {
	var txs []Transaction
	if err := c.get(ctx, fmt.Sprintf("/league/%s/transactions/%d", leagueID, week), &txs); err != nil {
		return nil, err
	}
	return txs, nil
}

// catalog is the player catalog, downloaded at most once a day.
type catalog struct {
	sync.Mutex
	players   map[string]Player
	updatedAt time.Time
	retryAt   time.Time
	loading   bool
}

// Players is every NFL player by ID. The catalog is 15 MB, so it's kept for a
// day; when a download fails, the last one is used and it's retried in 5 minutes.
func (c *Client) Players(ctx context.Context) (map[string]Player, error) {
	pc := &c.catalog
	pc.Lock()
	if len(pc.players) > 0 && time.Since(pc.updatedAt) < 24*time.Hour {
		players := pc.players
		pc.Unlock()
		return players, nil
	}
	if pc.loading || time.Now().Before(pc.retryAt) {
		players := pc.players
		pc.Unlock()
		if len(players) > 0 {
			return players, nil
		}
		return nil, errors.New("Sleeper: the player catalog is loading or temporarily unavailable")
	}
	pc.loading = true
	pc.Unlock()

	var players map[string]Player
	err := c.get(ctx, "/players/nfl", &players)
	if err == nil && len(players) == 0 {
		err = errors.New("Sleeper: the player catalog is empty")
	}

	pc.Lock()
	defer pc.Unlock()
	pc.loading = false
	if err != nil {
		pc.retryAt = time.Now().Add(5 * time.Minute)
		if len(pc.players) > 0 {
			log.Printf("[Sleeper] %v; using the catalog from %s", err, pc.updatedAt.Format("2006-01-02 15:04"))
			return pc.players, nil
		}
		return nil, err
	}
	// Replace the map; callers only read published catalogs.
	pc.players = players
	pc.updatedAt = time.Now()
	pc.retryAt = time.Time{}
	return players, nil
}
