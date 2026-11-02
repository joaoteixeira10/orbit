package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/search"
)

type diagramResp struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Version int    `json:"version"`
	Data    string `json:"data"`
	Preview string `json:"preview"`
}

const flow = `<mxfile><diagram id="a" name="Page-1"><mxGraphModel><root><mxCell id="0"/><mxCell id="2" value="Order placed" vertex="1"/></root></mxGraphModel></diagram></mxfile>`

// TestDiagrams: a diagram is made, saved by version (a stale save is
// refused), checked, found by its labels, listed with its picture, carried
// by export / import, deleted.
func TestDiagrams(t *testing.T) {
	r, db, admin, alice, bob := plannerSetupDB(t)
	if !search.Setup(db) {
		t.Skip("no FTS5")
	}
	t.Cleanup(func() { search.Setup(nil) })
	ws := createWS(t, r, alice, `{"name":"Shop","kind":"personal","starter":false,"modules":["home","diagrams"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	body := func(m map[string]any) string { b, _ := json.Marshal(m); return string(b) }
	expect(t, authdo(r, "POST", base+"/diagrams", `{"title":"Flow"}`, bob), http.StatusNotFound, "not a member")
	expect(t, authdo(r, "POST", base+"/diagrams", `{"title":"Flow","data":"not xml"}`, alice), http.StatusBadRequest, "not XML")
	expect(t, authdo(r, "POST", base+"/diagrams", `{"title":"Flow","preview":"<html>"}`, alice), http.StatusBadRequest, "not SVG")
	var d diagramResp
	w := authdo(r, "POST", base+"/diagrams", body(map[string]any{"title": "Flow", "data": flow, "preview": "<svg/>", "text": "Order placed"}), alice)
	expect(t, w, http.StatusCreated, "create")
	decode(t, w, &d)
	var saved diagramResp
	decode(t, authdo(r, "PUT", "/api/v1/diagrams/"+d.ID, `{"title":"Order flow","version":1,"text":"Order placed Shipped"}`, alice), &saved)
	if saved.Version != 2 || saved.Title != "Order flow" || saved.Data != flow {
		t.Fatalf("saved: %+v", saved)
	}
	expect(t, authdo(r, "PUT", "/api/v1/diagrams/"+d.ID, `{"title":"stale","version":1}`, alice), http.StatusConflict, "stale save")

	var hits []struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	decode(t, authdo(r, "GET", "/api/v1/search?q=shipped", "", alice), &hits)
	if len(hits) != 1 || hits[0].Kind != "diagram" || hits[0].ID != d.ID {
		t.Errorf("search: %+v", hits)
	}
	var list []diagramResp
	decode(t, authdo(r, "GET", base+"/diagrams", "", alice), &list)
	if len(list) != 1 || list[0].Preview != "<svg/>" || list[0].Data != "" {
		t.Errorf("list: %+v", list)
	}

	w = authdo(r, "GET", base+"/transfer", "", alice)
	expect(t, w, http.StatusOK, "export")
	w = importWS(r, admin, w.Body.Bytes(), false)
	expect(t, w, http.StatusCreated, "import")
	var got importResp
	decode(t, w, &got)
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID+"/diagrams", "", admin), &list)
	if len(list) != 1 || list[0].Title != "Order flow" || list[0].ID == d.ID {
		t.Fatalf("imported: %+v", list)
	}
	var full diagramResp
	decode(t, authdo(r, "GET", "/api/v1/diagrams/"+list[0].ID, "", admin), &full)
	if full.Data != flow {
		t.Errorf("imported data: %q", full.Data)
	}

	// A Mermaid diagram: its file is text.
	expect(t, authdo(r, "POST", base+"/diagrams", `{"title":"M","kind":"plantuml"}`, alice), http.StatusBadRequest, "unknown kind")
	var m diagramResp
	w = authdo(r, "POST", base+"/diagrams", body(map[string]any{"title": "Steps", "kind": "mermaid", "data": "flowchart TD\n  A[Pack] --> B[Ship]"}), alice)
	expect(t, w, http.StatusCreated, "create mermaid")
	decode(t, w, &m)
	if m.Kind != "mermaid" || !strings.HasPrefix(m.Data, "flowchart") {
		t.Fatalf("mermaid: %+v", m)
	}
	expect(t, authdo(r, "PUT", "/api/v1/diagrams/"+m.ID, `{"data":"sequenceDiagram\n  A->>B: hi","version":1}`, alice), http.StatusOK, "save mermaid")
	decode(t, authdo(r, "GET", base+"/diagrams", "", alice), &list)
	kinds := map[string]string{}
	for _, x := range list {
		kinds[x.Title] = x.Kind
	}
	if kinds["Steps"] != "mermaid" || kinds["Order flow"] != "drawio" {
		t.Errorf("kinds: %v", kinds)
	}

	expect(t, authdo(r, "DELETE", "/api/v1/diagrams/"+d.ID, "", alice), http.StatusNoContent, "delete")
	expect(t, authdo(r, "GET", "/api/v1/diagrams/"+d.ID, "", alice), http.StatusNotFound, "gone")
}

// TestDrawioAddress: draw.io's own by default; the owner sets their own
// (checked), which the branding carries and the CSP lets be framed; empty
// goes back.
func TestDrawioAddress(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	var b brandingResponse
	_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
	if b.DrawioURL != "https://embed.diagrams.net" {
		t.Fatalf("default: %q", b.DrawioURL)
	}
	if csp := do(r, "GET", "/api/v1/branding", "").Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src 'self' https://embed.diagrams.net;") {
		t.Errorf("csp: %s", csp)
	}
	owner := loginTok(t, r, "admin", "adminpass")
	for _, bad := range []string{"ftp://x", "javascript:alert(1)", "https://x.example/?a=1", "https://user@x.example"} {
		expect(t, authdo(r, "PUT", "/api/v1/settings", `{"drawio_url":"`+bad+`"}`, owner), http.StatusBadRequest, bad)
	}
	w := authdo(r, "PUT", "/api/v1/settings", `{"drawio_url":"https://drawio.example.org:8443/app/"}`, owner)
	expect(t, w, http.StatusOK, "set")
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if b.DrawioURL != "https://drawio.example.org:8443/app" {
		t.Errorf("set: %q", b.DrawioURL)
	}
	if csp := do(r, "GET", "/api/v1/branding", "").Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src 'self' https://drawio.example.org:8443;") {
		t.Errorf("csp after: %s", csp)
	}
	w = authdo(r, "PUT", "/api/v1/settings", `{"drawio_url":""}`, owner)
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if b.DrawioURL != "https://embed.diagrams.net" {
		t.Errorf("reset: %q", b.DrawioURL)
	}
}

// TestSketchColors: a person's own Quick Draw palette is kept with their
// preferences, colours checked.
func TestSketchColors(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var p struct {
		SketchColors []string `json:"sketch_colors"`
	}
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", owner), &p)
	if p.SketchColors == nil || len(p.SketchColors) != 0 {
		t.Fatalf("default: %#v", p.SketchColors)
	}
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"sketch_colors":["#ff0000","red"]}`, owner), http.StatusBadRequest, "not hex")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"sketch_colors":["#ff0000","#00AA88"]}`, owner), http.StatusOK, "save")
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", owner), &p)
	if len(p.SketchColors) != 2 || p.SketchColors[1] != "#00AA88" {
		t.Errorf("saved: %#v", p.SketchColors)
	}
}

// TestEditorThemes: Sheets, diagrams and whiteboards may each stay light or
// dark whatever Orbit's theme; "follow" goes back to following it.
func TestEditorThemes(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var p struct {
		EditorThemes map[string]string `json:"editor_themes"`
	}
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", owner), &p)
	if len(p.EditorThemes) != 0 {
		t.Fatalf("default: %#v", p.EditorThemes)
	}
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"editor_themes":{"pages":"light"}}`, owner), http.StatusBadRequest, "unknown editor")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"editor_themes":{"sheets":"blue"}}`, owner), http.StatusBadRequest, "unknown theme")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"editor_themes":{"sheets":"light","diagrams":"light"}}`, owner), http.StatusOK, "save")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"editor_themes":{"diagrams":"follow","whiteboards":"dark"}}`, owner), http.StatusOK, "change one")
	p.EditorThemes = nil
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", owner), &p)
	if len(p.EditorThemes) != 2 || p.EditorThemes["sheets"] != "light" || p.EditorThemes["whiteboards"] != "dark" {
		t.Errorf("saved: %#v", p.EditorThemes)
	}
}

// TestCalendarColors: a person's own colours for calendars are kept with
// their preferences; "" goes back to the calendar's own.
func TestCalendarColors(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var p struct {
		CalendarColors map[string]string `json:"calendar_colors"`
	}
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"calendar_colors":{"holidays":"grey"}}`, owner), http.StatusBadRequest, "not hex")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"calendar_colors":{"holidays":"#34C759","list:abc":"#FF9500"}}`, owner), http.StatusOK, "save")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"calendar_colors":{"holidays":""}}`, owner), http.StatusOK, "reset one")
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", owner), &p)
	if len(p.CalendarColors) != 1 || p.CalendarColors["list:abc"] != "#FF9500" {
		t.Errorf("saved: %#v", p.CalendarColors)
	}
}
