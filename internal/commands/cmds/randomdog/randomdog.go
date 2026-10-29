package randomdog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// RandomDog links a random dog picture, gif or video from random.dog.
type RandomDog struct {
	http *http.Client
}

func New(http *http.Client) *RandomDog { return &RandomDog{http: http} }

func (*RandomDog) Spec() commands.Spec {
	return commands.Spec{
		Name:        "randomdog",
		Description: "Gives you a random dog picture,gif or video from random.dog/woof.json",
		Usage:       "!randomdog",
		Timeout:     20 * time.Second,
		Concurrency: 16,
	}
}

func (d *RandomDog) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	type randomDog struct {
		FileSize int    `json:"fileSizeBytes"`
		URL      string `json:"url"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://random.dog/woof.json", nil)
	if err != nil {
		return commands.Reply{}, err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return commands.Reply{}, fmt.Errorf("random.dog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return commands.Reply{}, fmt.Errorf("random.dog: bad status: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var rd randomDog
	if err := json.NewDecoder(resp.Body).Decode(&rd); err != nil {
		return commands.Reply{}, fmt.Errorf("random.dog: %w", err)
	}

	return commands.Reply{Text: fmt.Sprintf("Heres a random dog. %s", rd.URL)}, nil
}
