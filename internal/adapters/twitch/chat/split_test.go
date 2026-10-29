package chat

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want []string
	}{
		"short":     {"hello there", []string{"hello there"}},
		"empty":     {"  ", nil},
		"at spaces": {strings.Repeat("a", 470) + " " + strings.Repeat("b", 20), []string{strings.Repeat("a", 470), strings.Repeat("b", 20)}},
		"one word":  {strings.Repeat("a", 500), []string{strings.Repeat("a", 480), strings.Repeat("a", 20)}},
		"capped": {strings.Repeat("a ", 1200), []string{
			strings.TrimSpace(strings.Repeat("a ", 240)),
			strings.TrimSpace(strings.Repeat("a ", 240)),
			strings.TrimSpace(strings.Repeat("a ", 240)),
			strings.TrimSpace(strings.Repeat("a ", 239)) + "…",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			got := split(tc.text)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || len(got) != len(tc.want) {
				t.Fatalf("got %d parts %q", len(got), got)
			}
			for _, p := range got {
				if length(p) > maxLength {
					t.Fatalf("a part is %d long", length(p))
				}
			}
		})
	}
}

// An emoji is 2 UTF-16 units, so 300 of them don't fit in one message.
func TestSplitEmoji(t *testing.T) {
	got := split(strings.Repeat("😀", 300))
	if len(got) != 2 || length(got[0]) != 480 || length(got[1]) != 120 {
		t.Fatalf("got %d parts, lengths %d", len(got), length(got[0]))
	}
}
