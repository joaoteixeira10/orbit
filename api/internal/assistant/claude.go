package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// claude answers with Claude, through Anthropic's SDK: a loop of
// messages.create while the model asks for tools.
type claude struct {
	client anthropic.Client
	model  string
}

// Models that may decline on safety grounds and take Anthropic's server-side
// fallback ("default": the API picks a model by the reason it declined).
var fallbackModels = map[string]bool{
	"claude-fable-5-1": true, "claude-opus-5-5": true, "claude-opus-5": true, "claude-sonnet-5-5": true,
}

func newClaude(c Config, httpc *http.Client) *claude {
	opts := []option.RequestOption{option.WithAPIKey(c.Key), option.WithHTTPClient(httpc), option.WithRequestTimeout(Timeout)}
	if c.URL != "" {
		opts = append(opts, option.WithBaseURL(c.URL))
	}
	if fallbackModels[c.Model] {
		opts = append(opts,
			option.WithHeaderAdd("anthropic-beta", "server-side-fallback-2026-07-01"),
			option.WithJSONSet("fallbacks", "default"))
	}
	return &claude{client: anthropic.NewClient(opts...), model: c.Model}
}

func (m *claude) Answer(ctx context.Context, r Request) (string, error) {
	tools := make([]anthropic.ToolUnionParam, 0, len(r.Tools))
	for _, t := range r.Tools {
		schema := anthropic.ToolInputSchemaParam{Properties: t.Schema["properties"]}
		if req, ok := t.Schema["required"].([]string); ok {
			schema.Required = req
		}
		tp := anthropic.ToolParam{Name: t.Name, Description: anthropic.String(t.Description), InputSchema: schema}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &tp})
	}
	// Tools, then the steady part of the system prompt, are cached; what
	// changes with each message (now, what's on screen) comes after.
	system := []anthropic.TextBlockParam{{Text: r.System, CacheControl: anthropic.NewCacheControlEphemeralParam()}}
	if r.Context != "" {
		system = append(system, anthropic.TextBlockParam{Text: r.Context})
	}
	var msgs []anthropic.MessageParam
	for _, t := range r.History {
		if t.Role == "assistant" {
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(t.Text)))
		} else {
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(t.Text)))
		}
	}

	for round := 0; ; round++ {
		resp, err := m.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     m.model,
			MaxTokens: 16000,
			System:    system,
			Messages:  msgs,
			Tools:     tools,
		})
		if err != nil {
			return "", err
		}
		if resp.StopReason == anthropic.StopReasonRefusal {
			return "", ErrRefused
		}
		// The answer goes back as it came (thinking included); a fallback's
		// marker block, which the API doesn't need again, is left out.
		var back []anthropic.ContentBlockParamUnion
		var text strings.Builder
		var results []anthropic.ContentBlockParamUnion
		for _, b := range resp.Content {
			switch v := b.AsAny().(type) {
			case anthropic.TextBlock:
				p := v.ToParam()
				back = append(back, anthropic.ContentBlockParamUnion{OfText: &p})
				text.WriteString(v.Text)
			case anthropic.ThinkingBlock:
				p := v.ToParam()
				back = append(back, anthropic.ContentBlockParamUnion{OfThinking: &p})
			case anthropic.RedactedThinkingBlock:
				p := v.ToParam()
				back = append(back, anthropic.ContentBlockParamUnion{OfRedactedThinking: &p})
			case anthropic.ToolUseBlock:
				p := v.ToParam()
				back = append(back, anthropic.ContentBlockParamUnion{OfToolUse: &p})
				var args map[string]any
				if json.Unmarshal([]byte(v.JSON.Input.Raw()), &args) != nil || args == nil {
					args = map[string]any{}
				}
				if r.Step != nil {
					r.Step(Step{Tool: v.Name, Hint: hint(args)})
				}
				out, isErr := r.Call(ctx, v.Name, args)
				results = append(results, anthropic.NewToolResultBlock(v.ID, clipResult(out), isErr))
			}
		}
		switch resp.StopReason {
		case anthropic.StopReasonToolUse:
		case anthropic.StopReasonMaxTokens:
			if text.Len() == 0 {
				return "", ErrTooLong
			}
			return text.String(), nil
		default:
			return text.String(), nil
		}
		if round+1 >= MaxRounds {
			return "", ErrTooMany
		}
		msgs = append(msgs, anthropic.NewAssistantMessage(back...), anthropic.NewUserMessage(results...))
	}
}
