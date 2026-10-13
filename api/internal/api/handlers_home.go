package api

import (
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// homeResponse is the overview: what's late, due today and coming up, the
// next events and recently edited pages, across workspaces (or one).
type homeResponse struct {
	Overdue  []taskResponse `json:"overdue"`
	Today    []taskResponse `json:"today"`
	Upcoming []taskResponse `json:"upcoming"`
	Events   []occurrence   `json:"events"`
	Pages    []pageSummary  `json:"pages"`
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
	resp := homeResponse{Overdue: []taskResponse{}, Today: []taskResponse{}, Upcoming: []taskResponse{}, Events: []occurrence{}, Pages: []pageSummary{}}
	if len(wsIDs) == 0 {
		c.JSON(http.StatusOK, resp)
		return
	}
	uid := auth.UserID(c)
	d, _ := time.Parse(planner.DateLayout, today)
	weekEnd := d.AddDate(0, 0, 7).Format(planner.DateLayout)
	// My tasks: assigned to me, or unassigned ones I can see.
	mine := h.db.Where("workspace_id IN ? AND completed_at IS NULL AND parent_id IS NULL AND (assignee_id IS NULL OR assignee_id = ?)", wsIDs, uid)
	var overdue, todays, upcoming []models.Task
	mine.Session(&gorm.Session{}).Where("due < ?", today).Order("due, due_time").Limit(50).Find(&overdue)
	mine.Session(&gorm.Session{}).Where("due = ?", today).Order("due_time, priority DESC, rank").Limit(100).Find(&todays)
	mine.Session(&gorm.Session{}).Where("due > ? AND due <= ?", today, weekEnd).Order("due, due_time").Limit(100).Find(&upcoming)
	resp.Overdue, resp.Today, resp.Upcoming = toTaskResponses(overdue), toTaskResponses(todays), toTaskResponses(upcoming)
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
	c.JSON(http.StatusOK, resp)
}

// searchHit is one search result.
type searchHit struct {
	Kind        string     `json:"kind"` // page | task | event
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

// search finds pages (title and text), tasks (title and notes) and events
// (title) across my workspaces, or one (?ws=).
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
	if len(wsIDs) == 0 {
		c.JSON(http.StatusOK, out)
		return
	}
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
	var events []models.Event
	h.db.Where(`workspace_id IN ? AND LOWER(title) LIKE ? ESCAPE '\'`, wsIDs, pat).Order("starts_at DESC").Limit(8).Find(&events)
	for _, e := range events {
		s := e.StartsAt
		out = append(out, searchHit{Kind: "event", ID: e.ID, WorkspaceID: e.WorkspaceID, Title: e.Title, Start: &s})
	}
	c.JSON(http.StatusOK, out)
}
