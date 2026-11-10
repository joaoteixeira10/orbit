package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// While an admin acts as someone, every change they make is audited — under
// the person and the admin (impersonator_id), shown with a "via admin" pill —
// not only the kinds of change audited anyway. A change a handler audits
// itself (recordCtx) isn't recorded twice; the same change to the same thing
// again within a few minutes (typing in a page) is one entry.

const ctxAudited = "audit.recorded" // set by recordCtx

// impersonatedSkip: writes that aren't changes someone made (presence, the
// live-editing plumbing, the event stream's ticket, the view settings the
// app keeps as one looks around, ending the session — audited on its own).
var impersonatedSkip = []string{"/presence/", "/collab/", "/auth/events/ticket", "/me/prefs", "/impersonate/end"}

const impersonatedRepeat = 3 * time.Minute

type impersonatedSeen struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (s *impersonatedSeen) again(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil || len(s.last) > 5000 {
		s.last = map[string]time.Time{}
	}
	if t, ok := s.last[key]; ok && now.Sub(t) < impersonatedRepeat {
		return true
	}
	s.last[key] = now
	return false
}

// auditImpersonated records each successful change made while impersonating.
func (h *Handlers) auditImpersonated() gin.HandlerFunc {
	seen := &impersonatedSeen{}
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		route := c.FullPath()
		for _, s := range impersonatedSkip {
			if strings.Contains(route, s) {
				c.Next()
				return
			}
		}
		// Before the change: what it's on, by name (a deleted one has none
		// after), and a new one's name as sent.
		var name, wsID, sent string
		if auth.ImpersonatorID(c) != "" {
			name, wsID = h.thingName(route, c.Param("id"))
			sent = sentName(c)
		}
		c.Next()
		imp := auth.ImpersonatorID(c)
		if imp == "" || c.Writer.Status() >= 400 || c.GetBool(ctxAudited) {
			return
		}
		action, targetType, targetID := changeOf(c.Request.Method, route, c.Param("id"), c.Param("ws"))
		if action == "" {
			return
		}
		uid := auth.UserID(c)
		if seen.again(imp+"|"+uid+"|"+action+"|"+targetID, time.Now()) {
			return
		}
		// Shown as the audit log shows any change: where (scope), on what (targets).
		detail := gin.H{"route": c.Request.Method + " " + strings.TrimPrefix(route, "/api/v1")}
		if wsID == "" {
			wsID = c.Param("ws")
		}
		if wsID != "" {
			var w models.Workspace
			if h.db.Select("name").First(&w, "id = ?", wsID).Error == nil {
				detail["scope"] = w.Name
			}
		}
		thing := strings.TrimSuffix(action[:strings.Index(action, ".")], "s")
		switch {
		case name != "":
			detail["targets"] = []string{targetType + " · " + name}
		case sent != "":
			detail["targets"] = []string{thing + " · " + sent}
		}
		h.audit.RecordImpersonated(uid, &imp, action, targetType, targetID, c.ClientIP(), detail)
	}
}

// impersonatedNames: what a route's :id is, where its name is kept.
var impersonatedNames = map[string]struct {
	model any
	col   string
}{
	"tasks": {&models.Task{}, "title"}, "pages": {&models.Page{}, "title"}, "notes": {&models.Note{}, "title"},
	"lists": {&models.TaskList{}, "name"}, "events": {&models.Event{}, "title"}, "comments": {&models.TaskComment{}, "body"},
	"habits": {&models.Habit{}, "title"}, "goals": {&models.Goal{}, "title"}, "bookmarks": {&models.Bookmark{}, "title"},
	"sheets": {&models.Sheet{}, "title"}, "diagrams": {&models.Diagram{}, "title"}, "drawings": {&models.Drawing{}, "title"},
	"folders": {&models.Folder{}, "name"}, "files": {&models.Upload{}, "name"}, "uploads": {&models.Upload{}, "name"},
	"courses": {&models.Course{}, "name"}, "grades": {&models.Grade{}, "title"}, "smart-lists": {&models.SmartList{}, "name"},
	"workspaces": {&models.Workspace{}, "name"}, // for the audit log's names (its routes say :ws)
}

// thingName is the name and workspace of what a route's :id is ("" when
// it's none of the kinds above, or nameless).
func (h *Handlers) thingName(route, id string) (string, string) {
	if id == "" {
		return "", ""
	}
	segs := strings.Split(route, "/")
	for i, sg := range segs {
		if sg != ":id" || i == 0 {
			continue
		}
		k, ok := impersonatedNames[segs[i-1]]
		if !ok {
			return "", ""
		}
		var row struct {
			Name        string
			WorkspaceID string
		}
		if h.db.Model(k.model).Select(k.col+" AS name", "workspace_id").Where("id = ?", id).Scan(&row).Error != nil {
			return "", ""
		}
		return clip(strings.Join(strings.Fields(row.Name), " "), 80), row.WorkspaceID
	}
	return "", ""
}

// sentName is the title or name in a JSON body (a new thing's), the body
// left for the handler to read.
func sentName(c *gin.Context) string {
	if c.Request.Body == nil || !strings.HasPrefix(c.ContentType(), "application/json") {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	rest, _ := io.ReadAll(c.Request.Body) // past the limit: kept as it was
	c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(b), bytes.NewReader(rest)))
	if err != nil {
		return ""
	}
	var v struct{ Title, Name string }
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return clip(strings.TrimSpace(v.Title+v.Name), 80)
}

// changeOf names a change from its route, as the audited ones are named:
// POST /workspaces/:ws/tasks → task.create, PUT /tasks/:id → task.update,
// POST /tasks/:id/move → task.move, DELETE /pages/:id/favorite →
// page.favorite_off. Its target: the thing (:id) or the workspace (:ws).
func changeOf(method, route, id, ws string) (action, targetType, targetID string) {
	var segs []string
	for _, s := range strings.Split(strings.TrimPrefix(route, "/api/v1/"), "/") {
		if s != "" && s != "me" {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return "", "", ""
	}
	isParam := func(s string) bool { return strings.HasPrefix(s, ":") || strings.HasPrefix(s, "*") }
	// The last word, and the thing it's on.
	word, resource := "", ""
	for i := len(segs) - 1; i >= 0; i-- {
		if isParam(segs[i]) {
			continue
		}
		if word == "" {
			word = segs[i]
			continue
		}
		resource = segs[i]
		break
	}
	verb := map[string]string{http.MethodPost: "create", http.MethodPut: "update", http.MethodPatch: "update", http.MethodDelete: "delete"}[method]
	last := segs[len(segs)-1]
	switch {
	case isParam(last), strings.HasSuffix(word, "s"), resource == "":
		// A thing (…/:id) or a collection (…/tasks): created, changed, deleted.
		resource = word
	default:
		// Something done to a thing: …/:id/move, …/notifications/read.
		verb = word
		if method == http.MethodDelete {
			verb += "_off"
		}
	}
	if verb == "" {
		return "", "", ""
	}
	// The target: what :id is (the word before it), else the workspace.
	for i, sg := range segs {
		if sg == ":id" && i > 0 && id != "" {
			targetType, targetID = strings.TrimSuffix(segs[i-1], "s"), id
		}
	}
	if targetID == "" && ws != "" {
		targetType, targetID = "workspace", ws
	}
	return strings.TrimSuffix(resource, "s") + "." + verb, targetType, targetID
}
