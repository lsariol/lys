package ping

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// Ping reports how long a Helix request takes.
type Ping struct {
	http *http.Client
}

func New(http *http.Client) *Ping { return &Ping{http: http} }

func (*Ping) Spec() commands.Spec {
	return commands.Spec{Name: "ping", Description: "Latency check.", Usage: "!ping", Timeout: 3 * time.Second}
}

func (p *Ping) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users?id=965482552", nil)
	if err != nil {
		return commands.Reply{}, err
	}

	start := time.Now()
	resp, err := p.http.Do(req)
	if err != nil {
		return commands.Reply{Text: "pong! HelixAPI: unknown (Error recevied on ping)"}, nil
	}
	resp.Body.Close()
	diff := time.Since(start)
	return commands.Reply{Text: fmt.Sprintf("pong! Helix API: %d", diff.Milliseconds())}, nil
}
