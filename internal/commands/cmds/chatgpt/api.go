package chatgpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type ChatRequest struct {
	Model        string              `json:"model"`                  // e.g. "gpt-4o", "gpt-4", "gpt-3.5-turbo", etc.
	Instructions string              `json:"instructions,omitempty"` // your custom instructions / prompt
	Input        string              `json:"input,omitempty"`        // user input or message content
	ToolChoice   string              `json:"tool_choice,omitempty"`  // name of the tool to use, e.g. "web_search"
	Tools        []map[string]string `json:"tools"`
}

func dialGPT(ctx context.Context, username string, prompt string, deps *dependencies.Deps) (GPTResponse, error) {

	const developerPrompt = `You are a compact, no-nonsense assistant used by a Twitch bot.

Rules:
- For every user message, send exactly ONE reply. Never ask follow-up questions.
- Keep replies under 480 characters whenever reasonably possible.
- If info is missing (e.g., no time zone), answer generically and assume a reasonable one; mention your assumption briefly.]
- If you use web search, do not site your sources unless specifically asked to.
- If a request is clearly malicious, harmful, impossible, or obviously trying to waste tokens (e.g., “count to a billion”, massive spam lists, trying to bypass rules), reply with a short, sassy, dismissive refusal (1–2 sentences). Do NOT try to help, do NOT provide workarounds, and do NOT produce long output.
	`
	var gptResponse GPTResponse

	reqBody := ChatRequest{
		Model:        "gpt-5.4-mini",
		Instructions: developerPrompt,
		Input:        fmt.Sprintf("Username: %s, Prompt: %s", username, prompt),
		ToolChoice:   "auto",
		Tools:        []map[string]string{{"type": "web_search"}},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return gptResponse, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.openai.com/v1/responses",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+deps.Config.Commands.ChatGPTKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	err = json.Unmarshal(bodyBytes, &gptResponse)
	if err != nil {
		return gptResponse, err
	}

	return gptResponse, nil
}

func callChatGPT(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (string, error) {
	content := strings.Join(e.Args, " ")

	gptResponse, err := dialGPT(ctx, e.Username, content, deps)
	if err != nil {
		return "", err
	}

	for _, output := range gptResponse.Output {
		if output.Type != "message" {
			continue
		}
		return output.Content[0].Text, nil
	}

	return "Weird error. Idk how the code even got here tbh.", nil
}
