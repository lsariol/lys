package sleeper

import (
	"fmt"
	"sort"
	"strings"

	api "github.com/lsariol/lys/internal/features/sleeper"
)

// transactionNames turns roster and player IDs into names for messages.
type transactionNames struct {
	team   func(rosterID int) string
	player func(playerID string) string
}

// describeTransaction is the chat message for a completed trade or waiver
// claim; ok is false for anything else.
func describeTransaction(tx api.Transaction, n transactionNames) (string, bool) {
	if tx.Status != "complete" {
		return "", false
	}
	switch tx.Type {
	case "trade":
		var sides []string
		for _, roster := range tx.RosterIDs {
			var got []string
			for _, p := range sortedPlayers(tx.Adds, roster) {
				got = append(got, n.player(p))
			}
			for _, pick := range tx.DraftPicks {
				if pick.OwnerID == roster {
					got = append(got, fmt.Sprintf("%s %s-round pick", pick.Season, ordinal(pick.Round)))
				}
			}
			for _, b := range tx.WaiverBudget {
				if b.Receiver == roster {
					got = append(got, fmt.Sprintf("$%d FAAB", b.Amount))
				}
			}
			if len(got) == 0 {
				got = []string{"nothing"}
			}
			sides = append(sides, fmt.Sprintf("%s gets %s", n.team(roster), strings.Join(got, ", ")))
		}
		return truncate("🏈 Trade: " + strings.Join(sides, " | ")), true

	case "waiver":
		if len(tx.RosterIDs) == 0 {
			return "", false
		}
		roster := tx.RosterIDs[0]
		var adds, drops []string
		for _, p := range sortedPlayers(tx.Adds, roster) {
			adds = append(adds, n.player(p))
		}
		for _, p := range sortedPlayers(tx.Drops, roster) {
			drops = append(drops, n.player(p))
		}
		if len(adds) == 0 {
			return "", false
		}
		text := fmt.Sprintf("🏈 Waiver: %s adds %s", n.team(roster), strings.Join(adds, ", "))
		if tx.Settings != nil && tx.Settings.WaiverBid != nil && *tx.Settings.WaiverBid > 0 {
			text += fmt.Sprintf(" ($%d)", *tx.Settings.WaiverBid)
		}
		if len(drops) > 0 {
			text += ", drops " + strings.Join(drops, ", ")
		}
		return truncate(text), true
	}
	return "", false
}

func sortedPlayers(m map[string]int, roster int) []string {
	var out []string
	for p, r := range m {
		if r == roster {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// truncate keeps a message within Twitch's limit.
func truncate(s string) string {
	const max = 480
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	for len(string(r)) > max-3 {
		r = r[:len(r)-1]
	}
	return string(r) + "..."
}
