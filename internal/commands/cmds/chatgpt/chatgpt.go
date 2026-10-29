package chatgpt

import (
	"context"
	"errors"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// ChatGPT answers a question with OpenAI.
type ChatGPT struct {
	apiKey string
}

func New(apiKey string) *ChatGPT { return &ChatGPT{apiKey: apiKey} }

func (*ChatGPT) Spec() commands.Spec {
	return commands.Spec{Name: "gpt", Description: "Ask a question to ChatGPT.", Usage: "!gpt <message>", Timeout: 30 * time.Second,
		UserCooldown: 4 * time.Second, Concurrency: 16}
}

func (c *ChatGPT) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	resp, err := callChatGPT(ctx, e, c.apiKey)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return commands.Reply{Text: "ChatGPT took too long. Your response has timed out."}, err
		}
		return commands.Reply{Text: "Unable to complete request. Received error response form ChatGPT."}, err
	}

	return commands.Reply{Text: resp}, nil
}
