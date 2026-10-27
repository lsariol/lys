package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func getPlayoffBracket(sleeperLeagueID string, ctx context.Context, sl *Sleeper) (*[]SleeperPlayoffMatchup, error) {

	url := fmt.Sprintf("https://api.sleeper.app/v1/league/%s/winners_bracket", sleeperLeagueID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := sl.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sleeper API returned status %d", resp.StatusCode)
	}

	var bracket []SleeperPlayoffMatchup
	if err := json.NewDecoder(resp.Body).Decode(&bracket); err != nil {
		return nil, err
	}
	return &bracket, nil
}

func getPlayers(ctx context.Context, sl *Sleeper) (map[string]SleeperPlayer, error) {

	url := "https://api.sleeper.app/v1/players/nfl"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := sl.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sleeper API returned status %d", resp.StatusCode)
	}

	var players map[string]SleeperPlayer
	if err := json.NewDecoder(resp.Body).Decode(&players); err != nil {
		return nil, err
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("sleeper API returned an empty player catalog")
	}
	return players, nil
}

func getNFLState(ctx context.Context, sl *Sleeper) (*SleeperNFLState, error) {

	url := "https://api.sleeper.app/v1/state/nfl"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := sl.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sleeper API returned status %d", resp.StatusCode)
	}

	var state SleeperNFLState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, err
	}
	if state.Season == "" {
		return nil, fmt.Errorf("sleeper API returned NFL state without a season")
	}

	return &state, nil
}

func getMatchups(sleeperLeagueID string, week int, ctx context.Context, sl *Sleeper) (*[]SleeperMatchup, error) {

	if week < 1 {
		return nil, fmt.Errorf("invalid matchup week %d", week)
	}

	url := fmt.Sprintf("https://api.sleeper.app/v1/league/%s/matchups/%d", sleeperLeagueID, week)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := sl.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sleeper API returned status %d", resp.StatusCode)
	}

	var matchups []SleeperMatchup
	if err := json.NewDecoder(resp.Body).Decode(&matchups); err != nil {
		return nil, err
	}

	return &matchups, nil
}

func GetLeague(ctx context.Context, leagueID string, httpClient *http.Client) (*League, error) {

	url := fmt.Sprintf("https://api.sleeper.app/v1/league/%s", leagueID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("league not found %s", leagueID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sleeper API returned status %d", resp.StatusCode)
	}

	var league League
	if err := json.NewDecoder(resp.Body).Decode(&league); err != nil {
		return nil, err
	}

	return &league, nil

}

func getPlayersInLeague(sleeperLeagueID string, ctx context.Context, sl *Sleeper) (*[]SleeperLeagueUser, error) {

	var users []SleeperLeagueUser
	url := fmt.Sprintf("https://api.sleeper.app/v1/league/%s/users", sleeperLeagueID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := sl.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sleeper API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, err
	}

	return &users, nil
}

func getRosters(sleeperLeagueID string, ctx context.Context, sl *Sleeper) (*[]SleeperRoster, error) {

	var rosters []SleeperRoster

	url := fmt.Sprintf("https://api.sleeper.app/v1/league/%s/rosters", sleeperLeagueID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := sl.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sleeper API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&rosters); err != nil {
		return nil, err
	}

	return &rosters, nil
}
