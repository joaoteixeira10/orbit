package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/assistant"
	"github.com/joaoteixeira10/orbit/api/internal/config"
	"github.com/joaoteixeira10/orbit/api/internal/models"
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
	// "files:<workspace id>:<file id>": put the file in Files, and make a
	// page of it.
	if rest, ok := strings.CutPrefix(fmt.Sprint(last["content"]), "files:"); ok && last["role"] == "user" {
		ws, id, _ := strings.Cut(rest, ":")
		calls := []string{
			`{"name":"save_attachment","arguments":` + jsonString(`{"file_id":"`+id+`","workspace_id":"`+ws+`","summary":"Put it in Files"}`) + `}`,
			`{"name":"page_from_attachment","arguments":` + jsonString(`{"file_id":"`+id+`","workspace_id":"`+ws+`","summary":"Make a page of it"}`) + `}`,
		}
		for i, fn := range calls {
			chunk(fmt.Sprintf(`{"tool_calls":[{"index":%d,"id":"f%d","type":"function","function":%s}]}`, i, i, fn), "")
		}
		chunk(`{}`, "tool_calls")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		return
	}
	// "misuse:<id>": moves a "page" by an id that isn't one (a task's).
	if id, ok := strings.CutPrefix(fmt.Sprint(last["content"]), "misuse:"); ok && last["role"] == "user" {
		chunk(`{"tool_calls":[{"index":0,"id":"m1","type":"function","function":{"name":"move_page","arguments":`+jsonString(`{"page_id":"`+id+`","summary":"Move Buy milk to Personal"}`)+`}}]}`, "tool_calls")
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
	if len(chat.Turns) != 4 || chat.Title != "What workspaces do I have?" || len(chat.Turns[1].Steps) != 1 || chat.Turns[0].At == "" || chat.Turns[1].At == "" {
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
	if acts[0].Title != "Create a task list" || acts[0].Summary != "Make the Launch list" || acts[0].State != "proposed" || acts[0].Args["summary"] != nil || acts[0].Where != "Work" {
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

	// A task's id given as a page's isn't lined up: the model is told what
	// it is. A real one shows what it's on, as Orbit has it.
	text, _ := mcpToolCall(t, r, owner, "create_task", map[string]any{"workspace_id": ws.ID, "title": "Buy milk"})
	var task struct{ ID string }
	_ = json.Unmarshal([]byte(text), &task)
	ev = sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"mode":"agent","message":"misuse:`+task.ID+`"}`, owner).Body.String())
	if len(ev) != 3 || len(ev[1].Actions) != 0 || !strings.Contains(ev[1].Text, `it's the task "Buy milk"`) {
		t.Errorf("a task as a page: %+v", ev)
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
	text, _ = mcpToolCall(t, r, owner, "create_page", map[string]any{"workspace_id": ws.ID, "title": "Notes", "content": "First thoughts"})
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

// TestAssistantWorkspaceSwitch: a workspace's owners can keep the assistant
// out of it (it can't see it at all, though apps through MCP still can), and
// give it instructions for theirs.
func TestAssistantWorkspaceSwitch(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	fake := &fakeModel{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	old := assistantHTTP
	assistantHTTP = func() *http.Client { return srv.Client() }
	defer func() { assistantHTTP = old }()

	owner := loginTok(t, r, "admin", "adminpass")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"openai","url":"`+srv.URL+`/v1","model":"llama3.1"}`, owner), http.StatusOK, "set up")
	school := createWS(t, r, owner, `{"name":"School","kind":"school"}`)
	secret := createWS(t, r, owner, `{"name":"Secret","kind":"personal"}`)
	var got workspaceResponse
	decode(t, authdo(r, "PUT", "/api/v1/workspaces/"+school.ID, `{"assistant_notes":"  Answer in Portuguese.  "}`, owner), &got)
	if got.AssistantNotes != "Answer in Portuguese." {
		t.Errorf("notes: %q", got.AssistantNotes)
	}
	decode(t, authdo(r, "PUT", "/api/v1/workspaces/"+secret.ID, `{"assistant_off":true}`, owner), &got)
	if !got.AssistantOff {
		t.Fatal("not turned off")
	}

	// Asked from the secret workspace: it lists theirs (not that one), is
	// told why, and gets the school's instructions.
	ev := sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"hi","context":{"workspace_id":"`+secret.ID+`","path":"/w/`+secret.ID+`/tasks"}}`, owner).Body.String())
	var answer string
	for _, e := range ev {
		if e.Type == "answer" {
			answer = e.Text
		}
	}
	if !strings.Contains(answer, "School") || strings.Contains(answer, "Secret") {
		t.Errorf("listed: %q", answer)
	}
	sent := string(mustJSON(fake.asked[0]))
	if !strings.Contains(sent, "Answer in Portuguese.") || !strings.Contains(sent, "assistant turned off") || strings.Contains(sent, secret.ID+"\\n") {
		t.Errorf("told: %s", sent)
	}
	// Nor can it line up changes there.
	ev = sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"mode":"agent","message":"plan:`+secret.ID+`"}`, owner).Body.String())
	for _, e := range ev {
		if e.Type == "answer" && (len(e.Actions) != 0 || !strings.Contains(e.Text, "no workspace with id "+secret.ID)) {
			t.Errorf("lined up in it: %+v", e)
		}
	}
	// An app through MCP still sees it.
	if text, _ := mcpToolCall(t, r, owner, "list_workspaces", map[string]any{}); !strings.Contains(text, "Secret") {
		t.Errorf("MCP: %s", text)
	}
}

// TestAssistantOwn: when the owner lets people use their own model, each may
// set up theirs (public and https, or a server in AI_SERVERS) and it answers
// them, with no daily limit; the key is never sent back.
func TestAssistantOwn(t *testing.T) {
	_, db := newTestRouterDB(t, false)
	fake := &fakeModel{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	r := routerOn(t, db, &config.Config{GinMode: "test", AuthRateBurst: 100, AuthRatePerMinute: 6000, AIServers: []string{srv.URL}}, nil)
	owner := loginTok(t, r, "admin", "adminpass")
	status := func() assistantStatusResponse {
		var st assistantStatusResponse
		decode(t, authdo(r, "GET", "/api/v1/assistant", "", owner), &st)
		return st
	}
	mine := func(body string) *httptest.ResponseRecorder {
		return authdo(r, "PUT", "/api/v1/me/assistant", body, owner)
	}

	if st := status(); st.Enabled || st.OwnAllowed {
		t.Fatalf("off: %+v", st)
	}
	expect(t, mine(`{"use":true,"provider":"openai","url":"`+srv.URL+`/v1","model":"mine"}`), http.StatusForbidden, "not allowed yet")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":false,"allow_own":true}`, owner), http.StatusOK, "allow own")
	if st := status(); st.Enabled || !st.OwnAllowed {
		t.Fatalf("allowed: %+v", st)
	}

	expect(t, mine(`{"use":true,"provider":"ollama","url":"https://10.1.2.3:11434"}`), http.StatusBadRequest, "private address")
	expect(t, mine(`{"use":true,"provider":"ollama","url":"http://example.com:11434"}`), http.StatusBadRequest, "plain http")
	expect(t, mine(`{"use":true,"provider":"anthropic"}`), http.StatusBadRequest, "Claude without a key")
	w := mine(`{"use":true,"provider":"openai","url":"` + srv.URL + `/v1","model":"mine","key":"sk-secret"}`)
	expect(t, w, http.StatusOK, "on an AI_SERVERS server")
	if strings.Contains(w.Body.String(), "sk-secret") || !strings.Contains(w.Body.String(), `"has_key":true`) {
		t.Errorf("own: %s", w.Body.String())
	}
	if st := status(); !st.Enabled || !st.Own || st.Model != "mine" || st.DailyLimit != 0 {
		t.Fatalf("mine: %+v", st)
	}

	// It answers, with no daily limit though the instance's has one.
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"allow_own":true,"provider":"openai","url":"https://api.example.com/v1","model":"theirs","daily_limit":1}`, owner), http.StatusOK, "instance on")
	for i := 0; i < 2; i++ {
		ev := sseEvents(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"hi"}`, owner).Body.String())
		if len(ev) == 0 || ev[len(ev)-1].Type != "done" {
			t.Fatalf("answer %d: %+v", i, ev)
		}
	}
	if m := fake.asked[len(fake.asked)-1]["model"]; m != "mine" {
		t.Errorf("asked %v", m)
	}
	if fake.asked[0]["model"] != "mine" {
		t.Error("not mine")
	}

	// Choosing the instance's, or the owner taking it back, leaves theirs.
	expect(t, mine(`{"use":false,"provider":"openai","url":"`+srv.URL+`/v1","model":"mine"}`), http.StatusOK, "the instance's")
	if st := status(); st.Own || st.Model != "theirs" || st.DailyLimit != 1 {
		t.Errorf("instance's: %+v", st)
	}
	expect(t, mine(`{"use":true,"provider":"openai","url":"`+srv.URL+`/v1","model":"mine"}`), http.StatusOK, "mine again")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":false,"allow_own":false}`, owner), http.StatusOK, "owner stops it")
	if st := status(); st.Enabled || st.OwnAllowed {
		t.Errorf("stopped: %+v", st)
	}
	expect(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"hi"}`, owner), http.StatusForbidden, "no model")
}

// TestAssistantAttachments: files attached to a message are the person's
// alone; the model gets what a short one says and reads the rest; in Agent
// mode it can put one in Files and make a page of one, once approved; they
// go with the conversation.
func TestAssistantAttachments(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	fake := &fakeModel{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	old := assistantHTTP
	assistantHTTP = func() *http.Client { return srv.Client() }
	defer func() { assistantHTTP = old }()

	owner := loginTok(t, r, "admin", "adminpass")
	attach := func(tok, name, body string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = io.WriteString(fw, body)
		_ = mw.Close()
		req := httptest.NewRequest("POST", "/api/v1/assistant/files", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	expect(t, attach(owner, "plan.md", "# Plan"), http.StatusForbidden, "assistant off")
	expect(t, authdo(r, "PUT", "/api/v1/assistant/config", `{"enabled":true,"provider":"openai","url":"`+srv.URL+`/v1","model":"llama3.1"}`, owner), http.StatusOK, "set up")
	ws := createWS(t, r, owner, `{"name":"Work","kind":"personal"}`)

	w := attach(owner, "plan.md", "# The plan\n\nStep one: book the room.\n\n- invites\n")
	expect(t, w, http.StatusCreated, "attach")
	var f attachmentResponse
	decode(t, w, &f)
	if !f.Readable || f.Name != "plan.md" {
		t.Fatalf("attached: %+v", f)
	}

	// Someone else can't see it, nor send it.
	createUserVia(t, r, owner, `{"username":"bea","email":"bea@example.org","password":"beapassword","role_ids":[]}`)
	bea := loginTok(t, r, "bea", "beapassword")
	expect(t, authdo(r, "GET", "/api/v1/uploads/"+f.ID+"/plan.md", "", bea), http.StatusNotFound, "someone else's")
	expect(t, authdo(r, "GET", "/api/v1/assistant/files/"+f.ID+"/text", "", bea), http.StatusNotFound, "someone else's text")
	expect(t, authdo(r, "POST", "/api/v1/assistant/chat", `{"message":"hi","files":["`+f.ID+`"]}`, bea), http.StatusBadRequest, "someone else's file")

	// Sent with a message: the model is told what it says.
	w = authdo(r, "POST", "/api/v1/assistant/chat", `{"mode":"agent","message":"files:`+ws.ID+`:`+f.ID+`","files":["`+f.ID+`"]}`, owner)
	expect(t, w, http.StatusOK, "ask")
	sent := string(mustJSON(fake.asked[len(fake.asked)-1]))
	if !strings.Contains(sent, "Step one: book the room.") || !strings.Contains(sent, "page_from_attachment") {
		t.Errorf("told: %s", sent)
	}
	var chatID string
	var acts []assistant.Action
	for _, e := range sseEvents(t, w.Body.String()) {
		if e.Type == "done" {
			chatID = e.ChatID
		}
		if e.Type == "answer" {
			acts = e.Actions
		}
	}
	if len(acts) != 2 || acts[0].Title != "Put a file in Files" {
		t.Fatalf("lined up: %+v", acts)
	}
	var chat assistantChatResponse
	decode(t, authdo(r, "GET", "/api/v1/assistant/chats/"+chatID, "", owner), &chat)
	if len(chat.Turns[0].Files) != 1 || chat.Turns[0].Files[0].ID != f.ID {
		t.Fatalf("kept: %+v", chat.Turns[0])
	}

	// Approved: the file in Files, and a page of it.
	var turn assistant.Turn
	decode(t, authdo(r, "POST", "/api/v1/assistant/chats/"+chatID+"/apply", `{"turn":1,"approve":[1,2]}`, owner), &turn)
	if turn.Actions[0].State != "done" || turn.Actions[1].State != "done" {
		t.Fatalf("made: %+v", turn.Actions)
	}
	var files []fileResponse
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/files", "", owner), &files)
	if len(files) != 1 || files[0].Name != "plan.md" {
		t.Errorf("in Files: %+v", files)
	}
	var page struct{ Title, Content string }
	decode(t, authdo(r, "GET", "/api/v1/pages/"+turn.Actions[1].MadeID, "", owner), &page)
	if page.Title != "The plan" || !strings.Contains(page.Content, "Step one: book the room.") || strings.Contains(page.Content, "# The plan") { // its own title, as Import takes it
		t.Errorf("page: %+v", page)
	}

	// A draw.io or Excalidraw file goes in as a diagram or whiteboard.
	for _, file := range []struct{ name, body, list string }{
		{"network.drawio", `<mxfile><diagram name="p">x</diagram></mxfile>`, "/diagrams"},
		{"sketch.excalidraw", `{"type":"excalidraw","version":2,"elements":[]}`, "/drawings"},
	} {
		w := attach(owner, file.name, file.body)
		expect(t, w, http.StatusCreated, "attach "+file.name)
		var g attachmentResponse
		decode(t, w, &g)
		w = authdo(r, "POST", "/api/v1/assistant/chat", `{"chat_id":"`+chatID+`","mode":"agent","message":"files:`+ws.ID+`:`+g.ID+`","files":["`+g.ID+`"]}`, owner)
		expect(t, w, http.StatusOK, "ask "+file.name)
		var chat assistantChatResponse
		decode(t, authdo(r, "GET", "/api/v1/assistant/chats/"+chatID, "", owner), &chat)
		at := len(chat.Turns) - 1
		var turn assistant.Turn
		decode(t, authdo(r, "POST", "/api/v1/assistant/chats/"+chatID+"/apply", fmt.Sprintf(`{"turn":%d,"approve":[1]}`, at), owner), &turn)
		if turn.Actions[0].State != "done" {
			t.Fatalf("%s: %+v", file.name, turn.Actions[0])
		}
		var made []struct{ ID, Title string }
		decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+file.list, "", owner), &made)
		if len(made) != 1 || made[0].Title != strings.TrimSuffix(file.name, path.Ext(file.name)) || made[0].ID != turn.Actions[0].MadeID {
			t.Errorf("%s made: %+v (%s)", file.name, made, turn.Actions[0].MadeID)
		}
	}

	// Not among their own files; gone with the conversation (the copy in
	// Files stays).
	var mineFiles []fileResponse
	decode(t, authdo(r, "GET", "/api/v1/me/files", "", owner), &mineFiles)
	if len(mineFiles) != 0 {
		t.Errorf("my files: %+v", mineFiles)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/assistant/chats/"+chatID, "", owner), http.StatusNoContent, "delete")
	var n int64
	db.Model(&models.Upload{}).Where("id = ?", f.ID).Count(&n)
	if n != 0 {
		t.Error("attachment kept")
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/files", "", owner), &files)
	if len(files) != 1 {
		t.Error("the copy in Files went too")
	}
}

func TestPdfParagraphs(t *testing.T) {
	in := "Introduction\nThis is a long line of text that goes on and on across the page width\nand continues here with a hyphen-\nated word at the end.\nShort one.\n\nNext page text."
	want := "Introduction\n\nThis is a long line of text that goes on and on across the page width and continues here with a hyphenated word at the end.\n\nShort one.\n\nNext page text."
	if got := pdfParagraphs(in); got != want {
		t.Errorf("got %q", got)
	}
}
