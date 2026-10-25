package api

import (
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/search"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// homeResponse is the overview: what's late, due today and coming up, the
// next events, recently edited pages and pinned notes, across workspaces (or
// one).
type homeResponse struct {
	Overdue  []taskResponse `json:"overdue"`
	Today    []taskResponse `json:"today"`
	Upcoming []taskResponse `json:"upcoming"`
	Events   []occurrence   `json:"events"`
	Pages    []pageSummary  `json:"pages"`
	Notes    []noteResponse `json:"notes"` // pinned quick notes
	// Mine is the caller's latest personal notes, not pinned (Today only).
	Mine []noteResponse `json:"mine"`
	// Exams: the next ones (within 60 days), soonest first, to count down to.
	Exams []taskResponse `json:"exams"`
}

// home returns the overview. The browser sends its local date (?today=) and
// the instants of its local day and the following week (?from=&to=), so
// "today" is the viewer's today.
//
//	@Summary	Overview (Home)
//	@Tags		home
//	@Produce	json
//	@Security	BearerAuth
//	@Param		today	query		string	true	"Local date (YYYY-MM-DD)"
//	@Param		from	query		string	true	"RFC 3339: start of the local day"
//	@Param		to		query		string	true	"RFC 3339: end of the window (a week later)"
//	@Param		ws		query		string	false	"Only this workspace"
//	@Success	200		{object}	homeResponse
//	@Router		/home [get]
func (h *Handlers) home(c *gin.Context) {
	today := c.Query("today")
	if !planner.ValidDate(today) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "today must be YYYY-MM-DD"})
		return
	}
	from, to, _, _, ok := parseRange(c)
	if !ok {
		return
	}
	wsIDs := h.myWorkspaceIDs(c, true)
	if ws := c.Query("ws"); ws != "" {
		if _, _, ok := h.wsAccess(c, ws, models.WSViewer); !ok {
			return
		}
		wsIDs = []string{ws}
	}
	resp := homeResponse{Overdue: []taskResponse{}, Today: []taskResponse{}, Upcoming: []taskResponse{}, Events: []occurrence{}, Pages: []pageSummary{}, Notes: []noteResponse{}, Mine: []noteResponse{}, Exams: []taskResponse{}}
	uid := auth.UserID(c)
	// Today (all workspaces) also shows the caller's own notes.
	personal := c.Query("ws") == ""
	if personal {
		var mine []models.Note
		h.db.Where("workspace_id = '' AND owner_id = ? AND NOT pinned", uid).Order("updated_at DESC").Limit(9).Find(&mine)
		for _, n := range mine {
			resp.Mine = append(resp.Mine, toNoteResponse(n))
		}
	}
	if len(wsIDs) == 0 {
		var pinned []models.Note
		h.db.Where("workspace_id = '' AND owner_id = ? AND pinned", uid).Order("updated_at DESC").Limit(12).Find(&pinned)
		for _, n := range pinned {
			resp.Notes = append(resp.Notes, toNoteResponse(n))
		}
		resp.Notes, resp.Mine = h.withAttachments(resp.Notes), h.withAttachments(resp.Mine)
		c.JSON(http.StatusOK, resp)
		return
	}
	d, _ := time.Parse(planner.DateLayout, today)
	weekEnd := d.AddDate(0, 0, 7).Format(planner.DateLayout)
	// My tasks: assigned to me, or unassigned ones I can see.
	mine := h.db.Where("workspace_id IN ? AND completed_at IS NULL AND parent_id IS NULL AND "+models.MineOrUnassigned, wsIDs, models.AssigneeArg(uid))
	var overdue, todays, upcoming []models.Task
	mine.Session(&gorm.Session{}).Where("due < ?", today).Order("due, due_time").Limit(50).Find(&overdue)
	mine.Session(&gorm.Session{}).Where("due = ?", today).Order("due_time, priority DESC, rank").Limit(100).Find(&todays)
	mine.Session(&gorm.Session{}).Where("due > ? AND due <= ?", today, weekEnd).Order("due, due_time").Limit(100).Find(&upcoming)
	resp.Overdue, resp.Today, resp.Upcoming = h.counted(toTaskResponses(overdue)), h.counted(toTaskResponses(todays)), h.counted(toTaskResponses(upcoming))
	var exams []models.Task
	h.db.Where("workspace_id IN ? AND completed_at IS NULL AND kind = ? AND due >= ? AND due <= ?", wsIDs, "exam", today, d.AddDate(0, 0, 60).Format(planner.DateLayout)).
		Order("due, due_time").Limit(12).Find(&exams)
	resp.Exams = toTaskResponses(exams)
	resp.Events = h.calendarFor(wsIDs, from, to, today, today).Events
	if len(resp.Events) > 60 {
		resp.Events = resp.Events[:60]
	}
	var pages []models.Page
	h.db.Omit("content", "content_text").Where("workspace_id IN ? AND trashed_at IS NULL", wsIDs).Order("updated_at DESC").Limit(8).Find(&pages)
	fav := h.favoriteSet(uid)
	for _, p := range pages {
		resp.Pages = append(resp.Pages, summarize(p, fav))
	}
	var notes []models.Note
	q := h.db.Where("workspace_id IN ? AND pinned", wsIDs)
	if personal {
		q = h.db.Where("pinned AND (workspace_id IN ? OR (workspace_id = '' AND owner_id = ?))", wsIDs, uid)
	}
	q.Order("updated_at DESC").Limit(12).Find(&notes)
	for _, n := range notes {
		resp.Notes = append(resp.Notes, toNoteResponse(n))
	}
	resp.Notes, resp.Mine = h.withAttachments(resp.Notes), h.withAttachments(resp.Mine)
	c.JSON(http.StatusOK, resp)
}

