package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestOpenAIErrors: a refused key reads as one, Gemini's 400 included.
func TestOpenAIErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		badKey bool
	}{
		{http.StatusUnauthorized, `{"error":{"message":"Incorrect API key provided"}}`, true},
		{http.StatusBadRequest, `[{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}]`, true},
		{http.StatusBadRequest, `{"error":{"message":"API key not valid. Please pass a valid API key."}}`, true},
		{http.StatusBadRequest, `{"error":"bad request"}`, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		p, err := Config{Provider: OpenAI, URL: srv.URL, Model: "m", Key: "k"}.New(srv.Client())
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Answer(context.Background(), Request{History: []Turn{{Role: "user", Text: "hi"}}})
		srv.Close()
		var he *HTTPError
		if !errors.As(err, &he) || he.Status != tc.status || he.BadKey() != tc.badKey {
			t.Errorf("%d %s: %v", tc.status, tc.body, err)
		}
	}
}

// TestGeminiConfig: Gemini needs a key and goes to Google's address.
func TestGeminiConfig(t *testing.T) {
	if _, err := (Config{Provider: Gemini, Model: "gemini-flash-latest"}).New(http.DefaultClient); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("no key: %v", err)
	}
	p, err := Config{Provider: Gemini, Model: "gemini-flash-latest", Key: "k", URL: "http://elsewhere"}.New(http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if o := p.(*openAI); o.base != GeminiURL {
		t.Fatalf("base: %s", o.base)
	}
}

// TestOpenAIBusy: a busy service is asked again, then its no is passed on.
func TestOpenAIBusy(t *testing.T) {
	old := retryWaits
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { retryWaits = old }()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"This model is currently experiencing high demand."}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	p, _ := Config{Provider: OpenAI, URL: srv.URL, Model: "m"}.New(srv.Client())
	got, err := p.Answer(context.Background(), Request{History: []Turn{{Role: "user", Text: "hi"}}})
	if err != nil || got != "hello" || calls != 3 {
		t.Fatalf("%q %v after %d calls", got, err, calls)
	}
	calls = -10 // busy for longer than it waits
	_, err = p.Answer(context.Background(), Request{History: []Turn{{Role: "user", Text: "hi"}}})
	var he *HTTPError
	if !errors.As(err, &he) || !he.Busy() || calls != -7 {
		t.Fatalf("still busy: %v after %d", err, calls)
	}
}

// TestOpenAIThoughtSignature: what Gemini adds to a tool call (its thought
// signature) goes back with the call; calls sent whole under one index are
// kept apart.
func TestOpenAIThoughtSignature(t *testing.T) {
	var second map[string]any
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		if calls == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"get_today","arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig-1"}}}]}}]}` + "\n\n" +
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c2","type":"function","function":{"name":"list_tasks","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&second)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"Nothing today."},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	p, _ := Config{Provider: OpenAI, URL: srv.URL, Model: "m"}.New(srv.Client())
	ran := []string{}
	got, err := p.Answer(context.Background(), Request{
		History: []Turn{{Role: "user", Text: "today?"}},
		Tools:   []Tool{{Name: "get_today"}, {Name: "list_tasks"}},
		Call: func(_ context.Context, name string, _ map[string]any) (string, bool) {
			ran = append(ran, name)
			return "ok", false
		},
	})
	if err != nil || got != "Nothing today." || len(ran) != 2 {
		t.Fatalf("%q %v ran %v", got, err, ran)
	}
	b, _ := json.Marshal(second["messages"])
	if !strings.Contains(string(b), `"extra_content":{"google":{"thought_signature":"sig-1"}}`) || !strings.Contains(string(b), `"tool_call_id":"c2"`) {
		t.Fatalf("second round: %s", b)
	}
}
