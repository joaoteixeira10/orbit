package api

import (
	"encoding/json"
	"net/http"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Page restrictions: a page — and everything under it — can be open to
// only some of its workspace's people (Restricted, AllowedUsers). It's
// narrower than the workspace, never wider: a page under a restricted one
// is seen by those both allow. To the others it isn't there (not in the
// tree, search, Home, links, the graph…). If none of the people a page
// allows are in the workspace any more, its owners see it (so nothing is
// lost for good).

func decodeIDs(s string) []string {
	var ids []string
	if s != "" {
		_ = json.Unmarshal([]byte(s), &ids)
	}
	return ids
}

// pageGate answers "may uid see this page?" for the pages of some
// workspaces, from their restricted pages (loaded once).
type pageGate struct {
	uid        string
	restricted map[string]models.Page // the restricted pages, by id
	parent     map[string]string      // every page's parent ("" at the top)
	members    map[string]map[string]bool
	owners     map[string]map[string]bool
	memo       map[string]bool
}

// gate loads what's needed to judge uid's view of the pages of wsIDs.
func (h *Handlers) gate(uid string, wsIDs ...string) *pageGate {
	g := &pageGate{uid: uid, restricted: map[string]models.Page{}, memo: map[string]bool{}}
	if len(wsIDs) == 0 {
		return g
	}
	var rs []models.Page
	h.db.Select("id", "workspace_id", "parent_id", "restricted", "allowed_users").Where("workspace_id IN ? AND restricted = ?", wsIDs, true).Find(&rs)
	if len(rs) == 0 {
		return g // nothing restricted: everyone sees everything
	}
	wsWith := map[string]bool{}
	for _, p := range rs {
		g.restricted[p.ID] = p
		wsWith[p.WorkspaceID] = true
	}
	ws := make([]string, 0, len(wsWith))
	for w := range wsWith {
		ws = append(ws, w)
	}
	type pr struct {
		ID       string
		ParentID *string
	}
	var all []pr
	h.db.Model(&models.Page{}).Select("id", "parent_id").Where("workspace_id IN ?", ws).Scan(&all)
	g.parent = make(map[string]string, len(all))
	for _, p := range all {
		if p.ParentID != nil {
			g.parent[p.ID] = *p.ParentID
		}
	}
	var ms []models.WorkspaceMember
	h.db.Select("workspace_id", "user_id", "role").Where("workspace_id IN ?", ws).Find(&ms)
	g.members, g.owners = map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, m := range ms {
		if g.members[m.WorkspaceID] == nil {
			g.members[m.WorkspaceID], g.owners[m.WorkspaceID] = map[string]bool{}, map[string]bool{}
		}
		g.members[m.WorkspaceID][m.UserID] = true
		if m.Role == models.WSOwner {
			g.owners[m.WorkspaceID][m.UserID] = true
		}
	}
	return g
}

// allows: does the restricted page p let uid in?
func (g *pageGate) allows(p models.Page) bool {
	anyone := false
	for _, id := range decodeIDs(p.AllowedUsers) {
		if g.members[p.WorkspaceID][id] {
			anyone = true
			if id == g.uid {
				return true
			}
		}
	}
	// Nobody it allows is left: its workspace's owners.
	return !anyone && g.owners[p.WorkspaceID][g.uid]
}

// sees: may uid see the page with this id (itself and every page above it)?
func (g *pageGate) sees(id string) bool {
	if len(g.restricted) == 0 {
		return true
	}
	if v, ok := g.memo[id]; ok {
		return v
	}
	ok := true
	seen := map[string]bool{}
	for cur := id; cur != "" && !seen[cur]; cur = g.parent[cur] {
		seen[cur] = true
		if p, r := g.restricted[cur]; r && !g.allows(p) {
			ok = false
			break
		}
	}
	g.memo[id] = ok
	return ok
}

// pageVisible: may the caller see page p?
func (h *Handlers) pageVisible(c *gin.Context, p models.Page) bool {
	return h.gate(auth.UserID(c), p.WorkspaceID).sees(p.ID)
}

// lockedChain: is the page restricted, itself or by a page above it?
func (g *pageGate) lockedChain(id string) bool {
	seen := map[string]bool{}
	for cur := id; cur != "" && !seen[cur]; cur = g.parent[cur] {
		seen[cur] = true
		if _, r := g.restricted[cur]; r {
			return true
		}
	}
	return false
}

type pageAccessRequest struct {
	Restricted bool     `json:"restricted"`
	UserIDs    []string `json:"user_ids" binding:"max=500"`
}

// setPageAccess makes a page (and what's under it) open to only some of the
// workspace's people, or to all of them again (editors who can see it).
// Whoever restricts it stays in.
//
//	@Summary	Who can see a page
//	@Tags		pages
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Page id"
//	@Param		body	body		pageAccessRequest	true	"Restricted, and to whom"
//	@Success	200		{object}	pageResponse
//	@Router		/pages/{id}/access [put]
func (h *Handlers) setPageAccess(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req pageAccessRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	allowed := ""
	if req.Restricted {
		members := map[string]bool{}
		for _, m := range h.wsMembers(p.WorkspaceID) {
			members[m.id] = true
		}
		ids := []string{uid}
		seen := map[string]bool{uid: true}
		for _, id := range req.UserIDs {
			if !members[id] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "only the workspace's people can be let in"})
				return
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		b, _ := json.Marshal(ids)
		allowed = string(b)
	}
	if err := h.db.Model(&models.Page{}).Where("id = ?", p.ID).Updates(map[string]any{"restricted": req.Restricted, "allowed_users": allowed}).Error; err != nil {
		h.internalError(c, err, "could not change who can see the page")
		return
	}
	h.recordCtx(c, "page.access", "page", p.ID, gin.H{"restricted": req.Restricted, "users": len(decodeIDs(allowed))})
	h.publishWS(c, p.WorkspaceID, "pages")
	p = reloadPage(h, p.ID)
	c.JSON(http.StatusOK, pageOut(p, false, true))
}
