package catfacts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// CatFact gives a random cat fact from catfact.ninja.
type CatFact struct {
	http *http.Client
}

func New(http *http.Client) *CatFact { return &CatFact{http: http} }

func (*CatFact) Spec() commands.Spec {
	return commands.Spec{
		Name:        "catfact",
		Description: "Gives you a random catfact fetched from https://catfact.ninja",
		Usage:       "!catfact",
		Timeout:     20 * time.Second,
	}
}

// Run returns a single cat fact of at most 150 characters.
func (c *CatFact) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	type catfact struct {
		Fact   string `json:"fact"`
		Length int    `json:"length"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://catfact.ninja/fact?max_length=150", nil)
	if err != nil {
		return commands.Reply{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return commands.Reply{}, fmt.Errorf("catfact.ninja: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return commands.Reply{}, fmt.Errorf("catfact.ninja: bad status: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var cf catfact
	if err := json.NewDecoder(resp.Body).Decode(&cf); err != nil {
		return commands.Reply{}, fmt.Errorf("catfact.ninja: %w", err)
	}

	return commands.Reply{Text: fmt.Sprintf("Catfact! %s", cf.Fact)}, nil
}
