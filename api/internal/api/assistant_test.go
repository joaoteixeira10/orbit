package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/assistant"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
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
	// Streamed, as Ollama and OpenAI do: the answer in two pieces, the tool
	// call's arguments split across chunks.
	w.Header().Set("Content-Type", "text/event-stream")
	chunk := func(delta, finish string) {
		fin := "null"
		if finish != "" {
			fin = `"` + finish + `"`
		}
		_, _ = io.WriteString(w, `data: {"choices":[{"index":0,"delta":`+delta+`,"finish_reason":`+fin+`}]}`+"\n\n")
	}
	// "plan:<workspace id>" asks for changes: a list, a task in it (by the
	// list's new:1), and one more task.
	if ws, ok := strings.CutPrefix(fmt.Sprint(last["content"]), "plan:"); ok && last["role"] == "user" {
		calls := []string{
			`{"name":"create_task_list","arguments":` + jsonString(`{"workspace_id":"`+ws+`","name":"Launch","summary":"Make the Launch list"}`) + `}`,
			`{"name":"create_task","arguments":` + jsonString(`{"workspace_id":"`+ws+`","list_id":"new:1","title":"Book the room","summary":"Add Book the room to it"}`) + `}`,
			`{"name":"create_task","arguments":` + jsonString(`{"workspace_id":"`+ws+`","title":"Unwanted","summary":"Add Unwanted"}`) + `}`,
		}
		for i, fn := range calls {
			chunk(fmt.Sprintf(`{"tool_calls":[{"index":%d,"id":"p%d","type":"function","function":%s}]}`, i, i, fn), "")
		}
		chunk(`{}`, "tool_calls")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		return
	}
	// "claim:<workspace id>": it first only says it lined a task up; told
	// it didn't, it calls the tool.
	if strings.HasPrefix(fmt.Sprint(last["content"]), "claim:") && last["role"] == "user" {
		chunk(`{"role":"assistant","content":"I've lined up a task for you."}`, "stop")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		return
	}
	if strings.HasPrefix(fmt.Sprint(last["content"]), "(From Orbit") {
		ws, _ := strings.CutPrefix(fmt.Sprint(msgs[len(msgs)-3].(map[string]any)["content"]), "claim:")
		chunk(`{"tool_calls":[{"index":0,"id":"k1","type":"function","function":{"name":"create_task","arguments":`+jsonString(`{"workspace_id":"`+ws+`","title":"Buy milk","summary":"Add Buy milk"}`)+`}}]}`, "tool_calls")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		return
	}
	if last["role"] == "tool" {
		chunk(`{"role":"assistant","content":"You have: "}`, "")
		chunk(`{"content":`+jsonString(last["content"].(string))+`}`, "")
		chunk(`{}`, "stop")
	} else {
		chunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"list_workspaces","arguments":"{"}}]}`, "")
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}`, "")
		chunk(`{}`, "tool_calls")
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

// sseEvents reads a text/event-stream answer into its events, the answer's
// pieces (delta) left out.
func sseEvents(t *testing.T, body string) []assistantEvent {
	t.Helper()
	return sseAll(t, body, false)
}