// searchHit is one search result.
type searchHit struct {
	Kind        string     `json:"kind"` // page | task | event | note | drawing | bookmark | journal (ID: its date)
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Title       string     `json:"title"`
	Icon        string     `json:"icon,omitempty"`
	Snippet     string     `json:"snippet,omitempty"`
	Due         *string    `json:"due,omitempty"`
	Done        bool       `json:"done,omitempty"`
	Start       *time.Time `json:"start,omitempty"`
	ListID      string     `json:"list_id,omitempty"`
}

// likeEscape escapes LIKE wildcards in user input (ESCAPE '\').
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

// search finds pages (title and text), tasks (title and notes), quick notes
// (title, text and checklist) and events (title) across my workspaces, or
// one (?ws=).
//
//	@Summary	Search
//	@Tags		home
//	@Produce	json
//	@Security	BearerAuth
//	@Param		q	query	string	true	"Text to find (2 characters or more)"
//	@Param		ws	query	string	false	"Only this workspace"
//	@Success	200	{array}	searchHit
//	@Router		/search [get]
func (h *Handlers) search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	out := []searchHit{}
	if len([]rune(q)) < 2 {
		c.JSON(http.StatusOK, out)
		return
	}
	if len(q) > 100 {
		q = q[:100]
	}
	wsIDs := h.myWorkspaceIDs(c, false)
	if ws := c.Query("ws"); ws != "" {
		if _, _, ok := h.wsAccess(c, ws, models.WSViewer); !ok {
			return
		}
		wsIDs = []string{ws}
	}
	if search.Ready() {
		c.JSON(http.StatusOK, h.searchIndex(c, q, wsIDs))
		return
	}
	// No workspaces: "IN ?" matches nothing, and the caller's own notes still
	// count.
	pat := "%" + strings.ToLower(likeEscape(q)) + "%"
	var pages []models.Page
	h.db.Select("id", "workspace_id", "title", "icon", "content_text").
		Where(`workspace_id IN ? AND trashed_at IS NULL AND (LOWER(title) LIKE ? ESCAPE '\' OR LOWER(content_text) LIKE ? ESCAPE '\')`, wsIDs, pat, pat).
		Order("updated_at DESC").Limit(12).Find(&pages)
	for _, p := range pages {
		out = append(out, searchHit{Kind: "page", ID: p.ID, WorkspaceID: p.WorkspaceID, Title: p.Title, Icon: p.Icon, Snippet: planner.Snippet(p.ContentText, q, 120)})
	}
	var tasks []models.Task
	h.db.Where(`workspace_id IN ? AND (LOWER(title) LIKE ? ESCAPE '\' OR LOWER(notes) LIKE ? ESCAPE '\')`, wsIDs, pat, pat).
		Order("completed_at IS NOT NULL, updated_at DESC").Limit(12).Find(&tasks)
	for _, t := range tasks {
		out = append(out, searchHit{Kind: "task", ID: t.ID, WorkspaceID: t.WorkspaceID, Title: t.Title, Snippet: planner.Snippet(t.Notes, q, 100), Due: t.Due, Done: t.CompletedAt != nil, ListID: t.ListID})
	}
	var notes []models.Note
	scope := h.db.Where("workspace_id IN ?", wsIDs)
	if c.Query("ws") == "" {
		// And the caller's own notes.
		scope = h.db.Where("workspace_id IN ? OR (workspace_id = '' AND owner_id = ?)", wsIDs, auth.UserID(c))
	}
	h.db.Where(scope).Where(`LOWER(title) LIKE ? ESCAPE '\' OR LOWER(plain_text) LIKE ? ESCAPE '\'`, pat, pat).
		Order("updated_at DESC").Limit(8).Find(&notes)
	for _, n := range notes {
		out = append(out, searchHit{Kind: "note", ID: n.ID, WorkspaceID: n.WorkspaceID, Title: n.Title, Snippet: planner.Snippet(n.PlainText, q, 100)})
	}
	var events []models.Event
	h.db.Where(`workspace_id IN ? AND LOWER(title) LIKE ? ESCAPE '\'`, wsIDs, pat).Order("starts_at DESC").Limit(8).Find(&events)
	for _, e := range events {
		s := e.StartsAt
		out = append(out, searchHit{Kind: "event", ID: e.ID, WorkspaceID: e.WorkspaceID, Title: e.Title, Start: &s})
	}
	c.JSON(http.StatusOK, out)
}

