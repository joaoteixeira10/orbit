package api

import (
	"net/http"
	"testing"
)

// TestWorkspaceFeed: members' additions, completions, comments and joins are
// a workspace's feed, newest first, with who did them; outsiders can't read
// it.
func TestWorkspaceFeed(t *testing.T) {
	r, admin, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Casa","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Chores"}`, alice), http.StatusCreated, "list")
	var task struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Fix the tap"}`, bob), &task)
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"completed":true}`, bob), http.StatusOK, "done")
	expect(t, authdo(r, "POST", "/api/v1/tasks/"+task.ID+"/comments", `{"body":"Thanks!"}`, alice), http.StatusCreated, "comment")

	var feed []struct {
		Verb      string `json:"verb"`
		Title     string `json:"title"`
		ActorName string `json:"actor_name"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/activity", "", bob), &feed)
	want := []string{"commented", "task_done", "task_added", "member_joined"}
	if len(feed) != len(want) {
		t.Fatalf("feed: %+v", feed)
	}
	for i, v := range want {
		if feed[i].Verb != v {
			t.Fatalf("feed[%d] = %+v, want %s", i, feed[i], v)
		}
	}
	if feed[0].ActorName != "alice" || feed[1].ActorName != "bob" || feed[1].Title != "Fix the tap" {
		t.Fatalf("who / what: %+v", feed)
	}
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/activity", "", admin), http.StatusNotFound, "outsider")
}