func sseAll(t *testing.T, body string, deltas bool) []assistantEvent {
	t.Helper()
	var out []assistantEvent
	for _, line := range strings.Split(body, "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var e assistantEvent
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				t.Fatalf("event %q: %v", data, err)
			}
			if deltas || e.Type != "delta" {
				out = append(out, e)
			}
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

	// Ollama: its address kept bare (an OpenAI-style …/v1 too), and how much
	// the model reads.
	decode(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"ollama","url":"http://10.0.0.5:11434/v1/","model":"qwen3","context":32768}`, owner), &cfg)
	if cfg.URL != "http://10.0.0.5:11434" || cfg.Context != 32768 {
		t.Fatalf("ollama: %+v", cfg)
	}
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"ollama","url":"http://10.0.0.5:11434","model":"qwen3","context":100}`, owner), http.StatusBadRequest, "a tiny context")
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
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"openai","url":"`+srv.URL+`/v1","model":"llama3.1","daily_limit":3}`, owner), http.StatusOK, "set up")
	hol := createWS(t, r, owner, `{"name":"Holiday plans","kind":"personal"}`)
	createUserVia(t, r, owner, `{"username":"bea","email":"bea@example.org","password":"beapassword","role_ids":[]}`)
	bea := loginTok(t, r, "bea", "beapassword")

	w := authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"What workspaces do I have?","context":{"path":"/","title":"Today","now":"Thursday 5 November 2026, 10:00","time_zone":"Europe/Lisbon"}}`, bea)
	expect(t, w, http.StatusOK, "ask")
	// The answer comes in pieces as it's written, then whole.
	var pieces string
	for _, e := range sseAll(t, w.Body.String(), true) {
		if e.Type == "delta" {
			pieces += e.Text
		}
	}
	ev := sseEvents(t, w.Body.String())
	if len(ev) == 3 && (pieces != ev[1].Text || !strings.HasPrefix(pieces, "You have: ")) {
		t.Errorf("pieces %q, answer %q", pieces, ev[1].Text)
	}
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

	// The page on screen goes along — its id and what it says, read as the
	// one asking: Bea, who can't open the owner's page, gets neither content.
	text, _ := mcpToolCall(t, r, owner, "create_page", map[string]any{"workspace_id": hol.ID, "title": "Packing", "content": "Sunscreen and a hat"})
	var pg struct{ ID string }
	_ = json.Unmarshal([]byte(text), &pg)
	onPage := `{"message":"Sum up this page","context":{"path":"/w/` + hol.ID + `/p/` + pg.ID + `"}}`
	sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", onPage, owner).Body.String())
	sys = fake.asked[len(fake.asked)-2]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "the page") || !strings.Contains(sys, "page_id "+pg.ID) || !strings.Contains(sys, "Sunscreen and a hat") {
		t.Errorf("page on screen: %s", sys)
	}
	sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", onPage, bea).Body.String())
	sys = fake.asked[len(fake.asked)-2]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if strings.Contains(sys, "Sunscreen") {
		t.Errorf("Bea was shown the owner's page: %s", sys)
	}

	// Three a day: the fourth waits for tomorrow.
	expect(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"Once more"}`, bea), http.StatusTooManyRequests, "over the limit")
	var st assistantStatusResponse
	decode(t, authdo(r, "GET", "/api/v1/assistant", "", bea), &st)
	if st.UsedToday != 3 || st.DailyLimit != 3 {
		t.Errorf("status: %+v", st)
	}

	expect(t, authdo(r, "DELETE", "/api/v1/assistant/chats/"+chatID, "", bea), http.StatusNoContent, "delete")
	var list []assistantChatSummary
	decode(t, authdo(r, "GET", "/api/v1/assistant/chats", "", bea), &list)
	if len(list) != 1 || list[0].Title != "Sum up this page" {
		t.Errorf("left: %+v", list)
	}
}

// TestAssistantAgent: in Agent mode the changes the model calls for are only
// lined up; approved, the ticked ones are made as the person, in order (a
// task lands in the list made just before it), marked "via assistant"; the
// rest aren't; a decided list can't be approved again; the next message
// tells the model what became of them.
func TestAssistantAgent(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	fake := &fakeModel{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	old := assistantHTTP
	assistantHTTP = func() *http.Client { return srv.Client() }
	defer func() { assistantHTTP = old }()

	owner := loginTok(t, r, "admin", "adminpass")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"openai","url":"`+srv.URL+`/v1","model":"llama3.1"}`, owner), http.StatusOK, "set up")
	ws := createWS(t, r, owner, `{"name":"Work","kind":"personal"}`)
	lists := func() string {
		text, _ := mcpToolCall(t, r, owner, "list_task_lists", map[string]any{"workspace_id": ws.ID})
		return text
	}

	w := authdo(r, "POST", "/api/v1/assistant/chat", `{"mode":"agent","message":"plan:`+ws.ID+`"}`, owner)
	expect(t, w, http.StatusOK, "ask")
	ev := sseEvents(t, w.Body.String())
	if len(ev) != 5 || ev[3].Type != "answer" || len(ev[3].Actions) != 3 {
		t.Fatalf("events: %+v", ev)
	}
	acts := ev[3].Actions
	if acts[0].Title != "Create a task list" || acts[0].Summary != "Make the Launch list" || acts[0].State != "proposed" || acts[0].Args["summary"] != nil {
		t.Errorf("lined up: %+v", acts[0])
	}
	if strings.Contains(lists(), "Launch") {
		t.Fatal("made before it was approved")
	}
	first := fake.asked[0]
	sys := first["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "Agent mode") || !strings.Contains(sys, "- Work: "+ws.ID) {
		t.Errorf("system: %s", sys)
	}
	for _, x := range first["tools"].([]any) {
		fn := x.(map[string]any)["function"].(map[string]any)
		if fn["name"] == "create_task" && !strings.Contains(string(mustJSON(fn["parameters"])), `"summary"`) {
			t.Errorf("create_task without a summary: %v", fn["parameters"])
		}
	}

	chatID := ev[4].ChatID
	createUserVia(t, r, owner, `{"username":"cy","email":"cy@example.org","password":"cypassword","role_ids":[]}`)
	cy := loginTok(t, r, "cy", "cypassword")
	expect(t, authdo(r, "POST", "/api/v1/assistant/chats/"+chatID+"/apply", `{"turn":1,"approve":[1,2,3]}`, cy), http.StatusNotFound, "someone else's")

	var turn assistant.Turn
	decode(t, authdo(r, "POST", "/api/v1/assistant/chats/"+chatID+"/apply", `{"turn":1,"approve":[1,2]}`, owner), &turn)
	a := turn.Actions
	if a[0].State != "done" || a[0].MadeID == "" || a[1].State != "done" || a[2].State != "skipped" {
		t.Fatalf("outcome: %+v", a)
	}
	if !strings.Contains(lists(), "Launch") {
		t.Fatal("the list wasn't made")
	}
	tasks, _ := mcpToolCall(t, r, owner, "list_tasks", map[string]any{"workspace_id": ws.ID, "list_id": a[0].MadeID})
	if !strings.Contains(tasks, "Book the room") || strings.Contains(tasks, "Unwanted") {
		t.Errorf("tasks: %s", tasks)
	}
	expect(t, authdo(r, "POST", "/api/v1/assistant/chats/"+chatID+"/apply", `{"turn":1,"approve":[3]}`, owner), http.StatusConflict, "again")
	var n int64
	db.Model(&models.AuditLog{}).Where("action = ?", "assistant.apply").Count(&n)
	if n != 1 {
		t.Errorf("audit: %d", n)
	}

	// The next message carries what became of them.
	sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"chat_id":"`+chatID+`","message":"Thanks"}`, owner).Body.String())
	msgs := string(mustJSON(fake.asked[len(fake.asked)-2]["messages"]))
	// …told apart from its answers, which stay as they were (a model told
	// it in them copies it into its next answer).
	sent := fake.asked[len(fake.asked)-2]["messages"].([]any)
	if sys := sent[0].(map[string]any)["content"].(string); !strings.Contains(sys, "made (id "+a[0].MadeID+")") || !strings.Contains(sys, "not made (not approved)") {
		t.Errorf("outcomes not told: %s", sys)
	}
	if strings.Contains(sent[2].(map[string]any)["content"].(string), "made (id") {
		t.Errorf("outcomes in its answer: %s", msgs)
	}

	// Approving the task alone: it needs the list, which isn't made.
	ev = sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"mode":"agent","message":"plan:`+ws.ID+`"}`, owner).Body.String())
	decode(t, authdo(r, "POST", "/api/v1/assistant/chats/"+ev[4].ChatID+"/apply", `{"turn":1,"approve":[2]}`, owner), &turn)
	if turn.Actions[1].State != "skipped" || !strings.Contains(turn.Actions[1].Result, "needs change 1") {
		t.Errorf("dependent: %+v", turn.Actions[1])
	}

	// A workspace they don't have isn't lined up: the model is told at once,
	// with theirs.
	ev = sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"mode":"agent","message":"plan:nope"}`, owner).Body.String())
	if len(ev) != 5 || len(ev[3].Actions) != 0 || !strings.Contains(ev[3].Text, "no workspace with id nope") || !strings.Contains(ev[3].Text, ws.ID) {
		t.Errorf("unknown workspace: %+v", ev)
	}

	// Saying it lined a change up without calling the tool: told so once,
	// it calls it, and the change is there to approve.
	w = authdo(r, "POST", "/api/v1/assistant/chat", `{"mode":"agent","message":"claim:`+ws.ID+`"}`, owner)
	all := sseAll(t, w.Body.String(), true)
	restarted := false
	for _, e := range all {
		restarted = restarted || e.Type == "restart"
	}
	ev = sseEvents(t, w.Body.String())
	var got assistantEvent
	for _, e := range ev {
		if e.Type == "answer" {
			got = e
		}
	}
	if !restarted || len(got.Actions) != 1 || got.Actions[0].Summary != "Add Buy milk" {
		t.Errorf("claimed only: %+v", ev)
	}

	// Ask mode offers nothing that changes.
	sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"hi"}`, owner).Body.String())
	if strings.Contains(string(mustJSON(fake.asked[len(fake.asked)-1]["tools"])), "create_task") {
		t.Error("Ask mode offered create_task")
	}

	// A page saved through MCP says so in its history (the assistant's
	// changes go the same way, marked "assistant").
	text, _ := mcpToolCall(t, r, owner, "create_page", map[string]any{"workspace_id": ws.ID, "title": "Notes", "content": "First thoughts"})
	var pg struct{ ID string }
	_ = json.Unmarshal([]byte(text), &pg)
	var v models.PageVersion
	db.Where("page_id = ?", pg.ID).First(&v)
	if v.Via != "mcp" {
		t.Errorf("via: %q", v.Via)
	}
}

func TestClaimsChanges(t *testing.T) {
	for text, want := range map[string]bool{
		"I've lined up creating a task Buy milk":           true,
		"I created the page for you.":                      true,
		"The task has been queued.":                        true,
		"Error: not lined up: there's no workspace":        false,
		"I couldn't line it up: no such workspace.":        false,
		"You have three tasks due this week.":              false,
		"Lined up: nothing — tell me which list you mean.": false,
	} {
		if got := claimsChanges(text); got != want {
			t.Errorf("claimsChanges(%q) = %v", text, got)
		}
	}
}
