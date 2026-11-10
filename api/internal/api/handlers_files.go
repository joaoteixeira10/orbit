package api

import (
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
)

// Files: a workspace's files in its folders — files of their own and the
// attachments of its pages, tasks and notes (each where what it's on is) —
// with their versions and a trash. See internal/uploads.

type fileResponse struct {
	uploadResponse
	uploads.Place
	Versions  int    `json:"versions"`             // how many it has (1: just this)
	EmptiesAt string `json:"empties_at,omitempty"` // in the trash: when it goes for good
}

// filesOf lists a workspace's files ("" with the caller: their personal
// ones) — the newest versions, in the trash or not — that the caller may
// see (an attachment of a page they can't see isn't listed).
func (h *Handlers) filesOf(c *gin.Context, wsID string, trash bool) []fileResponse {
	q := h.db.Where("replaced_at IS NULL")
	if wsID == "" {
		q = q.Where("workspace_id = '' AND owner_id = ? AND target <> ?", auth.UserID(c), uploads.Chat)
	} else {
		q = q.Where("workspace_id = ?", wsID)
	}
	if trash {
		q = q.Where("trashed_at IS NOT NULL")
	} else {
		q = q.Where("trashed_at IS NULL")
	}
	var rows []models.Upload
	q.Order("created_at DESC").Find(&rows)
	// Attachments of pages the caller can't see stay hidden.
	pageIDs := []string{}
	for _, u := range rows {
		if u.Target == uploads.Page {
			pageIDs = append(pageIDs, u.TargetID)
		}
	}
	hidden := map[string]bool{}
	if len(pageIDs) > 0 {
		var pages []models.Page
		h.db.Where("id IN ?", pageIDs).Find(&pages)
		for _, p := range pages {
			hidden[p.ID] = !h.pageVisible(c, p)
		}
	}
	lineages := []string{}
	for _, u := range rows {
		lineages = append(lineages, uploads.LineageOf(u))
	}
	type count struct {
		Lineage string
		N       int
	}
	var counts []count
	if len(lineages) > 0 {
		h.db.Model(&models.Upload{}).Select("lineage, COUNT(*) AS n").Where("lineage IN ?", lineages).Group("lineage").Scan(&counts)
	}
	versions := map[string]int{}
	for _, x := range counts {
		versions[x.Lineage] = x.N
	}
	places := uploads.Places(h.db, rows)
	out := make([]fileResponse, 0, len(rows))
	for _, u := range rows {
		if u.Target == uploads.Page && hidden[u.TargetID] {
			continue
		}
		f := fileResponse{uploadResponse: toUploadResponse(u), Place: places[u.ID], Versions: max(versions[uploads.LineageOf(u)], 1)}
		if u.TrashedAt != nil {
			f.EmptiesAt = stamp(u.TrashedAt.Add(uploads.TrashDays * 24 * time.Hour))
		}
		out = append(out, f)
	}
	return out
}

// listFiles lists a workspace's files: its own and its pages', tasks' and
// notes' (each with where it shows and what it's on); ?trash=1 those in the
// trash (editors).
//
//	@Summary	A workspace's files
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path	string	true	"Workspace id"
//	@Param		trash	query	bool	false	"The trash"
//	@Success	200		{array}	fileResponse
//	@Router		/workspaces/{ws}/files [get]
func (h *Handlers) listFiles(c *gin.Context) {
	trash := c.Query("trash") == "1"
	need := models.WSViewer
	if trash {
		need = models.WSEditor
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), need)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.filesOf(c, ws.ID, trash))
}

type fileUsage struct {
	Used  int64 `json:"used"`  // bytes this workspace's files take (each set of bytes once)
	Files int64 `json:"files"` // how many (the newest versions, not in the trash)
	Quota int64 `json:"quota"` // the instance's limit (0 none)
	All   int64 `json:"all"`   // what the instance's files take
}

// filesUsage says how much space a workspace's files take.
//
//	@Summary	A workspace's space used
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	fileUsage
//	@Router		/workspaces/{ws}/files/usage [get]
func (h *Handlers) filesUsage(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var n int64
	h.db.Model(&models.Upload{}).Where("workspace_id = ? AND "+uploads.Shown, ws.ID).Count(&n)
	c.JSON(http.StatusOK, fileUsage{Used: uploads.Used(h.db, "workspace_id = ?", ws.ID), Files: n, Quota: h.cfg.UploadQuotaBytes, All: uploads.Used(h.db, "")})
}

type fileRequest struct {
	Name     *string `json:"name" binding:"omitempty,max=255"`
	FolderID *string `json:"folder_id"` // a file of its own only ("" the top)
}

// updateFile renames a file (every version) or moves a file of its own to
// another folder (editors). An attachment shows where what it's on is.
//
//	@Summary	Rename or move a file
//	@Tags		files
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"File id"
//	@Param		body	body		fileRequest	true	"Name, folder"
//	@Success	200		{object}	uploadResponse
//	@Router		/files/{id} [put]
func (h *Handlers) updateFile(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req fileRequest
	if !bindJSON(c, &req) {
		return
	}
	up := map[string]any{}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "give the file a name"})
			return
		}
		up["name"] = uploads.CleanName(n)
	}
	if req.FolderID != nil {
		if u.Target != uploads.File {
			c.JSON(http.StatusBadRequest, gin.H{"error": "an attachment is where what it's on is; move that"})
			return
		}
		if !h.fileFolderOK(c, u.WorkspaceID, *req.FolderID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no such folder here"})
			return
		}
		up["folder_id"] = *req.FolderID
	}
	if len(up) > 0 {
		if err := h.db.Model(&models.Upload{}).Where("lineage = ?", uploads.LineageOf(u)).Updates(up).Error; err != nil {
			h.internalError(c, err, "could not change the file")
			return
		}
	}
	h.db.First(&u, "id = ?", u.ID)
	h.publishFiles(c, u)
	c.JSON(http.StatusOK, toUploadResponse(u))
}

