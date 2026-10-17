package api

import (
	"net/http"
	"testing"
)

// TestComments: members comment on a task (viewers too); the assignee and
// earlier commenters are told, @mentions too; only the author edits, an
// owner may delete; deleting the task takes its comments.
func TestComments(t *testing.T) {
	r, admin, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Casa","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "bob in")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Chores"}`, alice), http.StatusCreated, "list")
	var task struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Fix the tap"}`, alice), &task)
	path := "/api/v1/tasks/" + task.ID + "/comments"

	expect(t, authdo(r, "POST", path, `{"body":"hello"}`, admin), http.StatusNotFound, "non-member")
	expect(t, authdo(r, "POST", path, `{"body":"   "}`, bob), http.StatusBadRequest, "empty")
	w := authdo(r, "POST", path, `{"body":"I can **look** at it on Saturday"}`, bob)
	expect(t, w, http.StatusCreated, "viewer comments")
	var c1 struct {
		ID         string `json:"id"`
		AuthorName string `json:"author_name"`
	}
	decode(t, w, &c1)
	if c1.AuthorName != "bob" {
		t.Fatalf("comment: %+v", c1)
	}
	// Alice made the task: she hears of Bob's comment.
	verbs := func(tok string) map[string]int {
		m := map[string]int{}
		for _, it := range inboxOf(t, r, tok).Items {
			m[it.Verb]++
		}
		return m
	}
	if verbs(alice)["task_comment"] != 1 {
		t.Fatalf("alice: %v", verbs(alice))
	}
	// Alice answers and mentions Bob: he's told he was mentioned (not twice).
	expect(t, authdo(r, "POST", path, `{"body":"Thanks @bob!"}`, alice), http.StatusCreated, "alice comments")
	if v := verbs(bob); v["mentioned"] != 1 || v["task_comment"] != 0 {
		t.Fatalf("bob: %v", v)
	}

	var tasks []struct {
		ID       string `json:"id"`
		Comments int    `json:"comment_count"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/tasks", "", bob), &tasks)
	if len(tasks) != 1 || tasks[0].Comments != 2 {
		t.Fatalf("tasks: %+v", tasks)
	}
	var list []struct {
		Body string `json:"body"`
	}
	decode(t, authdo(r, "GET", path, "", bob), &list)
	if len(list) != 2 || list[1].Body != "Thanks @bob!" {
		t.Fatalf("comments: %+v", list)
	}
	// Only the author edits; the owner may delete Bob's.
	expect(t, authdo(r, "PUT", "/api/v1/comments/"+c1.ID, `{"body":"x"}`, alice), http.StatusForbidden, "alice edits bob's")
	w = authdo(r, "PUT", "/api/v1/comments/"+c1.ID, `{"body":"Sunday, sorry"}`, bob)
	expect(t, w, http.StatusOK, "bob edits")
	var ed struct {
		EditedAt *string `json:"edited_at"`
	}
	decode(t, w, &ed)
	if ed.EditedAt == nil {
		t.Fatal("not marked edited")
	}
	expect(t, authdo(r, "DELETE", "/api/v1/comments/"+c1.ID, "", alice), http.StatusNoContent, "owner deletes")

	expect(t, authdo(r, "DELETE", "/api/v1/tasks/"+task.ID, "", alice), http.StatusNoContent, "delete task")
	expect(t, authdo(r, "GET", path, "", alice), http.StatusNotFound, "comments of a deleted task")
}
