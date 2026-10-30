package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/version"

	"github.com/gin-gonic/gin"
)

// MCP: Orbit as a Model Context Protocol server (Streamable HTTP, JSON
// answers, no server-sent stream), so an assistant — Claude, ChatGPT,
// Cursor — reads and writes a person's pages, tasks, calendar and the rest.
// It signs in as that person: an API token, or one handed out by Orbit's
// OAuth sign-in (oauth.go). Every tool is a call to the REST API with the
// caller's own token, so what it may see and do is exactly theirs (a
// read-only token reads only).

// The protocol versions spoken (newest first).
var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const mcpInstructions = `Orbit is a planner: workspaces hold pages (Markdown documents in a tree, which may be filed in folders), task lists with tasks (due dates, priorities, assignees, sub-tasks), calendar events, quick notes, sheets, diagrams (draw.io or Mermaid), whiteboards and folders.
Start with list_workspaces; most tools take a workspace_id. Use search to find things by their words. Pages are read and written as Markdown; a link to another Orbit thing is written [Title](/w/{workspace_id}/p/{page_id}) (s/ for a sheet, d/ for a diagram, whiteboards/ for a whiteboard), and such a link alone on its line shows that thing inside the page. Dates are YYYY-MM-DD; times HH:MM; instants RFC 3339.`

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

