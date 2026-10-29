package sleeper

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// MaxMessage is how long a message may be to fit in one chat message.
const MaxMessage = 480

// TradeSides is what each team in a trade gets: "@a gets X | Team B gets Y".
func TradeSides(tx Transaction, n Names) string {
	var sides []string
	for _, roster := range tx.RosterIDs {
		var got []string
		for _, p := range playersOf(tx.Adds, roster) {
			got = append(got, n.Player(p))
		}
		for _, pick := range tx.DraftPicks {
			if pick.OwnerID == roster {
				got = append(got, fmt.Sprintf("%s %s-round pick", pick.Season, Ordinal(pick.Round)))
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
		sides = append(sides, fmt.Sprintf("%s gets %s", n.Who(roster), strings.Join(got, ", ")))
	}
	return strings.Join(sides, " | ")
}

// DescribeTrade is the chat message for a completed trade.
func DescribeTrade(tx Transaction, n Names) string {
	return "🏈 Trade: " + TradeSides(tx, n)
}

// DescribeMoves is one chat message for a team's free-agent moves or waiver
// claims (kind "free_agent" or "waiver"): who it added and dropped, or how
// many moves it made when that doesn't fit in one message. Empty when the
// moves added and dropped no one.
func DescribeMoves(kind string, rosterID int, txs []Transaction, n Names) string {
	var added, dropped []string
	for _, tx := range txs {
		for _, p := range playersOf(tx.Adds, rosterID) {
			name := n.Player(p)
			if kind == "waiver" && tx.Settings != nil && tx.Settings.WaiverBid != nil && *tx.Settings.WaiverBid > 0 {
				name += fmt.Sprintf(" ($%d)", *tx.Settings.WaiverBid)
			}
			added = append(added, name)
		}
		for _, p := range playersOf(tx.Drops, rosterID) {
			dropped = append(dropped, n.Player(p))
		}
	}

	prefix, verb, noun := "🏈 ", "picked up", "free agent move"
	if kind == "waiver" {
		prefix, verb, noun = "🏈 Waivers: ", "claimed", "waiver claim"
	}
	var parts []string
	if len(added) > 0 {
		parts = append(parts, verb+" "+List(added))
	}
	if len(dropped) > 0 {
		parts = append(parts, "dropped "+List(dropped))
	}
	if len(parts) == 0 {
		return ""
	}
	who := n.Who(rosterID)
	if text := prefix + who + " " + strings.Join(parts, ", "); utf8.RuneCountInString(text) <= MaxMessage {
		return text
	}
	if len(txs) != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%s%s made %d %s", prefix, who, len(txs), noun)
}

// playersOf is the players in m that go to the roster, sorted by ID.
func playersOf(m map[string]int, roster int) []string {
	var out []string
	for p, r := range m {
		if r == roster {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
