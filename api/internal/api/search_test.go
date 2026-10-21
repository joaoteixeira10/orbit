package api

import (
	"net/http"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/search"
)

// TestSearchIndex: with the full-text index, search finds by word start and
// without accents, opens a task from its comment, skips trashed pages and
// other people's workspaces.
func TestSearchIndex(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	if !search.Setup(db) {
		t.Fatal("no index")
	}
	t.Cleanup(func() { search.Setup(nil) })
	admin := loginTok(t, r, "admin", "adminpass")
	createUserVia(t, r, admin, `{"username":"bob","password":"bobpassword"}`)
	bob := loginTok(t, r, "bob", "bobpassword")
	ws := createWS(t, r, admin, `{"name":"Casa"}`)
	other := createWS(t, r, bob, `{"name":"Bob's"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"L"}`, admin), http.StatusCreated, "list")
	var task struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Pagar a eletricidade"}`, admin), &task)
	authdo(r, "POST", "/api/v1/tasks/"+task.ID+"/comments", `{"body":"Fatura de outubro"}`, admin)
	authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Orçamento da casa"}`, admin)
	authdo(r, "POST", "/api/v1/workspaces/"+other.ID+"/pages", `{"title":"Orçamento do Bob"}`, bob)

	var hits []struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	decode(t, authdo(r, "GET", "/api/v1/search?q=orcam", "", admin), &hits)
	if len(hits) != 1 || hits[0].Kind != "page" {
		t.Fatalf("orcam: %+v", hits)
	}
	decode(t, authdo(r, "GET", "/api/v1/search?q=fatura", "", admin), &hits)
	if len(hits) != 1 || hits[0].Kind != "task" || hits[0].ID != task.ID {
		t.Fatalf("a comment opens its task: %+v", hits)
	}
	decode(t, authdo(r, "GET", "/api/v1/search?q=eletri", "", admin), &hits)
	if len(hits) != 1 {
		t.Fatalf("prefix: %+v", hits)
	}
}
