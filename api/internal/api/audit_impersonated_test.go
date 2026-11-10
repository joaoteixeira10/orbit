package api

import "testing"

func TestChangeOf(t *testing.T) {
	for _, c := range []struct{ method, route, id, ws, action, typ, target string }{
		{"POST", "/api/v1/workspaces/:ws/tasks", "", "w1", "task.create", "workspace", "w1"},
		{"PUT", "/api/v1/tasks/:id", "t1", "", "task.update", "task", "t1"},
		{"DELETE", "/api/v1/tasks/:id", "t1", "", "task.delete", "task", "t1"},
		{"POST", "/api/v1/tasks/:id/move", "t1", "", "task.move", "task", "t1"},
		{"POST", "/api/v1/tasks/:id/comments", "t1", "", "comment.create", "task", "t1"},
		{"DELETE", "/api/v1/pages/:id/favorite", "p1", "", "page.favorite_off", "page", "p1"},
		{"POST", "/api/v1/me/notes", "", "", "note.create", "", ""},
		{"POST", "/api/v1/me/notifications/read", "", "", "notification.read", "", ""},
		{"PUT", "/api/v1/workspaces/:ws/lists/order", "", "w1", "list.order", "workspace", "w1"},
		{"PUT", "/api/v1/habits/:id/checks/:date", "h1", "", "check.update", "habit", "h1"},
	} {
		a, typ, target := changeOf(c.method, c.route, c.id, c.ws)
		if a != c.action || typ != c.typ || target != c.target {
			t.Errorf("%s %s: %s %s %s, want %s %s %s", c.method, c.route, a, typ, target, c.action, c.typ, c.target)
		}
	}
}
