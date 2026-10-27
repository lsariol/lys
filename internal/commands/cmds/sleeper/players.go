package sleeper

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

var playerCatalog struct {
	sync.Mutex
	players   map[string]SleeperPlayer
	updatedAt time.Time
	retryAt   time.Time
	loading   bool
}

func getPlayerCatalog(ctx context.Context, sl *Sleeper) (map[string]SleeperPlayer, error) {

	playerCatalog.Lock()
	if len(playerCatalog.players) > 0 && time.Since(playerCatalog.updatedAt) < 24*time.Hour {
		players := playerCatalog.players
		playerCatalog.Unlock()
		return players, nil
	}

	if playerCatalog.loading || time.Now().Before(playerCatalog.retryAt) {
		players := playerCatalog.players
		playerCatalog.Unlock()
		if len(players) > 0 {
			return players, nil
		}
		return nil, fmt.Errorf("player catalog is loading or temporarily unavailable")
	}

	playerCatalog.loading = true
	playerCatalog.Unlock()

	players, err := getPlayers(ctx, sl)

	playerCatalog.Lock()
	defer playerCatalog.Unlock()
	playerCatalog.loading = false
	if err != nil {
		log.Print(err)
		playerCatalog.retryAt = time.Now().Add(5 * time.Minute)
		if len(playerCatalog.players) > 0 {
			return playerCatalog.players, nil
		}
		return nil, err
	}

	// Replace the map; callers only read published catalogs.
	playerCatalog.players = players
	playerCatalog.updatedAt = time.Now()
	playerCatalog.retryAt = time.Time{}
	return players, nil
}

func getPlayerName(playerID string, players map[string]SleeperPlayer) string {

	if playerID == "" || playerID == "0" {
		return "Empty"
	}
	player, found := players[playerID]
	if !found {
		return fmt.Sprintf("Player %s", playerID)
	}
	if player.Position == "DEF" {
		if player.Team != "" {
			return player.Team + " D/ST"
		}
		return playerID + " D/ST"
	}

	name := strings.TrimSpace(player.FullName)
	if name == "" {
		name = strings.TrimSpace(player.FirstName + " " + player.LastName)
	}
	if name == "" {
		return fmt.Sprintf("Player %s", playerID)
	}
	return name
}
