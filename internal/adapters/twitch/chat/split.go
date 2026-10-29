package chat

import (
	"strings"
	"unicode/utf16"
)

// maxLength is how long one message may be, a little under Twitch's 500
// characters. Length is counted in UTF-16 units, which is never less than
// Twitch's count, so an emoji can't push a part over.
const maxLength = 480

// maxParts caps how many messages one long text becomes; past it, the last
// part ends with "…".
const maxParts = 4

// split breaks text into messages Twitch accepts, at the last space that fits
// (or mid-word when a word is longer than a message).
func split(text string) []string {
	text = strings.TrimSpace(text)
	var parts []string
	for text != "" {
		if len(parts) == maxParts-1 && length(text) > maxLength {
			head, _ := cut(text, maxLength-1)
			return append(parts, head+"…")
		}
		head, rest := cut(text, maxLength)
		parts = append(parts, head)
		text = rest
	}
	return parts
}

// cut returns the longest start of s within max units, ending at a space when
// there is one, and what's left after it.
func cut(s string, max int) (head, rest string) {
	n, end, space := 0, len(s), -1
	for i, r := range s {
		w := utf16.RuneLen(r)
		if w < 0 {
			w = 1
		}
		if n+w > max {
			end = i
			break
		}
		n += w
		if r == ' ' || r == '\n' {
			space = i
		}
	}
	if end == len(s) {
		return s, ""
	}
	if space > 0 {
		end = space
	}
	return strings.TrimSpace(s[:end]), strings.TrimSpace(s[end:])
}

// length is s's length in UTF-16 units.
func length(s string) int {
	n := 0
	for _, r := range s {
		if w := utf16.RuneLen(r); w > 0 {
			n += w
		} else {
			n++
		}
	}
	return n
}
