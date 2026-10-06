package chatgpt

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type ChatGPT struct{}

func (ChatGPT) Name() string           { return "gpt" }
func (ChatGPT) Aliases() []string      { return nil }
func (ChatGPT) Regexes() []string      { return nil }
func (ChatGPT) Description() string    { return "Ask a question to ChatGPT." }
func (ChatGPT) Usage() string          { return "!gpt <message>" }
func (ChatGPT) Timeout() time.Duration { return 30 * time.Second }

func (ChatGPT) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	//Pre processing

	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)

	//Send command

	go func() {
		resp, err := callChatGPT(ctx, e, deps)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- resp
	}()

	//Post processing
	select {
	case <-ctx.Done():
		return adapter.Response{Text: "ChatGPT took too long. Your response has timed out."}, fmt.Errorf("Timeout")

	case err := <-errCh:
		return adapter.Response{Text: "Unable to complete request. Received error response form ChatGPT."}, err

	case resp := <-resultCh:
		return adapter.Response{Text: resp}, nil
	}

}
