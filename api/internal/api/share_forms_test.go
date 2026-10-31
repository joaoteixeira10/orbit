package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// TestGuestLinks: an editor makes a link to a page and what's under it; a
// guest (no account) reads them, and what they show — and nothing else; an
// edit link lets them edit (by version); a read one doesn't; a link taken
// back, expired, or with guest links off opens nothing.
func TestGuestLinks(t *testing.T) {
	r, db, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Docs","kind":"work","starter":false,"modules":["home","pages","sheets"]}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "bob in")
	base := "/api/v1/workspaces/" + ws.ID
	var sheet, other, root, child, outside struct{ ID string }
	decode(t, authdo(r, "POST", base+"/sheets", `{"title":"Budget","data":{"tabs":[]}}`, alice), &sheet)
	decode(t, authdo(r, "POST", base+"/sheets", `{"title":"Secret","data":{"tabs":[]}}`, alice), &other)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Handbook","content":"Welcome.\n\n[Budget](/w/`+ws.ID+`/s/`+sheet.ID+`)"}`, alice), &root)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Holidays","parent_id":"`+root.ID+`","content":"25 days."}`, alice), &child)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Salaries","content":"private"}`, alice), &outside)

	expect(t, authdo(r, "POST", "/api/v1/pages/"+root.ID+"/links", `{"mode":"read"}`, bob), http.StatusForbidden, "viewer can't")
	var link struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		Mode string `json:"mode"`
	}
	w := authdo(r, "POST", "/api/v1/pages/"+root.ID+"/links", `{"mode":"read","subpages":true}`, alice)
	expect(t, w, http.StatusCreated, "make link")
	decode(t, w, &link)
	tok := link.URL[strings.LastIndex(link.URL, "/")+1:]
	g := "/api/v1/shared/" + tok

	var shared struct {
		Mode  string `json:"mode"`
		Page  struct{ Title, Content string }
		Pages []struct{ ID, Title string }
	}
	decode(t, do(r, "GET", g, ""), &shared)
	if shared.Page.Title != "Handbook" || len(shared.Pages) != 2 || shared.Mode != "read" {
		t.Fatalf("shared: %+v", shared)
	}
	expect(t, do(r, "GET", g+"/pages/"+child.ID, ""), http.StatusOK, "sub-page")
	expect(t, do(r, "GET", g+"/pages/"+outside.ID, ""), http.StatusNotFound, "a page it doesn't open")
	expect(t, do(r, "GET", g+"/embed/sheet/"+sheet.ID, ""), http.StatusOK, "a sheet it shows")
	expect(t, do(r, "GET", g+"/embed/sheet/"+other.ID, ""), http.StatusNotFound, "a sheet it doesn't show")
	expect(t, do(r, "PUT", g+"/pages/"+root.ID, `{"content":"x","version":1}`), http.StatusForbidden, "read link")

	// Edit: by version; a stale one is refused; history says a guest did it.
	expect(t, authdo(r, "PUT", "/api/v1/links/"+link.ID, `{"mode":"edit"}`, alice), http.StatusOK, "to edit")
	var pg struct{ Version int }
	decode(t, do(r, "GET", g+"/pages/"+child.ID, ""), &pg)
	expect(t, do(r, "PUT", g+"/pages/"+child.ID, `{"content":"26 days.","version":`+strconv.Itoa(pg.Version)+`}`), http.StatusOK, "guest edit")
	expect(t, do(r, "PUT", g+"/pages/"+child.ID, `{"content":"27 days.","version":`+strconv.Itoa(pg.Version)+`}`), http.StatusConflict, "stale")
	var full struct{ Content string }
	decode(t, authdo(r, "GET", "/api/v1/pages/"+child.ID, "", alice), &full)
	if full.Content != "26 days." {
		t.Fatalf("after guest edit: %q", full.Content)
	}
	var vs []struct {
		UserID string `json:"user_id"`
	}
	decode(t, authdo(r, "GET", "/api/v1/pages/"+child.ID+"/versions", "", alice), &vs)
	if len(vs) == 0 || vs[0].UserID != "" {
		t.Fatalf("history: %+v", vs)
	}

	// Without sub-pages: only the page.
	expect(t, authdo(r, "PUT", "/api/v1/links/"+link.ID, `{"subpages":false}`, alice), http.StatusOK, "no sub-pages")
	expect(t, do(r, "GET", g+"/pages/"+child.ID, ""), http.StatusNotFound, "sub-page closed")

	// Off for the instance, expired, taken back: nothing.
	_ = feature.Set(db, feature.GuestLinks, false)
	expect(t, do(r, "GET", g, ""), http.StatusNotFound, "feature off")
	_ = feature.Set(db, feature.GuestLinks, true)
	db.Model(&models.ShareLink{}).Where("id = ?", link.ID).Update("expires_at", "2000-01-01 00:00:00")
	expect(t, do(r, "GET", g, ""), http.StatusNotFound, "expired")
	expect(t, authdo(r, "DELETE", "/api/v1/links/"+link.ID, "", alice), http.StatusNoContent, "take back")
	expect(t, do(r, "GET", g, ""), http.StatusNotFound, "taken back")
	expect(t, do(r, "GET", "/api/v1/shared/nonsense", ""), http.StatusNotFound, "made up")
}

