package chatgpt

import (
	"context"
	"errors"
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

	resp, err := callChatGPT(ctx, e, deps)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return adapter.Response{Text: "ChatGPT took too long. Your response has timed out."}, err
		}
		return adapter.Response{Text: "Unable to complete request. Received error response form ChatGPT."}, err
	}

	return adapter.Response{Text: resp}, nil
}
