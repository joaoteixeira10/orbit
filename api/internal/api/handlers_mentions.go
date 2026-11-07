package api

import (
	"net/http"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/inbox"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Mentions in pages: @username tells that member (once: a save that keeps a
// mention doesn't tell them again); a link to another page,
// [Title](/w/<ws>/p/<id>), makes this page one of its backlinks.

// notifyPageMentions tells the members p newly names (not in before, not
// the writer), with the line they're named on.
func (h *Handlers) notifyPageMentions(c *gin.Context, p models.Page, before string) {
	named := h.mentions(p.WorkspaceID, p.Content)
	if len(named) == 0 {
		return
	}
	was := map[string]bool{}
	for _, id := range h.mentions(p.WorkspaceID, before) {
		was[id] = true
	}
	uid := auth.UserID(c)
	var users []models.User
	h.db.Select("id", "username").Where("id IN ?", named).Find(&users)
	ws := h.workspaceOf(p.WorkspaceID)
	title := p.Title
	if title == "" {
		title = "Untitled"
	}
	for _, u := range users {
		if u.ID == uid || was[u.ID] || !h.gate(u.ID, p.WorkspaceID).sees(p.ID) {
			continue
		}
		params := map[string]any{"actor": h.actorName(uid), "title": title, "ws": ws.Name, "excerpt": excerpt(mentionLine(p.Content, u.Username))}
		h.inbox.Notify(inbox.Message{UserID: u.ID, Kind: inbox.Comment, Verb: "mentioned", WorkspaceID: ws.ID, ActorID: uid, Params: params, Link: pageLink(p)})
	}
}

// mentionLine is the line of md that names @username, without its Markdown
// list and heading marks.
func mentionLine(md, username string) string {
	at := "@" + strings.ToLower(username)
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(strings.ToLower(line), at) {
			return strings.TrimLeft(strings.TrimSpace(line), "#>-*+[] ")
		}
	}
	return ""
}

type backlinkTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	ListID string `json:"list_id"`
	Done   bool   `json:"done"`
}

type backlinksResponse struct {
	Pages []pageSummary  `json:"pages"`
	Tasks []backlinkTask `json:"tasks"`
}

// pageBacklinks lists what links to a page: the workspace's other pages,
// and tasks whose notes do.
//
//	@Summary	What links to a page
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Page id"
//	@Success	200	{object}	backlinksResponse
//	@Router		/pages/{id}/backlinks [get]
func (h *Handlers) pageBacklinks(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSViewer)
	if !ok {
		return
	}
	like := "%/p/" + p.ID + "%"
	out := backlinksResponse{Pages: []pageSummary{}, Tasks: []backlinkTask{}}
	var pages []models.Page
	h.db.Omit("content_text").Where("workspace_id = ? AND trashed_at IS NULL AND id <> ? AND content LIKE ?", p.WorkspaceID, p.ID, like).
		Order("title").Limit(200).Find(&pages)
	g := h.gate(auth.UserID(c), p.WorkspaceID)
	for _, x := range pages {
		if g.sees(x.ID) {
			out.Pages = append(out.Pages, summarize(x, nil))
		}
	}
	var tasks []models.Task
	h.db.Where("workspace_id = ? AND notes LIKE ?", p.WorkspaceID, like).Order("completed_at IS NOT NULL, title").Limit(200).Find(&tasks)
	for _, t := range tasks {
		out.Tasks = append(out.Tasks, backlinkTask{ID: t.ID, Title: t.Title, ListID: t.ListID, Done: t.CompletedAt != nil})
	}
	c.JSON(http.StatusOK, out)
}
