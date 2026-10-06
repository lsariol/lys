package ping

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type Ping struct{}

func (Ping) Name() string           { return "ping" }
func (Ping) Aliases() []string      { return nil }
func (Ping) Regexes() []string      { return nil }
func (Ping) Description() string    { return "Latency check." }
func (Ping) Usage() string          { return "!ping" }
func (Ping) Timeout() time.Duration { return 3 * time.Second }

func (Ping) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	start := time.Now()
	_, err := deps.HTTP.Get("https://api.twitch.tv/helix/users?id=965482552")
	if err != nil {
		return adapter.Response{Text: "pong! HelixAPI: unknown (Error recevied on ping)"}, nil
	}
	diff := time.Since(start)
	return adapter.Response{Text: fmt.Sprintf("pong! Helix API: %d", diff.Milliseconds())}, nil

}
