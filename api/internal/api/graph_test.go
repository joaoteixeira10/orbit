package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestGraph: a workspace's graph joins pages, files, lists, folders and the
// people pages mention, by links, embeds, mentions, sub-pages and folders;
// one workspace's links to another's things aren't in it; viewers only.
func TestGraph(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Map","kind":"work","starter":false,"modules":["home","tasks","pages","sheets","diagrams","whiteboards","files"]}`)
	other := createWS(t, r, alice, `{"name":"Away","kind":"personal","starter":false,"modules":["home","pages"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	var list, folder, sheet, board, away, root struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"Launch"}`, alice), &list)
	decode(t, authdo(r, "POST", base+"/folders", `{"name":"Launch","list_id":"`+list.ID+`"}`, alice), &folder)
	decode(t, authdo(r, "POST", base+"/sheets", `{"title":"Budget","folder_id":"`+folder.ID+`","data":{"tabs":[]}}`, alice), &sheet)
	decode(t, authdo(r, "POST", base+"/drawings", `{"title":"Sketch"}`, alice), &board)
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+other.ID+"/pages", `{"title":"Elsewhere"}`, alice), &away)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Plan","content":"Costs:\n\n[Budget](/w/`+ws.ID+`/s/`+sheet.ID+`)\n\nSee [sketch](/w/`+ws.ID+`/whiteboards/`+board.ID+`) and [away](/w/`+other.ID+`/p/`+away.ID+`), ask @alice."}`, alice), &root)
	authdo(r, "POST", base+"/pages", `{"title":"Details","parent_id":"`+root.ID+`"}`, alice)
	authdo(r, "POST", base+"/tasks", `{"title":"Draw","list_id":"`+list.ID+`","notes":"on [the board](/w/`+ws.ID+`/whiteboards/`+board.ID+`)"}`, alice)

	expect(t, authdo(r, "GET", base+"/graph", "", bob), http.StatusNotFound, "not a member")
	var g struct {
		Nodes []struct{ ID, Kind, Title string }
		Edges []struct{ From, To, Kind string }
	}
	decode(t, authdo(r, "GET", base+"/graph", "", alice), &g)
	kinds := map[string]string{}
	for _, n := range g.Nodes {
		kinds[n.Title] = n.Kind
	}
	if kinds["Plan"] != "page" || kinds["Budget"] != "sheet" || kinds["Launch"] == "" || kinds["Sketch"] != "board" || kinds["Elsewhere"] != "" {
		t.Fatalf("nodes: %+v", g.Nodes)
	}
	name := map[string]string{}
	for _, n := range g.Nodes {
		name[n.ID] = n.Kind + ":" + n.Title
	}
	got := map[string]bool{}
	for _, e := range g.Edges {
		got[name[e.From]+" -"+e.Kind+"-> "+name[e.To]] = true
	}
	for _, want := range []string{
		"page:Plan -embed-> sheet:Budget",
		"page:Plan -link-> board:Sketch",
		"page:Plan -parent-> page:Details",
		"folder:Launch -list-> list:Launch",
		"folder:Launch -folder-> sheet:Budget",
		"list:Launch -link-> board:Sketch",
	} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	mentioned := false
	for k := range got {
		mentioned = mentioned || strings.HasPrefix(k, "page:Plan -mention-> person:")
	}
	if !mentioned {
		t.Errorf("no mention: %v", got)
	}
}
