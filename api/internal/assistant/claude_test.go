package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClaudeLoop: against a stand-in for the Messages API, Claude asks for a
// tool, gets its answer back as a tool_result (after its own turn, unchanged),
// and answers; the server-side fallback is asked for; a refusal is said so.
func TestClaudeLoop(t *testing.T) {
	var bodies []map[string]any
	var betas []string
	refuse := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		betas = append(betas, r.Header.Get("anthropic-beta"))
		w.Header().Set("Content-Type", "application/json")
		msgs := body["messages"].([]any)
		last := msgs[len(msgs)-1].(map[string]any)
		switch {
		case refuse:
			_, _ = io.WriteString(w, `{"id":"m3","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":"refusal","usage":{"input_tokens":1,"output_tokens":1}}`)
		case strings.Contains(mustJSON(last), "tool_result"):
			_, _ = io.WriteString(w, `{"id":"m2","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"Two workspaces."}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		default:
			_, _ = io.WriteString(w, `{"id":"m1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"","signature":"sig"},{"type":"tool_use","id":"tu1","name":"list_workspaces","input":{"query":"all"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
		}
	}))
	defer srv.Close()

	p, err := Config{Provider: Anthropic, Model: "claude-opus-5-5", Key: "k", URL: srv.URL}.New(srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	var steps []Step
	ran := ""
	text, err := p.Answer(context.Background(), Request{
		System: "rules", Context: "now",
		History: []Turn{{Role: "user", Text: "hi"}, {Role: "assistant", Text: "hello"}, {Role: "user", Text: "how many?"}},
		Tools:   []Tool{{Name: "list_workspaces", Description: "d", Schema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}}},
		Step:    func(s Step) { steps = append(steps, s) },
		Call: func(_ context.Context, name string, args map[string]any) (string, bool) {
			ran = name + ":" + args["query"].(string)
			return `[{"name":"A"},{"name":"B"}]`, false
		},
	})
	if err != nil || text != "Two workspaces." || ran != "list_workspaces:all" || len(steps) != 1 || steps[0].Hint != "all" {
		t.Fatalf("answer %q err %v ran %q steps %+v", text, err, ran, steps)
	}
	second := bodies[1]["messages"].([]any)
	if len(second) != 5 {
		t.Fatalf("second request: %d messages", len(second))
	}
	turn := mustJSON(second[3])
	result := mustJSON(second[4])
	if !strings.Contains(turn, `"signature":"sig"`) || !strings.Contains(turn, `"tool_use"`) || !strings.Contains(result, `"tool_use_id":"tu1"`) {
		t.Errorf("assistant turn %s / result %s", turn, result)
	}
	if bodies[0]["fallbacks"] != "default" || !strings.Contains(betas[0], "server-side-fallback-2026-07-01") {
		t.Errorf("fallback: %v %q", bodies[0]["fallbacks"], betas[0])
	}
	sys := mustJSON(bodies[0]["system"])
	if !strings.Contains(sys, `"cache_control"`) || !strings.Contains(sys, `"now"`) {
		t.Errorf("system: %s", sys)
	}

	refuse = true
	if _, err := p.Answer(context.Background(), Request{System: "r", History: []Turn{{Role: "user", Text: "x"}}}); err != ErrRefused {
		t.Errorf("refusal: %v", err)
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
