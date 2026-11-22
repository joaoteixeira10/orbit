package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// A workspace's pages copied into a git repository as Markdown files (one
// way; gitsync/pages.go). Its owners set it up, writing as their own
// account on the server (Settings → Git).

type pagesRepoAccount struct {
	BaseURL string `json:"base_url"`
	Kind    string `json:"kind"`
	Login   string `json:"login"`
}

type pagesRepoResponse struct {
	Repo     *models.PagesRepo  `json:"repo"`
	Accounts []pagesRepoAccount `json:"accounts"` // the caller's, to write with
}

func (h *Handlers) pagesRepoOwner(c *gin.Context) (models.Workspace, bool) {
	if !h.gitEnabled(c) {
		return models.Workspace{}, false
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	return ws, ok
}

func (h *Handlers) pagesRepoOut(c *gin.Context, wsID string) pagesRepoResponse {
	out := pagesRepoResponse{Accounts: []pagesRepoAccount{}}
	var r models.PagesRepo
	if h.db.First(&r, "workspace_id = ?", wsID).Error == nil {
		out.Repo = &r
	}
	var accs []models.GitAccount
	h.db.Where("user_id = ?", auth.UserID(c)).Order("base_url").Find(&accs)
	for _, a := range accs {
		out.Accounts = append(out.Accounts, pagesRepoAccount{BaseURL: a.BaseURL, Kind: a.Kind, Login: a.Login})
	}
	return out
}

// getPagesRepo is where the workspace's pages are copied to, if anywhere
// (owners).
//
//	@Summary	Where a workspace's pages are copied (git)
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	pagesRepoResponse
//	@Router		/workspaces/{ws}/pages-repo [get]
func (h *Handlers) getPagesRepo(c *gin.Context) {
	ws, ok := h.pagesRepoOwner(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.pagesRepoOut(c, ws.ID))
}

type pagesRepoRequest struct {
	Host   string `json:"host" binding:"required,max=255"`
	Repo   string `json:"repo" binding:"required,max=255"` // owner/name
	Branch string `json:"branch" binding:"max=255"`
	Folder string `json:"folder" binding:"max=255"`
	Auto   bool   `json:"auto"`
}

// putPagesRepo sets where the workspace's pages are copied (owners; as
// their own account on that server).
//
//	@Summary	Copy a workspace's pages into a git repository
//	@Tags		git
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		body	body		pagesRepoRequest	true	"The repository"
//	@Success	200		{object}	pagesRepoResponse
//	@Router		/workspaces/{ws}/pages-repo [put]
func (h *Handlers) putPagesRepo(c *gin.Context) {
	ws, ok := h.pagesRepoOwner(c)
	if !ok {
		return
	}
	var req pagesRepoRequest
	if !bindJSON(c, &req) {
		return
	}
	owner, name, ok := strings.Cut(strings.Trim(strings.TrimSpace(req.Repo), "/"), "/")
	if !ok || owner == "" || name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the repository as owner/name"})
		return
	}
	name = strings.TrimSuffix(name, ".git")
	if _, err := h.git.ClientFor(auth.UserID(c), req.Host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	folder := strings.Trim(strings.TrimSpace(strings.ReplaceAll(req.Folder, "\\", "/")), "/")
	if strings.Contains("/"+folder+"/", "/../") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the folder can't go up (..)"})
		return
	}
	var r models.PagesRepo
	if h.db.First(&r, "workspace_id = ?", ws.ID).Error != nil {
		r = models.PagesRepo{WorkspaceID: ws.ID}
	}
	// Another place: what the copy wrote is counted afresh (the old place's
	// files stay as they are).
	if r.Host != req.Host || r.Owner != owner || r.Repo != name || r.Branch != branch || r.Folder != folder {
		r.Files, r.Commit, r.CopiedAt, r.LastError = "", "", nil, ""
	}
	r.UserID, r.Host, r.Owner, r.Repo, r.Branch, r.Folder, r.Auto = auth.UserID(c), req.Host, owner, name, branch, folder, req.Auto
	if err := h.db.Save(&r).Error; err != nil {
		h.internalError(c, err, "could not save it")
		return
	}
	h.recordCtx(c, "workspace.pages_repo", "workspace", ws.ID, gin.H{"host": r.Host, "repo": owner + "/" + name, "branch": branch, "folder": folder})
	c.JSON(http.StatusOK, h.pagesRepoOut(c, ws.ID))
}

// deletePagesRepo stops copying the pages (what was copied stays there).
//
//	@Summary	Stop copying a workspace's pages to git
//	@Tags		git
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	204
//	@Router		/workspaces/{ws}/pages-repo [delete]
func (h *Handlers) deletePagesRepo(c *gin.Context) {
	ws, ok := h.pagesRepoOwner(c)
	if !ok {
		return
	}
	h.db.Where("workspace_id = ?", ws.ID).Delete(&models.PagesRepo{})
	h.recordCtx(c, "workspace.pages_repo_delete", "workspace", ws.ID, nil)
	c.Status(http.StatusNoContent)
}

// copyPagesRepo copies the pages now (owners).
//
//	@Summary	Copy a workspace's pages to git now
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	pagesRepoResponse
//	@Failure	502	{object}	map[string]string
//	@Router		/workspaces/{ws}/pages-repo/copy [post]
func (h *Handlers) copyPagesRepo(c *gin.Context) {
	ws, ok := h.pagesRepoOwner(c)
	if !ok {
		return
	}
	var r models.PagesRepo
	if h.db.First(&r, "workspace_id = ?", ws.ID).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "this workspace's pages aren't copied anywhere"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	if err := h.git.CopyPages(ctx, &r); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "copying the pages: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.pagesRepoOut(c, ws.ID))
}
