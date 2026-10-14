package api

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

type inboxResp struct {
	Items []struct {
		ID     string         `json:"id"`
		Kind   string         `json:"kind"`
		Verb   string         `json:"verb"`
		Params map[string]any `json:"params"`
		Link   string         `json:"link"`
		Count  int            `json:"count"`
		Read   bool           `json:"read"`
	} `json:"items"`
	Unread int `json:"unread"`
}

func inboxOf(t *testing.T, r *gin.Engine, tok string) inboxResp {
	t.Helper()
	var in inboxResp
	w := authdo(r, "GET", "/api/v1/me/notifications", "", tok)
	expect(t, w, http.StatusOK, "inbox")
	decode(t, w, &in)
	return in
}

// TestNotifications: being added to a workspace, being given a task, and
// others' activity (folded into one entry per burst, mutable) land in the
// inbox; reading clears the count; nothing is sent to the one who acted.
func TestNotifications(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Casa","kind":"shared"}`)
	var bobID struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "GET", "/api/v1/users/lookup?q=bob", "", alice), &bobID)

	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "invite")
	in := inboxOf(t, r, bob)
	if in.Unread != 1 || in.Items[0].Verb != "added" || in.Items[0].Params["ws"] != "Casa" || in.Items[0].Link != "/w/"+ws.ID {
		t.Fatalf("bob after the invite: %+v", in)
	}

	// Alice gives Bob a task: Bob hears of it (once), Alice of nothing.
	w := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Fix the tap","assignee_id":"`+bobID.ID+`"}`, alice)
	expect(t, w, http.StatusCreated, "task")
	var task struct {
		ID string `json:"id"`
	}
	decode(t, w, &task)
	authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"notes":"under the sink"}`, alice)
	in = inboxOf(t, r, bob)
	assigned, added := 0, 0
	for _, n := range in.Items {
		switch n.Verb {
		case "task_assigned":
			assigned++
		case "task_added":
			added++
		}
	}
	// The task made for Bob is news to him once: "assigned", not also "added".
	if assigned != 1 || added != 0 {
		t.Errorf("bob: assigned %d, added %d: %+v", assigned, added, in.Items)
	}
	if a := inboxOf(t, r, alice); a.Unread != 0 {
		t.Errorf("alice notified of her own actions: %+v", a)
	}

	// Bob adds three tasks: one entry for Alice, counting 3.
	for _, title := range []string{"Milk", "Bread", "Eggs"} {
		authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"`+title+`"}`, bob)
	}
	in = inboxOf(t, r, alice)
	if in.Unread != 1 || in.Items[0].Verb != "task_added" || in.Items[0].Count != 3 || in.Items[0].Params["title"] != "Eggs" {
		t.Fatalf("alice after the burst: %+v", in)
	}

	// Read it; mute the workspace; Bob's next task doesn't reach her.
	expect(t, authdo(r, "POST", "/api/v1/me/notifications/read", `{"all":true}`, alice), http.StatusNoContent, "read")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/mute", `{"muted":true}`, alice), http.StatusNoContent, "mute")
	authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Butter"}`, bob)
	if in = inboxOf(t, r, alice); in.Unread != 0 {
		t.Errorf("muted, still notified: %+v", in)
	}
	var wsList []struct {
		ID    string `json:"id"`
		Muted bool   `json:"muted"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces", "", alice), &wsList)
	if len(wsList) != 1 || !wsList[0].Muted {
		t.Errorf("muted flag: %+v", wsList)
	}

	// A role change and a removal reach Bob.
	authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/members/"+bobID.ID, `{"role":"viewer"}`, alice)
	authdo(r, "DELETE", "/api/v1/workspaces/"+ws.ID+"/members/"+bobID.ID, "", alice)
	in = inboxOf(t, r, bob)
	if in.Items[0].Verb != "removed" || in.Items[1].Verb != "role_changed" || in.Items[1].Params["role"] != "viewer" {
		t.Errorf("bob's membership news: %+v", in.Items[:2])
	}
}

// TestNotificationPrefs: per-kind choices, the summary time, the language;
// push devices must be https.
func TestNotificationPrefs(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	var p struct {
		NotifyPrefs map[string]struct {
			Push     bool `json:"push"`
			Channels bool `json:"channels"`
		} `json:"notify_prefs"`
		DigestEnabled bool   `json:"digest_enabled"`
		DigestTime    string `json:"digest_time"`
		Locale        string `json:"locale"`
	}
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", alice), &p)
	if !p.NotifyPrefs["assigned"].Push || p.NotifyPrefs["activity"].Push || p.DigestTime != "08:00" {
		t.Fatalf("defaults: %+v", p)
	}
	w := authdo(r, "PUT", "/api/v1/me/prefs", `{"notify_prefs":{"activity":{"push":true,"channels":false}},"digest_enabled":true,"digest_time":"07:30","locale":"pt"}`, alice)
	expect(t, w, http.StatusOK, "save")
	decode(t, w, &p)
	if !p.NotifyPrefs["activity"].Push || !p.NotifyPrefs["assigned"].Push || !p.DigestEnabled || p.DigestTime != "07:30" || p.Locale != "pt" {
		t.Errorf("saved: %+v", p)
	}
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"digest_time":"7h"}`, alice), http.StatusBadRequest, "bad time")

	expect(t, authdo(r, "POST", "/api/v1/me/push", `{"endpoint":"http://10.0.0.1/x","keys":{"p256dh":"a","auth":"b"}}`, alice), http.StatusBadRequest, "http endpoint")
	expect(t, authdo(r, "POST", "/api/v1/me/push", `{"endpoint":"https://push.example.org/abc","keys":{"p256dh":"a","auth":"b"}}`, alice), http.StatusNoContent, "subscribe")
	var info struct {
		Available bool   `json:"available"`
		PublicKey string `json:"public_key"`
		Devices   []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	decode(t, authdo(r, "GET", "/api/v1/me/push", "", alice), &info)
	if !info.Available || info.PublicKey == "" || len(info.Devices) != 1 {
		t.Fatalf("push info: %+v", info)
	}
	expect(t, authdo(r, "POST", "/api/v1/me/push/remove", `{"id":"`+info.Devices[0].ID+`"}`, alice), http.StatusNoContent, "remove")
	decode(t, authdo(r, "GET", "/api/v1/me/push", "", alice), &info)
	if len(info.Devices) != 0 {
		t.Errorf("device not removed")
	}
}
