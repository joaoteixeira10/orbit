package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// mcpRPC sends one JSON-RPC message to /mcp.
func mcpRPC(t *testing.T, r *gin.Engine, token, method string, params any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	w := authdo(r, "POST", "/mcp", string(b), token)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// mcpTool calls a tool: its text, and whether it said it failed.
func mcpToolCall(t *testing.T, r *gin.Engine, token, name string, args map[string]any) (string, bool) {
	t.Helper()
	code, out := mcpRPC(t, r, token, "tools/call", map[string]any{"name": name, "arguments": args})
	if code != http.StatusOK || out["result"] == nil {
		t.Fatalf("%s: %d %v", name, code, out)
	}
	res := out["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	return text, res["isError"] == true
}

// TestMCP: the MCP endpoint wants a bearer token (and says where to get
// one); it speaks the protocol (initialize, tools, notifications); its tools
// read and write as the caller — a read-only token only reads.
func TestMCP(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Agency","kind":"work","starter":false,"modules":["home","tasks","pages","calendar","notes","diagrams"]}`)

	w := do(r, "POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Header().Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource") {
		t.Fatalf("no token: %d %q", w.Code, w.Header().Get("WWW-Authenticate"))
	}
	expect(t, authdo(r, "GET", "/mcp", "", alice), http.StatusMethodNotAllowed, "no stream")

	code, out := mcpRPC(t, r, alice, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}})
	res, _ := out["result"].(map[string]any)
	if code != 200 || res["protocolVersion"] != "2025-06-18" || res["serverInfo"].(map[string]any)["name"] != "orbit" {
		t.Fatalf("initialize: %d %v", code, out)
	}
	expect(t, authdo(r, "POST", "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, alice), http.StatusAccepted, "notification")
	_, out = mcpRPC(t, r, alice, "tools/list", nil)
	tools := out["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 25 {
		t.Fatalf("tools: %d", len(tools))
	}
	_, out = mcpRPC(t, r, alice, "nope", nil)
	if out["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Fatalf("unknown method: %v", out)
	}

	text, bad := mcpToolCall(t, r, alice, "list_workspaces", nil)
	if bad || !strings.Contains(text, ws.ID) {
		t.Fatalf("workspaces: %s", text)
	}
	text, bad = mcpToolCall(t, r, alice, "create_page", map[string]any{"workspace_id": ws.ID, "title": "Kickoff", "content": "Agenda"})
	var pg struct{ ID string }
	_ = json.Unmarshal([]byte(text), &pg)
	if bad || pg.ID == "" {
		t.Fatalf("create page: %s", text)
	}
	if _, bad = mcpToolCall(t, r, alice, "update_page", map[string]any{"page_id": pg.ID, "append": "- coffee"}); bad {
		t.Fatal("append")
	}
	text, _ = mcpToolCall(t, r, alice, "read_page", map[string]any{"page_id": pg.ID})
	if text != "# Kickoff\n\nAgenda\n\n- coffee" {
		t.Fatalf("read page: %q", text)
	}
	// A model writing a page as Orbit exports one: its YAML header becomes
	// tags and properties, and the title isn't said twice.
	text, bad = mcpToolCall(t, r, alice, "create_page", map[string]any{"workspace_id": ws.ID, "title": "Site Fix", "tags": []any{"Ops"},
		"content": "---\ntags: [\"Procedure\"]\nowner: Ana\n---\n# Site Fix\n\n```sh\nscp a b\n```"})
	_ = json.Unmarshal([]byte(text), &pg)
	if bad {
		t.Fatalf("create page with a header: %s", text)
	}
	var got struct {
		Content string
		Tags    []string
		Fields  []map[string]any
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/pages/"+pg.ID, "", alice).Body.Bytes(), &got)
	if got.Content != "```sh\nscp a b\n```" || strings.Join(got.Tags, ",") != "Ops,Procedure" || len(got.Fields) != 1 || got.Fields[0]["value"] != "Ana" {
		t.Fatalf("header: %+v", got)
	}
	if _, bad = mcpToolCall(t, r, alice, "update_page", map[string]any{"page_id": pg.ID, "content": "---\ntags:\n  - Runbook\n---\nDone."}); bad {
		t.Fatal("update with a header")
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/pages/"+pg.ID, "", alice).Body.Bytes(), &got)
	if got.Content != "Done." || strings.Join(got.Tags, ",") != "Runbook" || len(got.Fields) != 1 {
		t.Fatalf("update header: %+v", got)
	}
	if text, bad = mcpToolCall(t, r, alice, "create_task_list", map[string]any{"workspace_id": ws.ID, "name": "Launch"}); bad {
		t.Fatalf("list: %s", text)
	}
	text, bad = mcpToolCall(t, r, alice, "create_task", map[string]any{"workspace_id": ws.ID, "title": "Book the room", "due": "2026-11-02", "priority": 3})
	var task struct{ ID string }
	_ = json.Unmarshal([]byte(text), &task)
	if bad || task.ID == "" {
		t.Fatalf("create task: %s", text)
	}
	if text, _ = mcpToolCall(t, r, alice, "list_tasks", map[string]any{"workspace_id": ws.ID}); !strings.Contains(text, "Book the room") {
		t.Fatalf("tasks: %s", text)
	}
	if text, bad = mcpToolCall(t, r, alice, "complete_task", map[string]any{"task_id": task.ID}); bad || !strings.Contains(text, `"done": true`) {
		t.Fatalf("complete: %s", text)
	}
	if text, bad = mcpToolCall(t, r, alice, "create_event", map[string]any{"workspace_id": ws.ID, "title": "Kickoff", "starts_at": "2026-11-02T09:00:00Z", "ends_at": "2026-11-02T10:00:00Z"}); bad {
		t.Fatalf("event: %s", text)
	}
	if text, _ = mcpToolCall(t, r, alice, "list_events", map[string]any{"from": "2026-11-01T00:00:00Z", "to": "2026-11-08T00:00:00Z", "workspace_id": ws.ID}); !strings.Contains(text, "Kickoff") {
		t.Fatalf("events: %s", text)
	}
	text, bad = mcpToolCall(t, r, alice, "create_mermaid_diagram", map[string]any{"workspace_id": ws.ID, "title": "Flow", "mermaid": "flowchart TD\n  A --> B"})
	var dg struct{ ID string }
	_ = json.Unmarshal([]byte(text), &dg)
	if bad || dg.ID == "" {
		t.Fatalf("diagram: %s", text)
	}
	if text, _ = mcpToolCall(t, r, alice, "read_diagram", map[string]any{"diagram_id": dg.ID}); !strings.Contains(text, "A --> B") {
		t.Fatalf("read diagram: %s", text)
	}
	// Someone else's workspace, a missing argument: errors, as tool results.
	if text, bad = mcpToolCall(t, r, bob, "list_pages", map[string]any{"workspace_id": ws.ID}); !bad {
		t.Fatalf("not a member: %s", text)
	}
	if _, bad = mcpToolCall(t, r, alice, "read_page", map[string]any{}); !bad {
		t.Fatal("missing argument")
	}

	// A read-only token reads, and doesn't write.
	var tok struct{ Token string }
	decode(t, authdo(r, "POST", "/api/v1/me/api-tokens", `{"name":"assistant","read_only":true}`, alice), &tok)
	if text, bad = mcpToolCall(t, r, tok.Token, "read_page", map[string]any{"page_id": pg.ID}); bad {
		t.Fatalf("read-only reads: %s", text)
	}
	if _, bad = mcpToolCall(t, r, tok.Token, "create_page", map[string]any{"workspace_id": ws.ID, "title": "X"}); !bad {
		t.Fatal("read-only wrote")
	}
}

// TestOAuth: Orbit describes its OAuth; an app registers (not with a
// dangerous address), the person allows it (or not), and the app trades
// the code — with its PKCE verifier, once — for a token that works on /mcp;
// "read only" gives a read-only one.
func TestOAuth(t *testing.T) {
	r, _, _, alice, _ := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Agency","kind":"work","starter":false,"modules":["home","pages"]}`)

	var meta map[string]any
	decode(t, do(r, "GET", "/.well-known/oauth-authorization-server", ""), &meta)
	if !strings.HasSuffix(meta["token_endpoint"].(string), "/api/v1/oauth/token") || !strings.HasSuffix(meta["authorization_endpoint"].(string), "/oauth/authorize") {
		t.Fatalf("metadata: %v", meta)
	}
	decode(t, do(r, "GET", "/.well-known/oauth-protected-resource", ""), &meta)
	if !strings.HasSuffix(meta["resource"].(string), "/mcp") {
		t.Fatalf("resource: %v", meta)
	}

	expect(t, do(r, "POST", "/api/v1/oauth/register", `{"client_name":"Evil","redirect_uris":["javascript:alert(1)"]}`), http.StatusBadRequest, "bad redirect")
	expect(t, do(r, "POST", "/api/v1/oauth/register", `{"client_name":"Plain","redirect_uris":["http://example.org/cb"]}`), http.StatusBadRequest, "plain http elsewhere")
	var cl struct {
		ClientID string `json:"client_id"`
	}
	w := do(r, "POST", "/api/v1/oauth/register", `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback","http://localhost:6274/cb"]}`)
	expect(t, w, http.StatusCreated, "register")
	decode(t, w, &cl)

	var info map[string]any
	decode(t, authdo(r, "GET", "/api/v1/oauth/clients/"+cl.ClientID+"?redirect_uri="+url.QueryEscape("https://claude.ai/api/mcp/auth_callback"), "", alice), &info)
	if info["client_name"] != "Claude" || info["redirect_host"] != "claude.ai" {
		t.Fatalf("client info: %v", info)
	}
	expect(t, authdo(r, "GET", "/api/v1/oauth/clients/"+cl.ClientID+"?redirect_uri="+url.QueryEscape("https://evil.example/cb"), "", alice), http.StatusBadRequest, "unregistered redirect")

	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	allow := func(readOnly, deny bool) url.Values {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"client_id": cl.ClientID, "redirect_uri": "https://claude.ai/api/mcp/auth_callback", "state": "xyz", "code_challenge": challenge, "code_challenge_method": "S256", "read_only": readOnly, "deny": deny})
		var out struct{ Redirect string }
		decode(t, authdo(r, "POST", "/api/v1/oauth/authorize", string(b), alice), &out)
		u, err := url.Parse(out.Redirect)
		if err != nil || u.Host != "claude.ai" {
			t.Fatalf("redirect: %q", out.Redirect)
		}
		return u.Query()
	}
	if q := allow(false, true); q.Get("error") != "access_denied" || q.Get("state") != "xyz" {
		t.Fatalf("deny: %v", q)
	}
	token := func(code, verifier string) *httptest.ResponseRecorder {
		form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"https://claude.ai/api/mcp/auth_callback"}, "client_id": {cl.ClientID}, "code_verifier": {verifier}}
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)
		return w
	}
	q := allow(false, false)
	expect(t, token(q.Get("code"), "wrong-"+verifier), http.StatusBadRequest, "wrong verifier")
	q = allow(false, false)
	w = token(q.Get("code"), verifier)
	expect(t, w, http.StatusOK, "token")
	var tok struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
	}
	decode(t, w, &tok)
	expect(t, token(q.Get("code"), verifier), http.StatusBadRequest, "code used twice")
	if text, bad := mcpToolCall(t, r, tok.AccessToken, "create_page", map[string]any{"workspace_id": ws.ID, "title": "From Claude"}); bad {
		t.Fatalf("oauth token writes: %s", text)
	}
	// Read only.
	q = allow(true, false)
	w = token(q.Get("code"), verifier)
	decode(t, w, &tok)
	if tok.Scope != "orbit:read" {
		t.Fatalf("scope: %v", tok)
	}
	if _, bad := mcpToolCall(t, r, tok.AccessToken, "create_page", map[string]any{"workspace_id": ws.ID, "title": "X"}); !bad {
		t.Fatal("read-only oauth token wrote")
	}
	// The tokens are the person's API tokens, named after the app.
	var list []struct{ Name string }
	decode(t, authdo(r, "GET", "/api/v1/me/api-tokens", "", alice), &list)
	named := 0
	for _, x := range list {
		if x.Name == "OAuth: Claude" {
			named++
		}
	}
	if named != 2 {
		t.Fatalf("tokens: %+v", list)
	}
}

