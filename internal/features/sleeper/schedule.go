package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // the image has no time zone files; kickoffs are shown in Eastern time
)

// Sleeper's schedule has dates but no times, so kickoffs come from ESPN's
// public scoreboard. It's unofficial: an answer that doesn't parse is an
// error, never a guessed time.
const scheduleURL = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard"

// scheduleFor is how long a week's kickoffs are kept before asking again.
const scheduleFor = 6 * time.Hour

// Eastern is the NFL's time zone, for showing kickoffs.
var Eastern = mustLoad("America/New_York")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("sleeper: time zone %s: %v", name, err))
	}
	return loc
}

// Game is one NFL game of a week.
type Game struct {
	Kickoff time.Time
	Away    string // team abbreviations, such as "JAX"
	Home    string
}

// Name is the game as "JAX @ BAL".
func (g Game) Name() string { return g.Away + " @ " + g.Home }

type weekKey struct {
	season string
	week   int
}

type cachedWeek struct {
	games []Game
	at    time.Time
}

// schedule keeps each week's games for scheduleFor.
type schedule struct {
	sync.Mutex
	weeks map[weekKey]cachedWeek
}

// Games is the regular-season week's games, earliest kickoff first.
func (c *Client) Games(ctx context.Context, season string, week int) ([]Game, error) {
	key := weekKey{season, week}
	c.schedule.Lock()
	if w, ok := c.schedule.weeks[key]; ok && time.Since(w.at) < scheduleFor {
		c.schedule.Unlock()
		return w.games, nil
	}
	c.schedule.Unlock()

	url := fmt.Sprintf("%s?seasontype=2&week=%d&dates=%s", scheduleURL, week, season)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ESPN: week %d schedule: %w", week, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN: week %d schedule: status %d", week, resp.StatusCode)
	}
	var board scoreboard
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		return nil, fmt.Errorf("ESPN: week %d schedule: %w", week, err)
	}
	games, err := board.games(week)
	if err != nil {
		return nil, fmt.Errorf("ESPN: week %d schedule: %w", week, err)
	}

	c.schedule.Lock()
	if c.schedule.weeks == nil {
		c.schedule.weeks = make(map[weekKey]cachedWeek)
	}
	c.schedule.weeks[key] = cachedWeek{games: games, at: time.Now()}
	c.schedule.Unlock()
	return games, nil
}

// scoreboard is the part of ESPN's answer Lys reads.
type scoreboard struct {
	Week struct {
		Number int `json:"number"`
	} `json:"week"`
	Events []struct {
		Date         string `json:"date"`
		Competitions []struct {
			Competitors []struct {
				HomeAway string `json:"homeAway"`
				Team     struct {
					Abbreviation string `json:"abbreviation"`
				} `json:"team"`
			} `json:"competitors"`
		} `json:"competitions"`
	} `json:"events"`
}

func (b scoreboard) games(week int) ([]Game, error) {
	if b.Week.Number != week {
		return nil, fmt.Errorf("asked for week %d, got week %d", week, b.Week.Number)
	}
	if len(b.Events) == 0 {
		return nil, fmt.Errorf("no games in week %d", week)
	}
	games := make([]Game, 0, len(b.Events))
	for _, e := range b.Events {
		kickoff, err := parseKickoff(e.Date)
		if err != nil {
			return nil, err
		}
		g := Game{Kickoff: kickoff}
		if len(e.Competitions) > 0 {
			for _, team := range e.Competitions[0].Competitors {
				switch team.HomeAway {
				case "home":
					g.Home = team.Team.Abbreviation
				case "away":
					g.Away = team.Team.Abbreviation
				}
			}
		}
		if g.Home == "" || g.Away == "" {
			return nil, fmt.Errorf("a game on %s has no teams", e.Date)
		}
		games = append(games, g)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].Kickoff.Before(games[j].Kickoff) })
	return games, nil
}

// parseKickoff reads ESPN's times, which leave out the seconds ("2026-11-06T01:15Z").
func parseKickoff(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z07:00"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("kickoff %q isn't a time", s)
}
