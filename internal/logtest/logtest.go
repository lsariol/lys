// Package logtest gives tests a logger that discards, or one that keeps records through blackboxlog.
package logtest

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/lsariol/blackboxlog"
)

// Discard logs nothing.
var Discard = slog.New(slog.DiscardHandler)

// Buffer keeps log records as JSON lines, safe for concurrent writers.
type Buffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// New returns a logger whose records go to the returned buffer.
func New() (*slog.Logger, *Buffer) {
	b := &Buffer{}
	return blackboxlog.New(blackboxlog.Options{Output: b, Format: blackboxlog.JSON, Level: slog.LevelDebug}), b
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String is everything logged so far.
func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Records decodes every record logged so far.
func (b *Buffer) Records(t testing.TB) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("not one JSON record per line: %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}
