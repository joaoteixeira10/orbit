package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ollama answers with a model on an Ollama server, through Ollama's own API
// (/api/chat), not its OpenAI-style one: only that lets a request say how
// much the model reads (num_ctx). Ollama's default — a few thousand tokens —
// silently drops the start of Orbit's requests (the rules, most tools), and
// the model is lost. A loop of streamed chats while the model asks for tools.
type ollama struct {
	root  string // http://host:11434
	key   string // for an Ollama behind a proxy that wants one
	model string
	ctx   int // num_ctx
	http  *http.Client
}

// DefaultContext is how much an Ollama model reads of a request unless the
// owner says otherwise: Orbit's rules and tools are some 7,000 tokens, and a
// page or a tool's answer adds to it.
const DefaultContext = 16384

type olMessage struct {
	Role      string       `json:"role"`
	Content   string       `json:"content"`
	ToolCalls []olToolCall `json:"tool_calls,omitempty"`
	ToolName  string       `json:"tool_name,omitempty"`
}

type olToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

func (m *ollama) Answer(ctx context.Context, r Request) (string, error) {
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
	msgs := []olMessage{{Role: "system", Content: system}}
	for _, t := range r.History {
		role := "user"
		if t.Role == "assistant" {
			role = "assistant"
		}
		msgs = append(msgs, olMessage{Role: role, Content: t.Text})
	}

	for round := 0; ; round++ {
		body := map[string]any{"model": m.model, "messages": msgs, "stream": true, "options": map[string]any{"num_ctx": m.ctx}}
		if len(tools) > 0 {
			body["tools"] = tools
		}
		msg, reason, err := m.chat(ctx, body, r.Delta)
		if err != nil {
			return "", err
		}
		if len(msg.ToolCalls) == 0 {
			if reason == "length" && msg.Content == "" {
				return "", ErrTooLong
			}
			return msg.Content, nil
		}
		if round+1 >= MaxRounds {
			return "", ErrTooMany
		}
		msgs = append(msgs, msg)
		for _, tc := range msg.ToolCalls {
			args := tc.Function.Arguments
			if args == nil {
				args = map[string]any{}
			}
			if r.Step != nil {
				r.Step(Step{Tool: tc.Function.Name, Hint: hint(args)})
			}
			out, isErr := r.Call(ctx, tc.Function.Name, args)
			if isErr {
				out = "Error: " + out
			}
			msgs = append(msgs, olMessage{Role: "tool", ToolName: tc.Function.Name, Content: clipResult(out)})
		}
	}
}

// chat is one /api/chat, read as it streams (a JSON object a line): the
// text to delta as it comes, the tool calls as they're given.
func (m *ollama) chat(ctx context.Context, body any, delta func(string)) (olMessage, string, error) {
	out := olMessage{Role: "assistant"}
	b, err := json.Marshal(body)
	if err != nil {
		return out, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.root+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return out, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.key != "" {
		req.Header.Set("Authorization", "Bearer "+m.key)
	}
	res, err := m.http.Do(req)
	if err != nil {
		return out, "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		if e.Error == "" {
			e.Error = http.StatusText(res.StatusCode)
		}
		return out, "", fmt.Errorf("the model's server said: %s (HTTP %d)", strings.TrimSpace(e.Error), res.StatusCode)
	}
	var text strings.Builder
	reason := ""
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var line struct {
			Message    olMessage `json:"message"`
			Done       bool      `json:"done"`
			DoneReason string    `json:"done_reason"`
			Error      string    `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.Error != "" {
			return out, "", fmt.Errorf("the model's server said: %s", line.Error)
		}
		if c := line.Message.Content; c != "" {
			text.WriteString(c)
			if delta != nil {
				delta(c)
			}
		}
		out.ToolCalls = append(out.ToolCalls, line.Message.ToolCalls...)
		if line.Done {
			reason = line.DoneReason
			break
		}
	}
	if err := sc.Err(); err != nil {
		return out, "", err
	}
	out.Content = text.String()
	return out, reason, nil
}