type versionResponse struct {
	uploadResponse
	Current bool `json:"current"`
}

// fileVersions lists a file's versions, newest first (each one's url opens
// that version).
//
//	@Summary	A file's versions
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"File id"
//	@Success	200	{array}	versionResponse
//	@Router		/files/{id}/versions [get]
func (h *Handlers) fileVersions(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSViewer)
	if !ok {
		return
	}
	vs := uploads.Versions(h.db, u)
	out := make([]versionResponse, 0, len(vs))
	for _, v := range vs {
		r := toUploadResponse(v)
		r.URL += "?exact=1"
		out = append(out, versionResponse{uploadResponse: r, Current: v.ReplacedAt == nil})
	}
	c.JSON(http.StatusOK, out)
}

// fileCurrent makes an older version the newest again (editors).
//
//	@Summary	Restore a version
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"The version's id"
//	@Success	200	{object}	uploadResponse
//	@Router		/files/{id}/current [post]
func (h *Handlers) fileCurrent(c *gin.Context) {
	old, ok := h.uploadAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if old.TrashedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "take it out of the trash first"})
		return
	}
	if old.ReplacedAt == nil {
		c.JSON(http.StatusOK, toUploadResponse(old))
		return
	}
	u, err := uploads.MakeCurrent(h.db, old, auth.UserID(c))
	if err != nil {
		h.internalError(c, err, "could not restore the version")
		return
	}
	h.publishFiles(c, u)
	c.JSON(http.StatusOK, toUploadResponse(u))
}

// fileUntrash takes a file out of the trash (editors); one whose page,
// task or note is gone comes back as a file of its own, where it was.
//
//	@Summary	Take a file out of the trash
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"File id"
//	@Success	200	{object}	uploadResponse
//	@Router		/files/{id}/restore [post]
func (h *Handlers) fileUntrash(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if u.TrashedAt == nil {
		c.JSON(http.StatusOK, toUploadResponse(u))
		return
	}
	if u.Target == uploads.File && !h.fileFolderOK(c, u.WorkspaceID, u.FolderID) {
		u.FolderID = "" // its folder is gone: the top
		h.db.Model(&models.Upload{}).Where("lineage = ?", uploads.LineageOf(u)).Update("folder_id", "")
	}
	if err := uploads.Untrash(h.db, u); err != nil {
		h.internalError(c, err, "could not restore the file")
		return
	}
	h.db.First(&u, "id = ?", u.ID)
	h.publishFiles(c, u)
	c.JSON(http.StatusOK, toUploadResponse(u))
}

// publishFiles tells whoever shows a file's place that it changed.
func (h *Handlers) publishFiles(c *gin.Context, u models.Upload) {
	if u.WorkspaceID == "" {
		h.events.Publish(u.OwnerID, `{"type":"ws","ws":"me","what":"files"}`)
		return
	}
	h.publishWS(c, u.WorkspaceID, "files")
	h.publishWS(c, u.WorkspaceID, "uploads")
}

// getFile is one file as Files shows it: where it is, what it's on, how many
// versions (for the file viewer).
//
//	@Summary	A file
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"File id"
//	@Success	200	{object}	fileResponse
//	@Router		/files/{id} [get]
func (h *Handlers) getFile(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSViewer)
	if !ok {
		return
	}
	if u.Target == uploads.Page {
		var p models.Page
		if h.db.First(&p, "id = ?", u.TargetID).Error == nil && !h.pageVisible(c, p) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
	}
	f := fileResponse{uploadResponse: toUploadResponse(u), Place: uploads.Places(h.db, []models.Upload{u})[u.ID], Versions: len(uploads.Versions(h.db, u))}
	if u.TrashedAt != nil {
		f.EmptiesAt = stamp(u.TrashedAt.Add(uploads.TrashDays * 24 * time.Hour))
	}
	c.JSON(http.StatusOK, f)
}

type fileTextResponse struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Text string `json:"text"` // "" when it has no words to read (a picture, say)
}

// getFileText reads what a file says: a text-like one as text, a PDF from
// what the app read of it, a sheet or document as its lines.
//
//	@Summary	A file's text
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"File id"
//	@Success	200	{object}	fileTextResponse
//	@Router		/files/{id}/text [get]
func (h *Handlers) getFileText(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSViewer)
	if !ok {
		return
	}
	if u.Target == uploads.Page {
		var p models.Page
		if h.db.First(&p, "id = ?", u.TargetID).Error == nil && !h.pageVisible(c, p) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
	}
	c.JSON(http.StatusOK, fileTextResponse{Name: u.Name, Type: u.Type, Text: h.attachmentText(u)})
}

type fileTextRequest struct {
	Text string `json:"text" binding:"max=2000000"`
}

// setFileText keeps what a PDF says, read by the app (pdf.js), for search
// (editors). Other files' text the server reads itself.
//
//	@Summary	A PDF's text
//	@Tags		files
//	@Accept		json
//	@Security	BearerAuth
//	@Param		id		path	string			true	"File id"
//	@Param		body	body	fileTextRequest	true	"Its text"
//	@Success	204
//	@Router		/files/{id}/text [put]
func (h *Handlers) setFileText(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req fileTextRequest
	if !bindJSON(c, &req) {
		return
	}
	if u.Type != "application/pdf" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only a PDF's text is sent; the rest is read here"})
		return
	}
	if err := uploads.SetText(h.db, u.ID, req.Text); err != nil {
		h.internalError(c, err, "could not keep the text")
		return
	}
	c.Status(http.StatusNoContent)
}
