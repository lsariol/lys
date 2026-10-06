package catfacts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type CatFact struct{}

func (CatFact) Name() string      { return "catfact" }
func (CatFact) Aliases() []string { return nil }
func (CatFact) Regexes() []string { return nil }
func (CatFact) Description() string {
	return "Gives you a random catfact fetched from https://catfact.ninja"
}
func (CatFact) Usage() string          { return "!catfact" }
func (CatFact) Timeout() time.Duration { return 20 * time.Second }

// Returns a single catfact with max length set to 150
func (CatFact) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	type catfact struct {
		Fact   string `json:"fact"`
		Length int    `json:"length"`
	}

	resp, err := deps.HTTP.Get("https://catfact.ninja/fact?max_length=150")
	if err != nil {
		return adapter.Response{Error: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return adapter.Response{Error: true}, fmt.Errorf("bad status: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var cf catfact
	if err := json.NewDecoder(resp.Body).Decode(&cf); err != nil {
		return adapter.Response{Error: true}, err
	}

	return adapter.Response{Text: fmt.Sprintf("Catfact! %s", cf.Fact)}, nil

}
