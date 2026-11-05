package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// openAI answers with a model behind an OpenAI-style chat API — OpenAI's
// own, or a server that speaks it (Ollama's /v1, LM Studio, vLLM…): a loop of
// chat/completions while the model asks for tools.
type openAI struct {
	base  string // …/v1
	key   string // "" for a server without one (Ollama)
	model string
	http  *http.Client
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func str(s string) *string { return &s }

func (m *openAI) Answer(ctx context.Context, r Request) (string, error) {
	tools := make([]map[string]any, 0, len(r.Tools))
	for _, t := range r.Tools {
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": t.Name, "description": t.Description, "parameters": t.Schema,
		}})
	}
	system := r.System
	if r.Context != "" {
		system += "\n\n" + r.Context
	}
	msgs := []oaMessage{{Role: "system", Content: str(system)}}
	for _, t := range r.History {
		role := "user"
		if t.Role == "assistant" {
			role = "assistant"
		}
		msgs = append(msgs, oaMessage{Role: role, Content: str(t.Text)})
	}

	for round := 0; ; round++ {
		body := map[string]any{"model": m.model, "messages": msgs}
		if len(tools) > 0 {
			body["tools"] = tools
		}
		var resp struct {
			Choices []struct {
				Message      oaMessage `json:"message"`
				FinishReason string    `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := m.post(ctx, body, &resp); err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("the model sent no answer")
		}
		ch := resp.Choices[0]
		text := ""
		if ch.Message.Content != nil {
			text = *ch.Message.Content
		}
		if len(ch.Message.ToolCalls) == 0 {
			if ch.FinishReason == "length" && text == "" {
				return "", ErrTooLong
			}
			return text, nil
		}
		if round+1 >= MaxRounds {
			return "", ErrTooMany
		}
		msgs = append(msgs, oaMessage{Role: "assistant", Content: ch.Message.Content, ToolCalls: ch.Message.ToolCalls})
		for _, tc := range ch.Message.ToolCalls {
			var args map[string]any
			if json.Unmarshal([]byte(tc.Function.Arguments), &args) != nil || args == nil {
				args = map[string]any{}
			}
			if r.Step != nil {
				r.Step(Step{Tool: tc.Function.Name, Hint: hint(args)})
			}
			out, isErr := r.Call(ctx, tc.Function.Name, args)
			if isErr {
				out = "Error: " + out
			}
			msgs = append(msgs, oaMessage{Role: "tool", ToolCallID: tc.ID, Content: str(clipResult(out))})
		}
	}
}

func (m *openAI) post(ctx context.Context, body any, into any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.key != "" {
		req.Header.Set("Authorization", "Bearer "+m.key)
	}
	res, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		var e struct {
			Error any `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		msg := http.StatusText(res.StatusCode)
		switch v := e.Error.(type) {
		case string:
			msg = v
		case map[string]any:
			if s, ok := v["message"].(string); ok {
				msg = s
			}
		}
		return fmt.Errorf("the model's server said: %s (HTTP %d)", strings.TrimSpace(msg), res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(into)
}