// mcpChallenge answers a call without a bearer token with 401 and where to
// sign in (RFC 9728), as MCP clients expect.
func (h *Handlers) mcpChallenge() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") {
			c.Header("WWW-Authenticate", `Bearer resource_metadata="`+h.baseURL(c)+`/.well-known/oauth-protected-resource"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "sign in: an Orbit API token, or Orbit's OAuth"})
			return
		}
		c.Next()
	}
}

// mcpNoStream: no server-to-client stream (GET) and no sessions to end (DELETE).
func mcpNoStream(c *gin.Context) {
	c.Header("Allow", "POST")
	c.Status(http.StatusMethodNotAllowed)
}

// mcpHandler answers MCP's JSON-RPC messages (one, or a batch). api is the
// router itself, which the tools call.
func (h *Handlers) mcpHandler(api http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var raw json.RawMessage
		if err := json.NewDecoder(c.Request.Body).Decode(&raw); err != nil {
			c.JSON(http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpError{Code: -32700, Message: "parse error"}})
			return
		}
		call := &mcpCall{c: c, api: api}
		if t := bytes.TrimSpace(raw); len(t) > 0 && t[0] == '[' {
			var reqs []mcpRequest
			if json.Unmarshal(raw, &reqs) != nil {
				c.JSON(http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpError{Code: -32700, Message: "parse error"}})
				return
			}
			var out []mcpResponse
			for _, r := range reqs {
				if res, ok := h.mcpOne(call, r); ok {
					out = append(out, res)
				}
			}
			if len(out) == 0 {
				c.Status(http.StatusAccepted)
				return
			}
			c.JSON(http.StatusOK, out)
			return
		}
		var req mcpRequest
		if json.Unmarshal(raw, &req) != nil {
			c.JSON(http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpError{Code: -32600, Message: "invalid request"}})
			return
		}
		res, ok := h.mcpOne(call, req)
		if !ok {
			c.Status(http.StatusAccepted) // a notification, or an answer to us
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

// mcpOne answers one message (ok false: nothing to answer).
func (h *Handlers) mcpOne(call *mcpCall, req mcpRequest) (mcpResponse, bool) {
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return mcpResponse{}, false
	}
	res := mcpResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := mcpVersions[0]
		for _, x := range mcpVersions {
			if x == p.ProtocolVersion {
				v = x
			}
		}
		res.Result = gin.H{
			"protocolVersion": v,
			"capabilities":    gin.H{"tools": gin.H{"listChanged": false}},
			"serverInfo":      gin.H{"name": "orbit", "title": "Orbit", "version": version.Version},
			"instructions":    mcpInstructions,
		}
	case "ping":
		res.Result = gin.H{}
	case "tools/list":
		tools := make([]gin.H, 0, len(mcpTools))
		for _, t := range mcpTools {
			tools = append(tools, gin.H{
				"name": t.Name, "title": t.Title, "description": t.Description, "inputSchema": t.schema(),
				"annotations": gin.H{"title": t.Title, "readOnlyHint": t.ReadOnly, "destructiveHint": t.Destructive, "openWorldHint": false},
			})
		}
		res.Result = gin.H{"tools": tools}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &p) != nil {
			res.Error = &mcpError{Code: -32602, Message: "invalid params"}
			break
		}
		t := mcpToolByName(p.Name)
		if t == nil {
			res.Error = &mcpError{Code: -32602, Message: "unknown tool: " + p.Name}
			break
		}
		if p.Arguments == nil {
			p.Arguments = map[string]any{}
		}
		for _, k := range t.Required {
			if v, ok := p.Arguments[k]; !ok || v == nil || v == "" {
				res.Result = mcpText("missing argument: "+k, true)
				return res, true
			}
		}
		out, err := t.Run(call, mcpArgs(p.Arguments))
		if err != nil {
			res.Result = mcpText(err.Error(), true)
			break
		}
		if s, ok := out.(string); ok {
			res.Result = mcpText(s, false)
			break
		}
		// As written: < > & stay themselves (Markdown, Mermaid's arrows).
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", " ")
		_ = enc.Encode(out)
		r := mcpText(strings.TrimSpace(buf.String()), false)
		if m, ok := out.(map[string]any); ok {
			r["structuredContent"] = m
		}
		res.Result = r
	default:
		res.Error = &mcpError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return res, true
}

func mcpText(s string, isErr bool) map[string]any {
	return map[string]any{"content": []gin.H{{"type": "text", "text": s}}, "isError": isErr}
}

// ---- Calling the API as the caller ----

type mcpCall struct {
	c   *gin.Context
	api http.Handler
}

// bufWriter is a ResponseWriter that keeps the answer.
type bufWriter struct {
	head http.Header
	code int
	body bytes.Buffer
}

func (w *bufWriter) Header() http.Header         { return w.head }
func (w *bufWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *bufWriter) WriteHeader(code int)        { w.code = code }

// do calls the API (path under /api/v1) with the caller's sign-in; an answer
// of 400 or more is an error saying what the API said.
func (m *mcpCall) do(method, path string, q url.Values, body any, into any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	u := "/api/v1" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(m.c.Request.Context(), method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", m.c.GetHeader("Authorization"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Orbit-Client", "mcp")
	req.RemoteAddr = m.c.Request.RemoteAddr
	w := &bufWriter{head: http.Header{}, code: http.StatusOK}
	m.api.ServeHTTP(w, req)
	if w.code >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(w.body.Bytes(), &e)
		if e.Error == "" {
			e.Error = http.StatusText(w.code)
		}
		return fmt.Errorf("%s (HTTP %d)", e.Error, w.code)
	}
	if into != nil && w.body.Len() > 0 {
		return json.Unmarshal(w.body.Bytes(), into)
	}
	return nil
}

// ---- Arguments ----

type mcpArgs map[string]any

func (a mcpArgs) str(k string) string {
	switch v := a[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprint(v)
	}
	return ""
}

func (a mcpArgs) has(k string) bool {
	_, ok := a[k]
	return ok
}

// pick copies the named arguments that were given (as they came).
func (a mcpArgs) pick(keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := a[k]; ok {
			out[k] = v
		}
	}
	return out
}

// keep reduces a list of objects to some of their fields.
func keep(list []map[string]any, keys ...string) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, x := range list {
		y := map[string]any{}
		for _, k := range keys {
			if v, ok := x[k]; ok && v != nil && v != "" {
				y[k] = v
			}
		}
		out = append(out, y)
	}
	return out
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
