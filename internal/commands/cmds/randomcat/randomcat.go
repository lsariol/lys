package randomcat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// Endpoint is Cat as a Service, cataas.com.
const Endpoint = "https://cataas.com"

// failReply is sent when cataas.com can't be reached or answers oddly.
const failReply = "Couldn't reach cataas.com, try again in a moment."

// RandomCat links a random cat picture or gif from cataas.com, either at random.
type RandomCat struct {
	http     *http.Client
	endpoint string
	gif      func() bool
}

func New(http *http.Client) *RandomCat {
	return &RandomCat{http: http, endpoint: Endpoint, gif: func() bool { return rand.IntN(2) == 0 }}
}

func (*RandomCat) Spec() commands.Spec {
	return commands.Spec{
		Name:        "randomcat",
		Description: "Gives you a random cat picture or gif from cataas.com",
		Usage:       "!randomcat",
		Timeout:     20 * time.Second,
		Concurrency: 16,
	}
}

// Run picks a picture or a gif, then links the cat cataas.com returns by its ID.
func (c *RandomCat) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	path := "/cat"
	if c.gif() {
		path = "/cat/gif"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path+"?json=true", nil)
	if err != nil {
		return commands.Reply{Text: failReply}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return commands.Reply{Text: failReply}, fmt.Errorf("cataas.com: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return commands.Reply{Text: failReply}, fmt.Errorf("cataas.com: bad status: %s, body: %s", resp.Status, body)
	}

	var cat struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&cat); err != nil {
		return commands.Reply{Text: failReply}, fmt.Errorf("cataas.com: %w", err)
	}
	if cat.ID == "" {
		return commands.Reply{Text: failReply}, fmt.Errorf("cataas.com: the answer has no cat ID; has the API changed?")
	}
	return commands.Reply{Text: "Here's a random cat. " + Endpoint + "/cat/" + cat.ID}, nil
}
