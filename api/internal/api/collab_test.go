package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPresenceAndCollab: two people on a page see each other; a change
// joins the page's log, in order, read from where a tab left off; one tab
// may start an empty log; a fold keeps what's after it; saved by the tab
// saving for everyone, the log goes on — saved any other way, it starts
// over; a viewer reads but doesn't add; others see nothing.
func TestPresenceAndCollab(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Team","kind":"work","starter":false,"modules":["home","pages"]}`)
	var page struct {
		ID      string
		Version int
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Plan","content":"Hi"}`, alice), &page)
	doc := "/api/v1/collab/page/" + page.ID
	pres := "/api/v1/presence/page/" + page.ID
	type person struct {
		Client  string
		Name    string
		Editing bool
	}
	expect(t, authdo(r, "POST", pres, `{"client":"b1"}`, bob), http.StatusNotFound, "not a member")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "bob in")

	var people []person
	decode(t, authdo(r, "POST", pres, `{"client":"a1","editing":true}`, alice), &people)
	decode(t, authdo(r, "POST", pres, `{"client":"b1"}`, bob), &people)
	if len(people) != 2 || people[0].Client != "a1" || !people[0].Editing || people[1].Name != "bob" {
		t.Fatalf("who's here: %+v", people)
	}
	decode(t, authdo(r, "POST", pres, `{"client":"b1","leave":true}`, bob), &people)
	if len(people) != 1 {
		t.Fatalf("after bob left: %+v", people)
	}

	// The log: empty (start it), then in order.
	type log struct {
		Seq     int
		Fresh   bool
		Updates []struct {
			Seq    int
			Data   []byte
			Client string
		}
	}
	var l log
	decode(t, authdo(r, "GET", doc, "", alice), &l)
	if !l.Fresh || l.Seq != 0 {
		t.Fatalf("a new log: %+v", l)
	}
	var seed struct{ Go bool }
	decode(t, authdo(r, "POST", doc+"/seed", `{"client":"a1"}`, alice), &seed)
	if !seed.Go {
		t.Fatal("the first tab may start it")
	}
	decode(t, authdo(r, "POST", doc+"/seed", `{"client":"a2"}`, alice), &seed)
	if seed.Go {
		t.Fatal("a second tab may not, meanwhile")
	}
	expect(t, authdo(r, "POST", doc, `{"client":"b1","data":"AQI="}`, bob), http.StatusForbidden, "a viewer adds")
	for _, d := range []string{"AQ==", "Ag==", "Aw=="} {
		expect(t, authdo(r, "POST", doc, `{"client":"a1","data":"`+d+`"}`, alice), http.StatusOK, "add")
	}
	decode(t, authdo(r, "GET", doc+"?since=1", "", bob), &l)
	if l.Seq != 3 || len(l.Updates) != 2 || l.Updates[0].Seq != 2 || l.Updates[1].Data[0] != 3 {
		t.Fatalf("since 1: %+v", l)
	}
	// Folded up to 2: one change for both, the third after it.
	expect(t, authdo(r, "POST", doc+"/compact", `{"client":"a1","upto":2,"data":"DA=="}`, alice), http.StatusNoContent, "fold")
	decode(t, authdo(r, "GET", doc, "", alice), &l)
	if len(l.Updates) != 2 || l.Updates[0].Seq != 2 || l.Updates[0].Data[0] != 12 || l.Updates[1].Seq != 3 {
		t.Fatalf("folded: %+v", l)
	}

	// Saved by the tab saving for everyone: the log goes on.
	w := authdoH(r, "PUT", "/api/v1/pages/"+page.ID, `{"content":"Hi there","version":1}`, alice, map[string]string{"X-Orbit-Collab": "1"})
	expect(t, w, http.StatusOK, "collab save")
	decode(t, authdo(r, "GET", doc, "", alice), &l)
	if l.Seq != 3 || l.Fresh {
		t.Fatalf("after its own save: %+v", l)
	}
	// Saved another way: it starts over.
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+page.ID, `{"content":"Rewritten","version":2}`, alice), http.StatusOK, "plain save")
	decode(t, authdo(r, "GET", doc, "", alice), &l)
	if !l.Fresh || l.Seq != 0 {
		t.Fatalf("after another save: %+v", l)
	}
}

// authdoH is authdo with more headers.
func authdoH(r http.Handler, method, path, body, token string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(w, req)
	return w
}
