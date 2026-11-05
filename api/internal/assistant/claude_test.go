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

// TestClaudeLoop: against a stand-in for the Messages API (streamed), Claude
// asks for a tool, gets its answer back as a tool_result (after its own turn,
// unchanged), and answers word by word; the server-side fallback is asked
// for; a refusal is said so.
func TestClaudeLoop(t *testing.T) {
	var bodies []map[string]any
	var betas []string
	refuse := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		betas = append(betas, r.Header.Get("anthropic-beta"))
		w.Header().Set("Content-Type", "text/event-stream")
		msgs := body["messages"].([]any)
		last := msgs[len(msgs)-1].(map[string]any)
		switch {
		case refuse:
			sse(w, start("m3"), stop("refusal"), `{"type":"message_stop"}`)
		case strings.Contains(mustJSON(last), "tool_result"):
			sse(w, start("m2"),
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Two "}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"workspaces."}}`,
				`{"type":"content_block_stop","index":0}`,
				stop("end_turn"), `{"type":"message_stop"}`)
		default:
			sse(w, start("m1"),
				`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu1","name":"list_workspaces","input":{}}}`,
				`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`,
				`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"all\"}"}}`,
				`{"type":"content_block_stop","index":1}`,
				stop("tool_use"), `{"type":"message_stop"}`)
		}
	}))
	defer srv.Close()

	p, err := Config{Provider: Anthropic, Model: "claude-opus-5-5", Key: "k", URL: srv.URL}.New(srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	var steps []Step
	var words []string
	ran := ""
	text, err := p.Answer(context.Background(), Request{
		System: "rules", Context: "now",
		History: []Turn{{Role: "user", Text: "hi"}, {Role: "assistant", Text: "hello"}, {Role: "user", Text: "how many?"}},
		Tools:   []Tool{{Name: "list_workspaces", Description: "d", Schema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}}},
		Step:    func(s Step) { steps = append(steps, s) },
		Delta:   func(s string) { words = append(words, s) },
		Call: func(_ context.Context, name string, args map[string]any) (string, bool) {
			ran = name + ":" + args["query"].(string)
			return `[{"name":"A"},{"name":"B"}]`, false
		},
	})
	if err != nil || text != "Two workspaces." || ran != "list_workspaces:all" || len(steps) != 1 || steps[0].Hint != "all" {
		t.Fatalf("answer %q err %v ran %q steps %+v", text, err, ran, steps)
	}
	if strings.Join(words, "|") != "Two |workspaces." || bodies[0]["stream"] != true {
		t.Errorf("streamed: %q, stream %v", words, bodies[0]["stream"])
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

// sse writes a Messages API stream: each event as its type and data.
func sse(w io.Writer, events ...string) {
	for _, e := range events {
		var ev struct{ Type string }
		_ = json.Unmarshal([]byte(e), &ev)
		_, _ = io.WriteString(w, "event: "+ev.Type+"\ndata: "+e+"\n\n")
	}
}

func start(id string) string {
	return `{"type":"message_start","message":{"id":"` + id + `","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`
}

func stop(reason string) string {
	return `{"type":"message_delta","delta":{"stop_reason":"` + reason + `"},"usage":{"output_tokens":1}}`
}

func TestOllamaBase(t *testing.T) {
	for in, want := range map[string]string{
		"http://10.0.0.5:11434":     "http://10.0.0.5:11434/v1",
		"http://10.0.0.5:11434/":    "http://10.0.0.5:11434/v1",
		"http://10.0.0.5:11434/v1":  "http://10.0.0.5:11434/v1",
		"https://ai.lan/ollama/v1/": "https://ai.lan/ollama/v1",
	} {
		if got := OllamaBase(in); got != want {
			t.Errorf("OllamaBase(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestOpenAIWhole: a server that answers whole, not streamed, still works.
func TestOpenAIWhole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Hello."}}]}`)
	}))
	defer srv.Close()
	p, _ := Config{Provider: OpenAI, Model: "m", URL: srv.URL + "/v1"}.New(srv.Client())
	text, err := p.Answer(context.Background(), Request{System: "r", History: []Turn{{Role: "user", Text: "hi"}}})
	if err != nil || text != "Hello." {
		t.Fatalf("answer %q err %v", text, err)
	}
}