// TestForms: a database's form asks for some of its properties; members
// answer it (viewers too); a public one is answered by its link without an
// account; answers are checked (required, types, choices); each makes a
// row; a closed one takes none; robots' answers are dropped.
func TestForms(t *testing.T) {
	r, db, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Events","kind":"work","starter":false,"modules":["home","pages"]}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "bob in")
	dbJSON := `{"props":[{"id":"size","name":"Size","type":"select","options":[{"id":"s","name":"S"},{"id":"m","name":"M"}]},{"id":"n","name":"Guests","type":"number"},{"id":"veg","name":"Vegetarian","type":"checkbox"},{"id":"f","name":"Total","type":"formula","formula":"=1"}],"views":[{"id":"v","name":"All","type":"table"}]}`
	var dbp struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"RSVPs","db":`+dbJSON+`}`, alice), &dbp)
	var plain struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Not a database"}`, alice), &plain)
	expect(t, authdo(r, "POST", "/api/v1/pages/"+plain.ID+"/forms", `{}`, alice), http.StatusBadRequest, "not a database")
	expect(t, authdo(r, "POST", "/api/v1/pages/"+dbp.ID+"/forms", `{"fields":[{"prop":"f"}]}`, alice), http.StatusBadRequest, "formula")

	var form struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Public bool   `json:"public"`
	}
	body := `{"title":"Party RSVP","fields":[{"prop":"title","label":"Your name","required":true},{"prop":"size","required":true},{"prop":"n"},{"prop":"veg"}],"thanks":"See you!"}`
	w := authdo(r, "POST", "/api/v1/pages/"+dbp.ID+"/forms", body, alice)
	expect(t, w, http.StatusCreated, "make form")
	decode(t, w, &form)
	if form.URL != "" {
		t.Fatalf("not public yet: %+v", form)
	}
	var fill struct {
		Fields []struct{ Prop, Label, Type string }
	}
	decode(t, authdo(r, "GET", "/api/v1/forms/"+form.ID, "", bob), &fill)
	if len(fill.Fields) != 4 || fill.Fields[0].Label != "Your name" || fill.Fields[1].Label != "Size" {
		t.Fatalf("fill: %+v", fill)
	}
	expect(t, authdo(r, "POST", "/api/v1/forms/"+form.ID+"/answers", `{"values":{"size":"s"}}`, bob), http.StatusBadRequest, "name needed")
	expect(t, authdo(r, "POST", "/api/v1/forms/"+form.ID+"/answers", `{"values":{"title":"Bob","size":"xl"}}`, bob), http.StatusBadRequest, "not a choice")
	expect(t, authdo(r, "POST", "/api/v1/forms/"+form.ID+"/answers", `{"values":{"title":"Bob","size":"m","n":"two"}}`, bob), http.StatusBadRequest, "not a number")
	expect(t, authdo(r, "POST", "/api/v1/forms/"+form.ID+"/answers", `{"values":{"title":"Bob","size":"m","n":"2","veg":true}}`, bob), http.StatusCreated, "viewer answers")

	// Public: by its link, no account.
	w = authdo(r, "PUT", "/api/v1/forms/"+form.ID, `{"public":true}`, alice)
	decode(t, w, &form)
	if !form.Public || !strings.Contains(form.URL, "/form/") {
		t.Fatalf("public: %+v", form)
	}
	tok := form.URL[strings.LastIndex(form.URL, "/")+1:]
	expect(t, do(r, "GET", "/api/v1/f/"+tok, ""), http.StatusOK, "public form")
	expect(t, do(r, "POST", "/api/v1/f/"+tok, `{"values":{"title":"Ana","size":"s"}}`), http.StatusCreated, "public answer")
	expect(t, do(r, "POST", "/api/v1/f/"+tok, `{"values":{"title":"Bot","size":"s","_hp":"http://spam"}}`), http.StatusOK, "robot")

	var rows []struct {
		Title    string          `json:"title"`
		ParentID *string         `json:"parent_id"`
		Props    json.RawMessage `json:"props"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", alice), &rows)
	got := map[string]string{}
	for _, x := range rows {
		if x.ParentID != nil && *x.ParentID == dbp.ID {
			got[x.Title] = string(x.Props)
		}
	}
	if len(got) != 2 || !strings.Contains(got["Bob"], `"n":2`) || !strings.Contains(got["Bob"], `"veg":true`) || !strings.Contains(got["Ana"], `"size":"s"`) {
		t.Fatalf("rows: %v", got)
	}
	var f models.Form
	db.First(&f, "id = ?", form.ID)
	if f.Answers != 2 {
		t.Fatalf("answers counted: %d", f.Answers)
	}

	// Closed: no more answers; off for the instance: the link is gone.
	expect(t, authdo(r, "PUT", "/api/v1/forms/"+form.ID, `{"open":false}`, alice), http.StatusOK, "close")
	expect(t, do(r, "POST", "/api/v1/f/"+tok, `{"values":{"title":"Late","size":"s"}}`), http.StatusConflict, "closed")
	_ = feature.Set(db, feature.PublicForms, false)
	expect(t, do(r, "GET", "/api/v1/f/"+tok, ""), http.StatusNotFound, "public forms off")
	expect(t, authdo(r, "PUT", "/api/v1/forms/"+form.ID, `{"public":true}`, alice), http.StatusForbidden, "can't make public while off")
}

// TestGuestMarkdown: a guest link's page as clean Markdown (for agents): its
// links to the pages it opens lead to their Markdown, a sheet it shows comes
// as a table, a Mermaid diagram as its code; ?all=1 every page in order; a
// page it doesn't open, nothing.
func TestGuestMarkdown(t *testing.T) {
	r, _, _, alice, _ := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Docs","kind":"work","starter":false,"modules":["home","pages","sheets","diagrams"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	var sheet, diag, root, child, outside struct{ ID string }
	decode(t, authdo(r, "POST", base+"/sheets", `{"title":"Budget","data":{"tabs":[{"id":"t","name":"Sheet 1","cells":{"A1":{"v":"Rent"},"B1":{"v":"650"},"B2":{"v":"=SUM(B1)"}}}]}}`, alice), &sheet)
	decode(t, authdo(r, "POST", base+"/diagrams", `{"title":"Flow","kind":"mermaid","data":"flowchart TD\n  A --> B"}`, alice), &diag)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Salaries","content":"private"}`, alice), &outside)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Handbook","content":"Welcome."}`, alice), &root)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Holidays","parent_id":"`+root.ID+`","content":"25 days."}`, alice), &child)
	content := "Welcome. See [holidays](/w/" + ws.ID + "/p/" + child.ID + ") and [pay](/w/" + ws.ID + "/p/" + outside.ID + ").\n\n[Budget](/w/" + ws.ID + "/s/" + sheet.ID + ")\n\n[Flow](/w/" + ws.ID + "/d/" + diag.ID + ")"
	body, _ := json.Marshal(map[string]any{"content": content, "version": 1})
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+root.ID, string(body), alice), http.StatusOK, "content")
	var link struct{ URL string }
	decode(t, authdo(r, "POST", "/api/v1/pages/"+root.ID+"/links", `{"mode":"read","subpages":true}`, alice), &link)
	g := "/api/v1/shared/" + link.URL[strings.LastIndex(link.URL, "/")+1:]

	w := do(r, "GET", g+"/markdown", "")
	expect(t, w, http.StatusOK, "markdown")
	md := w.Body.String()
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/markdown") || !strings.HasPrefix(md, "# Handbook\n") {
		t.Fatalf("markdown: %q (%s)", md, w.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		g + "/markdown?page=" + child.ID,    // a page it opens: its Markdown
		"/w/" + ws.ID + "/p/" + outside.ID,  // one it doesn't: Orbit's own link
		"| | A | B |", "| 1 | Rent | 650 |", // the sheet as a table
		"| 2 |  | =SUM(B1) |",                      // a formula as written
		"```mermaid\nflowchart TD\n  A --> B\n```", // the diagram as its code
		"## Pages inside", "[Holidays]",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	all := do(r, "GET", g+"/markdown?all=1", "").Body.String()
	if !strings.Contains(all, "# Handbook") || !strings.Contains(all, "# Holidays\n\n25 days.") || strings.Index(all, "# Holidays") < strings.Index(all, "# Handbook") {
		t.Fatalf("all: %s", all)
	}
	expect(t, do(r, "GET", g+"/markdown?page="+outside.ID, ""), http.StatusNotFound, "a page it doesn't open")
}
