package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeModel is a stand-in for an OpenAI-style server: asked first, it calls
// list_workspaces; given the tool's answer, it says what it found. It keeps
// what it was sent.
type fakeModel struct {
	mu    sync.Mutex
	asked []map[string]any
}

func (f *fakeModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.asked = append(f.asked, body)
	f.mu.Unlock()
	msgs, _ := body["messages"].([]any)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	w.Header().Set("Content-Type", "application/json")
	if last["role"] == "tool" {
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":`+jsonString("You have: "+last["content"].(string))+`}}]}`)
		return
	}
	_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"list_workspaces","arguments":"{}"}}]}}]}`)
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

// sseEvents reads a text/event-stream answer into its events.
func sseEvents(t *testing.T, body string) []assistantEvent {
	t.Helper()
	var out []assistantEvent
	for _, line := range strings.Split(body, "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var e assistantEvent
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				t.Fatalf("event %q: %v", data, err)
			}
			out = append(out, e)
		}
	}
	return out
}

// TestAssistantConfig: off until the owner sets it up; only the owner sees
// or changes it; Claude needs a key, which is never sent back.
func TestAssistantConfig(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var st assistantStatusResponse
	decode(t, authdo(r, "GET", "/api/v1/assistant", "", owner), &st)
	if st.Enabled {
		t.Fatal("on by default")
	}
	createUserVia(t, r, owner, `{"username":"ana","email":"ana@example.org","password":"anapassword","role_ids":[]}`)
	ana := loginTok(t, r, "ana", "anapassword")
	expect(t, authdo(r, "GET", "/api/v1/assistant/config", "", ana), http.StatusForbidden, "a member reads the set-up")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"anthropic"}`, owner), http.StatusBadRequest, "Claude with no key")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"ollama","url":"ftp://x"}`, owner), http.StatusBadRequest, "not http")

	var cfg assistantConfigResponse
	decode(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"anthropic","key":"sk-ant-test"}`, owner), &cfg)
	if !cfg.HasKey || cfg.Model != "claude-opus-5-5" || strings.Contains(authdo(r, "GET", "/api/v1/assistant/config", "", owner).Body.String(), "sk-ant-test") {
		t.Fatalf("set-up: %+v", cfg)
	}
	// Saving again without a key keeps it; clear_key removes it.
	decode(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"anthropic"}`, owner), &cfg)
	if !cfg.HasKey {
		t.Fatal("the key went")
	}
	decode(t, authdo(r, "GET", "/api/v1/assistant", "", ana), &st)
	if !st.Enabled || st.Model != "claude-opus-5-5" {
		t.Fatalf("status: %+v", st)
	}
	decode(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":false,"provider":"anthropic","clear_key":true}`, owner), &cfg)
	if cfg.HasKey || cfg.Enabled {
		t.Fatalf("cleared: %+v", cfg)
	}
	expect(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"hi"}`, ana), http.StatusForbidden, "asking while off")
}

// TestAssistantAsk: a message is answered with the tools that read, run as
// the person; the conversation is kept, continues, and can be deleted; the
// owner's daily limit stops the next one.
func TestAssistantAsk(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	fake := &fakeModel{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	old := assistantHTTP
	assistantHTTP = func() *http.Client { return srv.Client() }
	defer func() { assistantHTTP = old }()

	owner := loginTok(t, r, "admin", "adminpass")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"ollama","url":"`+srv.URL+`/v1","model":"llama3.1","daily_limit":2}`, owner), http.StatusOK, "set up")
	createWS(t, r, owner, `{"name":"Holiday plans","kind":"personal"}`)
	createUserVia(t, r, owner, `{"username":"bea","email":"bea@example.org","password":"beapassword","role_ids":[]}`)
	bea := loginTok(t, r, "bea", "beapassword")

	w := authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"What workspaces do I have?","context":{"path":"/","title":"Today","now":"Thursday 5 November 2026, 10:00","time_zone":"Europe/Lisbon"}}`, bea)
	expect(t, w, http.StatusOK, "ask")
	ev := sseEvents(t, w.Body.String())
	if len(ev) != 3 || ev[0].Type != "step" || ev[0].Tool != "list_workspaces" || ev[1].Type != "answer" || ev[2].Type != "done" || ev[2].ChatID == "" {
		t.Fatalf("events: %+v", ev)
	}
	// The tool ran as Bea: she isn't in the owner's workspace.
	if !strings.HasPrefix(ev[1].Text, "You have:") || strings.Contains(ev[1].Text, "Holiday plans") {
		t.Errorf("answer: %q", ev[1].Text)
	}
	// Ask mode offers only the tools that read; the model is told the rules
	// and what's on screen.
	first := fake.asked[0]
	names := map[string]bool{}
	for _, x := range first["tools"].([]any) {
		names[x.(map[string]any)["function"].(map[string]any)["name"].(string)] = true
	}
	if !names["search"] || !names["list_workspaces"] || names["create_task"] || names["delete_task"] || names["update_page"] {
		t.Errorf("tools offered: %v", names)
	}
	sys := first["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "Ask mode") || !strings.Contains(sys, "@bea") || !strings.Contains(sys, "Europe/Lisbon") || !strings.Contains(sys, `"Today" at /`) {
		t.Errorf("system: %s", sys)
	}

	// The conversation is kept and carries on.
	chatID := ev[2].ChatID
	w = authdo(r, "POST", "/api/v1/assistant/chat", `{"chat_id":"`+chatID+`","message":"And again?"}`, bea)
	expect(t, w, http.StatusOK, "ask again")
	third := fake.asked[2]["messages"].([]any)
	if len(third) != 4 || third[1].(map[string]any)["content"] != "What workspaces do I have?" || third[3].(map[string]any)["content"] != "And again?" {
		t.Errorf("history sent: %v", third)
	}
	var chat assistantChatResponse
	decode(t, authdo(r, "GET", "/api/v1/assistant/chats/"+chatID, "", bea), &chat)
	if len(chat.Turns) != 4 || chat.Title != "What workspaces do I have?" || len(chat.Turns[1].Steps) != 1 {
		t.Fatalf("kept: %+v", chat)
	}
	expect(t, authdo(r, "GET", "/api/v1/assistant/chats/"+chatID, "", owner), http.StatusNotFound, "someone else's conversation")
	// Asked by the owner, the same tool sees the owner's workspace.
	ev = sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"Mine?"}`, owner).Body.String())
	if len(ev) != 3 || !strings.Contains(ev[1].Text, "Holiday plans") {
		t.Errorf("owner's answer: %+v", ev)
	}

	// Two a day: the third waits for tomorrow.
	expect(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"Once more"}`, bea), http.StatusTooManyRequests, "over the limit")
	var st assistantStatusResponse
	decode(t, authdo(r, "GET", "/api/v1/assistant", "", bea), &st)
	if st.UsedToday != 2 || st.DailyLimit != 2 {
		t.Errorf("status: %+v", st)
	}

	expect(t, authdo(r, "DELETE", "/api/v1/assistant/chats/"+chatID, "", bea), http.StatusNoContent, "delete")
	var list []assistantChatSummary
	decode(t, authdo(r, "GET", "/api/v1/assistant/chats", "", bea), &list)
	if len(list) != 0 {
		t.Errorf("left: %+v", list)
	}
}