// TestMCPLife: habits, goals, the journal, bookmarks, focus and notes through
// MCP — each person's own habits and journal stay their own.
func TestMCPLife(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Home","kind":"personal","starter":false,"modules":["home","tasks","notes","habits","goals","journal","bookmarks","focus"]}`)
	call := func(tok, name string, args map[string]any) map[string]any {
		t.Helper()
		text, bad := mcpToolCall(t, r, tok, name, args)
		if bad {
			t.Fatalf("%s: %s", name, text)
		}
		var out map[string]any
		_ = json.Unmarshal([]byte(text), &out)
		return out
	}
	today := time.Now().Format("2006-01-02")

	// Habits: made, ticked, listed with the day done; untick; someone else can't.
	hb := call(alice, "create_habit", map[string]any{"workspace_id": ws.ID, "title": "NTE Farm", "icon": "🌱"})
	hid := hb["id"].(string)
	got := call(alice, "check_habit", map[string]any{"habit_id": hid, "date": today})
	if d := got["done_on"].([]any); len(d) != 1 || d[0] != today {
		t.Fatalf("ticked: %v", got)
	}
	list := call(alice, "list_habits", map[string]any{"workspace_id": ws.ID})["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["title"] != "NTE Farm" {
		t.Fatalf("habits: %v", list)
	}
	got = call(alice, "check_habit", map[string]any{"habit_id": hid, "date": today, "done": false})
	if len(got["done_on"].([]any)) != 0 {
		t.Fatalf("unticked: %v", got)
	}
	if _, bad := mcpToolCall(t, r, bob, "check_habit", map[string]any{"habit_id": hid, "date": today}); !bad {
		t.Fatal("bob ticked alice's habit")
	}
	call(alice, "update_habit", map[string]any{"habit_id": hid, "archived": true})

	// Goals, with a task counting towards one.
	g := call(alice, "create_goal", map[string]any{"workspace_id": ws.ID, "title": "Run 10k", "due": "2027-06-01"})
	call(alice, "create_task_list", map[string]any{"workspace_id": ws.ID, "name": "Errands"})
	call(alice, "create_task", map[string]any{"workspace_id": ws.ID, "title": "Buy shoes", "goal_id": g["id"]})
	goals := call(alice, "list_goals", map[string]any{"workspace_id": ws.ID})["items"].([]any)
	if gl := goals[0].(map[string]any); gl["title"] != "Run 10k" || gl["tasks"].(float64) != 1 {
		t.Fatalf("goals: %v", goals)
	}
	call(alice, "update_goal", map[string]any{"goal_id": g["id"], "achieved": true})

	// The journal: written, added to, read back; the month lists the day.
	call(alice, "write_journal", map[string]any{"workspace_id": ws.ID, "date": "2026-11-09", "content": "Planted the beans.", "mood": 4})
	call(alice, "write_journal", map[string]any{"workspace_id": ws.ID, "date": "2026-11-09", "append": "Rain later."})
	day := call(alice, "read_journal", map[string]any{"workspace_id": ws.ID, "date": "2026-11-09"})
	if day["content"] != "Planted the beans.\n\nRain later." || day["mood"].(float64) != 4 {
		t.Fatalf("journal: %v", day)
	}
	month := call(alice, "read_journal", map[string]any{"workspace_id": ws.ID, "month": "2026-11"})["items"].([]any)
	if len(month) != 1 {
		t.Fatalf("month: %v", month)
	}

	// Bookmarks.
	b := call(alice, "create_bookmark", map[string]any{"workspace_id": ws.ID, "url": "https://example.org/seeds", "title": "Seeds", "tags": []string{"farm"}, "read_later": true})
	call(alice, "update_bookmark", map[string]any{"bookmark_id": b["id"], "read": true})
	bs := call(alice, "list_bookmarks", map[string]any{"workspace_id": ws.ID})["items"].([]any)
	if bm := bs[0].(map[string]any); bm["title"] != "Seeds" || bm["read_at"] == nil {
		t.Fatalf("bookmarks: %v", bs)
	}
	call(alice, "delete_bookmark", map[string]any{"bookmark_id": b["id"]})

	// Focus: started, seen running, stopped.
	call(alice, "start_focus", map[string]any{"workspace_id": ws.ID, "minutes": 25})
	if f := call(alice, "get_focus", nil); f["running"] == nil {
		t.Fatalf("focus: %v", f)
	}
	call(alice, "stop_focus", nil)
	if f := call(alice, "get_focus", nil); f["running"] != nil {
		t.Fatalf("still running: %v", f)
	}

	// Notes: changed and deleted.
	n := call(alice, "create_note", map[string]any{"workspace_id": ws.ID, "body": "milk"})
	call(alice, "update_note", map[string]any{"note_id": n["id"], "body": "milk, eggs", "pinned": true})
	ns := call(alice, "list_notes", map[string]any{"workspace_id": ws.ID})["items"].([]any)
	if nn := ns[0].(map[string]any); nn["body"] != "milk, eggs" || nn["pinned"] != true {
		t.Fatalf("notes: %v", ns)
	}
	call(alice, "delete_note", map[string]any{"note_id": n["id"]})
	if ns := call(alice, "list_notes", map[string]any{"workspace_id": ws.ID})["items"].([]any); len(ns) != 0 {
		t.Fatalf("note not deleted: %v", ns)
	}
}
