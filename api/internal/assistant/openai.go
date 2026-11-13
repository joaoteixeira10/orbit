package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// openAI answers with a model behind an OpenAI-style chat API — OpenAI's
// own, or a server that speaks it (Ollama's /v1, LM Studio, vLLM…): a loop of
// streamed chat/completions while the model asks for tools, its words passed
// on as they come.
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
	// ExtraContent: what a service adds to a call, given back as it came —
	// Gemini's thought_signature, without which it refuses the next round.
	ExtraContent json.RawMessage `json:"extra_content,omitempty"`
}

func str(s string) *string { return &s }

// HTTPError: an OpenAI-style server said no.
type HTTPError struct {
	Status int
	Msg    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("the model's server said: %s (HTTP %d)", e.Msg, e.Status)
}

// Busy: the service is over its load or its limit for now.
func (e *HTTPError) Busy() bool {
	return e.Status == http.StatusServiceUnavailable || e.Status == http.StatusTooManyRequests
}

// BadKey: the server refused the key (Gemini says so with a 400).
func (e *HTTPError) BadKey() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden ||
		strings.Contains(strings.ToLower(e.Msg), "api key not valid")
}

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
		body := map[string]any{"model": m.model, "messages": msgs, "stream": true}
		if len(tools) > 0 {
			body["tools"] = tools
		}
		ch, err := m.retried(ctx, body, r.Delta)
		if err != nil {
			return "", err
		}
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

type oaChoice struct {
	Message      oaMessage `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

// retryWaits: how long to wait before asking again when the service is
// busy (503, 429) — before any of the answer has come.
var retryWaits = []time.Duration{2 * time.Second, 5 * time.Second}

// retried is round, asked again while the service is busy.
func (m *openAI) retried(ctx context.Context, body any, delta func(string)) (oaChoice, error) {
	for i := 0; ; i++ {
		ch, err := m.round(ctx, body, delta)
		var he *HTTPError
		if i >= len(retryWaits) || !errors.As(err, &he) || !he.Busy() {
			return ch, err
		}
		select {
		case <-ctx.Done():
			return ch, err
		case <-time.After(retryWaits[i]):
		}
	}
}

// round is one chat/completions: streamed (each piece of text to delta, the
// tool calls put together from theirs), or whole from a server that
// doesn't stream.
func (m *openAI) round(ctx context.Context, body any, delta func(string)) (oaChoice, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return oaChoice{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return oaChoice{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.key != "" {
		req.Header.Set("Authorization", "Bearer "+m.key)
	}
	res, err := m.http.Do(req)
	if err != nil {
		return oaChoice{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		type oaError struct {
			Error any `json:"error"`
		}
		var e oaError
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		if err := json.Unmarshal(raw, &e); err != nil {
			var list []oaError // Gemini's: [{"error":{…}}]
			if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
				e = list[0]
			}
		}
		msg := http.StatusText(res.StatusCode)
		switch v := e.Error.(type) {
		case string:
			msg = v
		case map[string]any:
			if s, ok := v["message"].(string); ok {
				msg = s
			}
		}
		return oaChoice{}, &HTTPError{Status: res.StatusCode, Msg: strings.TrimSpace(msg)}
	}

	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		var whole struct{ Choices []oaChoice }
		if err := json.NewDecoder(res.Body).Decode(&whole); err != nil {
			return oaChoice{}, err
		}
		if len(whole.Choices) == 0 {
			return oaChoice{}, fmt.Errorf("the model sent no answer")
		}
		return whole.Choices[0], nil
	}

	var out oaChoice
	var text strings.Builder
	got := false
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
						ExtraContent json.RawMessage `json:"extra_content"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		got = true
		c := chunk.Choices[0]
		if c.Delta.Content != "" {
			text.WriteString(c.Delta.Content)
			if delta != nil {
				delta(c.Delta.Content)
			}
		}
		for _, tc := range c.Delta.ToolCalls {
			i := tc.Index
			// A new call under an index already taken (Gemini may number
			// calls sent whole, side by side, all 0): one more.
			if i < len(out.Message.ToolCalls) && tc.ID != "" && out.Message.ToolCalls[i].ID != "" && out.Message.ToolCalls[i].ID != tc.ID {
				i = len(out.Message.ToolCalls)
			}
			for len(out.Message.ToolCalls) <= i {
				out.Message.ToolCalls = append(out.Message.ToolCalls, oaToolCall{Type: "function"})
			}
			call := &out.Message.ToolCalls[i]
			if tc.ID != "" {
				call.ID = tc.ID
			}
			call.Function.Name += tc.Function.Name
			call.Function.Arguments += tc.Function.Arguments
			if len(tc.ExtraContent) > 0 && string(tc.ExtraContent) != "null" {
				call.ExtraContent = tc.ExtraContent
			}
		}
		if c.FinishReason != "" {
			out.FinishReason = c.FinishReason
		}
	}
	if err := sc.Err(); err != nil {
		return oaChoice{}, err
	}
	if !got {
		return oaChoice{}, fmt.Errorf("the model sent no answer")
	}
	out.Message.Role = "assistant"
	if text.Len() > 0 || len(out.Message.ToolCalls) == 0 {
		out.Message.Content = str(text.String())
	}
	return out, nil
}
