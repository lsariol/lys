package randomdog

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

type RandomDog struct{}

func (RandomDog) Name() string      { return "randomdog" }
func (RandomDog) Aliases() []string { return nil }
func (RandomDog) Regexes() []string { return nil }
func (RandomDog) Description() string {
	return "Gives you a random dog picture,gif or video from random.dog/woof.json"
}
func (RandomDog) Usage() string          { return "!randomdog" }
func (RandomDog) Timeout() time.Duration { return 20 * time.Second }

// Returns a message with a link to a picture, gif or video of a random dog from random.dog
func (RandomDog) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	type randomDog struct {
		FileSize int    `json:"fileSizeBytes"`
		URL      string `json:"url"`
	}

	resp, err := deps.HTTP.Get("https://random.dog/woof.json")
	if err != nil {
		return adapter.Response{Error: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return adapter.Response{Error: true}, fmt.Errorf("bad status: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var rd randomDog
	if err := json.NewDecoder(resp.Body).Decode(&rd); err != nil {
		return adapter.Response{Error: true}, err
	}

	return adapter.Response{Text: fmt.Sprintf("Heres a random dog. %s", rd.URL)}, nil

}