// searchIndex answers a search from the full-text index: the best matches
// first, a few of each kind, each with what the list shows of it.
func (h *Handlers) searchIndex(c *gin.Context, q string, wsIDs []string) []searchHit {
	out := []searchHit{}
	uid := auth.UserID(c)
	notesScope := uid
	if c.Query("ws") != "" {
		notesScope = "" // one workspace: not the caller's own notes
	}
	hits, err := search.Find(h.db, q, wsIDs, notesScope, 60)
	if err != nil {
		h.log.Warn("search failed", zap.Error(err))
		return out
	}
	ids := map[string][]string{}
	for _, x := range hits {
		ids[x.Kind] = append(ids[x.Kind], x.Ref)
	}
	pages := map[string]models.Page{}
	tasks := map[string]models.Task{}
	notes := map[string]models.Note{}
	events := map[string]models.Event{}
	if l := ids["page"]; len(l) > 0 {
		var rows []models.Page
		h.db.Select("id", "workspace_id", "title", "icon", "content_text", "trashed_at").Where("id IN ?", l).Find(&rows)
		for _, r := range rows {
			pages[r.ID] = r
		}
	}
	if l := append(ids["task"], ids["comment"]...); len(l) > 0 {
		var rows []models.Task
		h.db.Where("id IN ?", l).Find(&rows)
		for _, r := range rows {
			tasks[r.ID] = r
		}
	}
	if l := ids["note"]; len(l) > 0 {
		var rows []models.Note
		h.db.Where("id IN ?", l).Find(&rows)
		for _, r := range rows {
			notes[r.ID] = r
		}
	}
	drawings := map[string]models.Drawing{}
	if l := ids["drawing"]; len(l) > 0 {
		var rows []models.Drawing
		h.db.Select("id", "workspace_id", "note_id", "title", "text").Where("id IN ?", l).Find(&rows)
		var noteIDs []string
		for _, r := range rows {
			drawings[r.ID] = r
			if r.NoteID != "" {
				noteIDs = append(noteIDs, r.NoteID)
			}
		}
		if len(noteIDs) > 0 {
			var ns []models.Note
			h.db.Where("id IN ?", noteIDs).Find(&ns)
			for _, r := range ns {
				notes[r.ID] = r
			}
		}
	}
	if l := ids["event"]; len(l) > 0 {
		var rows []models.Event
		h.db.Where("id IN ?", l).Find(&rows)
		for _, r := range rows {
			events[r.ID] = r
		}
	}
	journal := map[string]models.JournalEntry{}
	if l := ids["journal"]; len(l) > 0 {
		var rows []models.JournalEntry
		h.db.Select("id", "workspace_id", "owner_id", "date", "content_text").Where("id IN ? AND owner_id = ?", l, uid).Find(&rows)
		for _, r := range rows {
			journal[r.ID] = r
		}
	}
	bookmarks := map[string]models.Bookmark{}
	if l := ids["bookmark"]; len(l) > 0 {
		var rows []models.Bookmark
		h.db.Select("id", "workspace_id", "url", "title", "description").Where("id IN ?", l).Find(&rows)
		for _, r := range rows {
			bookmarks[r.ID] = r
		}
	}
	sheets := map[string]models.Sheet{}
	if l := ids["sheet"]; len(l) > 0 {
		var rows []models.Sheet
		h.db.Select("id", "workspace_id", "title", "text").Where("id IN ?", l).Find(&rows)
		for _, r := range rows {
			sheets[r.ID] = r
		}
	}
	diagrams := map[string]models.Diagram{}
	if l := ids["diagram"]; len(l) > 0 {
		var rows []models.Diagram
		h.db.Select("id", "workspace_id", "title", "text").Where("id IN ?", l).Find(&rows)
		for _, r := range rows {
			diagrams[r.ID] = r
		}
	}
	caps := map[string]int{"page": 12, "task": 12, "note": 8, "event": 8, "drawing": 8, "journal": 8, "bookmark": 8, "sheet": 8, "diagram": 8}
	seen := map[string]bool{}
	for _, x := range hits {
		kind := x.Kind
		if kind == "comment" {
			kind = "task" // a comment opens its task
		}
		if d, ok := drawings[x.Ref]; ok && kind == "drawing" && d.NoteID != "" {
			kind, x.Ref = "note", d.NoteID // a note's drawing opens the note
		}
		if seen[kind+x.Ref] || caps[kind] == 0 {
			continue
		}
		switch kind {
		case "page":
			p, ok := pages[x.Ref]
			if !ok || p.TrashedAt != nil {
				continue
			}
			out = append(out, searchHit{Kind: "page", ID: p.ID, WorkspaceID: p.WorkspaceID, Title: p.Title, Icon: p.Icon, Snippet: planner.Snippet(p.ContentText, q, 120)})
		case "task":
			t, ok := tasks[x.Ref]
			if !ok {
				continue
			}
			out = append(out, searchHit{Kind: "task", ID: t.ID, WorkspaceID: t.WorkspaceID, Title: t.Title, Snippet: planner.Snippet(t.Notes, q, 100), Due: t.Due, Done: t.CompletedAt != nil, ListID: t.ListID})
		case "note":
			n, ok := notes[x.Ref]
			if !ok {
				continue
			}
			out = append(out, searchHit{Kind: "note", ID: n.ID, WorkspaceID: n.WorkspaceID, Title: n.Title, Snippet: planner.Snippet(n.PlainText, q, 100)})
		case "event":
			e, ok := events[x.Ref]
			if !ok {
				continue
			}
			s := e.StartsAt
			out = append(out, searchHit{Kind: "event", ID: e.ID, WorkspaceID: e.WorkspaceID, Title: e.Title, Start: &s})
		case "bookmark":
			b, ok := bookmarks[x.Ref]
			if !ok {
				continue
			}
			title := b.Title
			if title == "" {
				title = b.URL
			}
			out = append(out, searchHit{Kind: "bookmark", ID: b.ID, WorkspaceID: b.WorkspaceID, Title: title, Snippet: b.URL})
		case "journal":
			e, ok := journal[x.Ref]
			if !ok || (c.Query("ws") != "" && e.WorkspaceID != c.Query("ws")) {
				continue
			}
			out = append(out, searchHit{Kind: "journal", ID: e.Date, WorkspaceID: e.WorkspaceID, Title: e.Date, Snippet: planner.Snippet(e.ContentText, q, 100)})
		case "sheet":
			s, ok := sheets[x.Ref]
			if !ok {
				continue
			}
			out = append(out, searchHit{Kind: "sheet", ID: s.ID, WorkspaceID: s.WorkspaceID, Title: s.Title, Snippet: planner.Snippet(s.Text, q, 100)})
		case "diagram":
			d, ok := diagrams[x.Ref]
			if !ok {
				continue
			}
			out = append(out, searchHit{Kind: "diagram", ID: d.ID, WorkspaceID: d.WorkspaceID, Title: d.Title, Snippet: planner.Snippet(d.Text, q, 100)})
		case "drawing":
			d, ok := drawings[x.Ref]
			if !ok {
				continue
			}
			out = append(out, searchHit{Kind: "drawing", ID: d.ID, WorkspaceID: d.WorkspaceID, Title: d.Title, Snippet: planner.Snippet(d.Text, q, 100)})
		}
		seen[kind+x.Ref] = true
		caps[kind]--
	}
	return out
}
