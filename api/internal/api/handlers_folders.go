package api

import (
	"net/http"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Folders: a workspace's sheets, diagrams, whiteboards and pages in
// folders, which go inside folders. Deleting a folder deletes nothing in
// it: what it held moves up to where it was. Moving files doesn't change
// them (no new version), so someone editing one isn't interrupted. A folder
// joined to a task list is a project (one list per folder, a list in one
// folder at most).

const maxFolderDepth = 32

// folderIn reports whether id is a folder of the workspace ("" is the top).
func (h *Handlers) folderIn(wsID, id string) bool {
	if id == "" {
		return true
	}
	var n int64
	h.db.Model(&models.Folder{}).Where("id = ? AND workspace_id = ?", id, wsID).Count(&n)
	return n == 1
}

// loops reports whether putting folder id inside parent would put it inside
// itself (or go deeper than folders may).
func (h *Handlers) loops(wsID, id, parent string) bool {
	for depth := 0; parent != ""; depth++ {
		if parent == id || depth > maxFolderDepth {
			return true
		}
		var f models.Folder
		if h.db.Select("parent_id").First(&f, "id = ? AND workspace_id = ?", parent, wsID).Error != nil {
			return false
		}
		parent = f.ParentID
	}
	return false
}

// folderCounts is what is right inside a folder, of each kind.
type folderCounts struct {
	Sheets   int64 `json:"sheets"`
	Diagrams int64 `json:"diagrams"`
	Boards   int64 `json:"boards"`
	Pages    int64 `json:"pages"`
	Files    int64 `json:"files"` // files of its own and attachments shown here
	Folders  int64 `json:"folders"`
}

type folderResponse struct {
	models.Folder
	Counts folderCounts `json:"counts"`
}

// listFolders lists a workspace's folders (all of them; the app makes the
// tree), each with how many of each kind it holds.
//
//	@Summary	A workspace's folders
//	@Tags		folders
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	folderResponse
//	@Router		/workspaces/{ws}/folders [get]
func (h *Handlers) listFolders(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var folders []models.Folder
	h.db.Where("workspace_id = ?", ws.ID).Order("name").Find(&folders)
	type row struct {
		F string
		N int64
	}
	count := func(m any, col, extra string) map[string]int64 {
		var rows []row
		q := h.db.Model(m).Select(col+" AS f, COUNT(*) AS n").Where("workspace_id = ? AND "+col+" <> ''", ws.ID)
		if extra != "" {
			q = q.Where(extra)
		}
		q.Group(col).Scan(&rows)
		out := map[string]int64{}
		for _, r := range rows {
			out[r.F] = r.N
		}
		return out
	}
	sheets := count(&models.Sheet{}, "folder_id", "")
	diagrams := count(&models.Diagram{}, "folder_id", "")
	boards := count(&models.Drawing{}, "folder_id", "note_id = ''")
	pages := count(&models.Page{}, "folder_id", "trashed_at IS NULL")
	subs := count(&models.Folder{}, "parent_id", "")
	files := map[string]int64{}
	for _, x := range h.filesOf(c, ws.ID, false) {
		files[x.FolderID]++
	}
	out := make([]folderResponse, 0, len(folders))
	for _, f := range folders {
		out = append(out, folderResponse{Folder: f, Counts: folderCounts{Sheets: sheets[f.ID], Diagrams: diagrams[f.ID], Boards: boards[f.ID], Pages: pages[f.ID], Files: files[f.ID], Folders: subs[f.ID]}})
	}
	c.JSON(http.StatusOK, out)
}

type folderRequest struct {
	Name     *string `json:"name" binding:"omitempty,max=200"`
	ParentID *string `json:"parent_id"`
	Color    *string `json:"color"`
	// ListID joins a task list to it ("" none); a list in another folder
	// moves here.
	ListID *string `json:"list_id"`
}

// createFolder makes a folder (editors).
//
//	@Summary	New folder
//	@Tags		folders
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		folderRequest	true	"Name, where"
//	@Success	201		{object}	models.Folder
//	@Router		/workspaces/{ws}/folders [post]
func (h *Handlers) createFolder(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req folderRequest
	if !bindJSON(c, &req) {
		return
	}
	f := models.Folder{WorkspaceID: ws.ID, CreatedBy: auth.UserID(c)}
	if code, msg := h.applyFolder(&f, req); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	if f.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a folder needs a name"})
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := unlistElsewhere(tx, f); err != nil {
			return err
		}
		return tx.Create(&f).Error
	})
	if err != nil {
		h.internalError(c, err, "could not create the folder")
		return
	}
	h.publishWS(c, ws.ID, "files")
	c.JSON(http.StatusCreated, f)
}

func (h *Handlers) applyFolder(f *models.Folder, req folderRequest) (int, string) {
	if req.Name != nil {
		f.Name = strings.TrimSpace(*req.Name)
		if f.Name == "" {
			return http.StatusBadRequest, "a folder needs a name"
		}
	}
	if req.Color != nil {
		if !validHexColor(*req.Color) {
			return http.StatusBadRequest, "color must be a #rrggbb hex colour"
		}
		f.Color = *req.Color
	}
	if req.ParentID != nil {
		p := *req.ParentID
		if (f.WorkspaceID == "" && !h.myFolderIn(f.OwnerID, p)) || (f.WorkspaceID != "" && !h.folderIn(f.WorkspaceID, p)) {
			return http.StatusBadRequest, "no such folder here"
		}
		if f.ID != "" && h.loops(f.WorkspaceID, f.ID, p) {
			return http.StatusBadRequest, "a folder can't go inside itself"
		}
		f.ParentID = p
	}
	if req.ListID != nil {
		l := *req.ListID
		if l != "" && f.WorkspaceID == "" {
			return http.StatusBadRequest, "a personal folder has no task list"
		}
		if l != "" {
			var n int64
			h.db.Model(&models.TaskList{}).Where("id = ? AND workspace_id = ?", l, f.WorkspaceID).Count(&n)
			if n != 1 {
				return http.StatusBadRequest, "no such list here"
			}
		}
		f.ListID = l
	}
	return 0, ""
}

// unlistElsewhere takes the folder's list out of any other folder (a list
// is in one folder at most).
func unlistElsewhere(tx *gorm.DB, f models.Folder) error {
	if f.ListID == "" {
		return nil
	}
	return tx.Model(&models.Folder{}).Where("workspace_id = ? AND list_id = ? AND id <> ?", f.WorkspaceID, f.ListID, f.ID).UpdateColumn("list_id", "").Error
}

func (h *Handlers) folderAccess(c *gin.Context, need string) (models.Folder, bool) {
	var f models.Folder
	if err := h.db.First(&f, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return f, false
	}
	if f.WorkspaceID == "" { // a personal folder (My files): its owner's alone
		if f.OwnerID != auth.UserID(c) {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return f, false
		}
		if need != models.WSViewer && auth.Effective(c).ReadOnly {
			c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
			return f, false
		}
		return f, true
	}
	_, _, ok := h.wsAccess(c, f.WorkspaceID, need)
	return f, ok
}

// myFolderIn reports whether id is one of uid's personal folders ("" is the top).
func (h *Handlers) myFolderIn(uid, id string) bool {
	if id == "" {
		return true
	}
	var n int64
	h.db.Model(&models.Folder{}).Where("id = ? AND workspace_id = '' AND owner_id = ?", id, uid).Count(&n)
	return n == 1
}

// publishFolders tells whoever shows a workspace's (or someone's own) folders.
func (h *Handlers) publishFolders(c *gin.Context, f models.Folder) {
	if f.WorkspaceID == "" {
		h.events.Publish(f.OwnerID, `{"type":"ws","ws":"me","what":"files"}`)
		return
	}
	h.publishWS(c, f.WorkspaceID, "files")
}

// updateFolder renames, recolours or moves a folder (editors).
//
//	@Summary	Change a folder
//	@Tags		folders
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Folder id"
//	@Param		body	body		folderRequest	true	"What changes"
//	@Success	200		{object}	models.Folder
//	@Router		/folders/{id} [put]
func (h *Handlers) updateFolder(c *gin.Context) {
	f, ok := h.folderAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req folderRequest
	if !bindJSON(c, &req) {
		return
	}
	if code, msg := h.applyFolder(&f, req); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := unlistElsewhere(tx, f); err != nil {
			return err
		}
		return tx.Model(&models.Folder{}).Where("id = ?", f.ID).Updates(map[string]any{"name": f.Name, "parent_id": f.ParentID, "color": f.Color, "list_id": f.ListID}).Error
	})
	if err != nil {
		h.internalError(c, err, "could not save the folder")
		return
	}
	h.publishFolders(c, f)
	c.JSON(http.StatusOK, f)
}

// deleteFolder deletes a folder (editors); what it held moves up to where it was.
//
//	@Summary	Delete a folder
//	@Tags		folders
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Folder id"
//	@Success	204
//	@Router		/folders/{id} [delete]
func (h *Handlers) deleteFolder(c *gin.Context) {
	f, ok := h.folderAccess(c, models.WSEditor)
	if !ok {
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		// What it held moves up: the workspace's things, or (a personal
		// folder) its owner's sketches and files.
		kinds := []any{&models.Sheet{}, &models.Diagram{}, &models.Drawing{}, &models.Page{}, &models.Upload{}}
		where, args := "workspace_id = ? AND folder_id = ?", []any{f.WorkspaceID, f.ID}
		if f.WorkspaceID == "" {
			kinds = []any{&models.Drawing{}, &models.Upload{}}
			where, args = "workspace_id = '' AND owner_id = ? AND folder_id = ?", []any{f.OwnerID, f.ID}
		}
		for _, m := range kinds {
			if err := tx.Model(m).Where(where, args...).UpdateColumn("folder_id", f.ParentID).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&models.Folder{}).Where("workspace_id = ? AND parent_id = ?", f.WorkspaceID, f.ID).UpdateColumn("parent_id", f.ParentID).Error; err != nil {
			return err
		}
		return tx.Delete(&f).Error
	})
	if err != nil {
		h.internalError(c, err, "could not delete the folder")
		return
	}
	h.publishFolders(c, f)
	c.Status(http.StatusNoContent)
}

type moveRequest struct {
	FolderID string   `json:"folder_id"` // where to ("" the top)
	Sheets   []string `json:"sheets" binding:"max=500"`
	Diagrams []string `json:"diagrams" binding:"max=500"`
	Drawings []string `json:"drawings" binding:"max=500"` // whiteboards (or, in My files, sketches)
	Pages    []string `json:"pages" binding:"max=500"`
	Files    []string `json:"files" binding:"max=500"` // files of their own (an attachment goes with what it's on)
	Folders  []string `json:"folders" binding:"max=500"`
}

// moveFiles puts sheets, diagrams, whiteboards, pages and folders in a folder
// (editors). Only the workspace's own move; a folder can't go inside itself.
//
//	@Summary	Move into a folder
//	@Tags		folders
//	@Accept		json
//	@Security	BearerAuth
//	@Param		ws		path	string		true	"Workspace id"
//	@Param		body	body	moveRequest	true	"What, where"
//	@Success	204
//	@Router		/workspaces/{ws}/files/move [post]
func (h *Handlers) moveFiles(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req moveRequest
	if !bindJSON(c, &req) {
		return
	}
	if !h.folderIn(ws.ID, req.FolderID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no such folder here"})
		return
	}
	for _, id := range req.Folders {
		if h.loops(ws.ID, id, req.FolderID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a folder can't go inside itself"})
			return
		}
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		moves := []struct {
			m   any
			ids []string
			col string
			// whiteboards only (not notes' drawings)
			extra string
		}{
			{&models.Sheet{}, req.Sheets, "folder_id", ""},
			{&models.Diagram{}, req.Diagrams, "folder_id", ""},
			{&models.Drawing{}, req.Drawings, "folder_id", "note_id = ''"},
			{&models.Page{}, req.Pages, "folder_id", ""},
			{&models.Folder{}, req.Folders, "parent_id", ""},
		}
		if len(req.Files) > 0 {
			var lineages []string
			tx.Model(&models.Upload{}).Where("workspace_id = ? AND id IN ? AND target = ?", ws.ID, req.Files, uploads.File).Pluck("lineage", &lineages)
			if err := tx.Model(&models.Upload{}).Where("workspace_id = ? AND lineage IN ?", ws.ID, append(lineages, "-")).UpdateColumn("folder_id", req.FolderID).Error; err != nil {
				return err
			}
		}
		for _, mv := range moves {
			if len(mv.ids) == 0 {
				continue
			}
			q := tx.Model(mv.m).Where("workspace_id = ? AND id IN ?", ws.ID, mv.ids)
			if mv.extra != "" {
				q = q.Where(mv.extra)
			}
			if err := q.UpdateColumn(mv.col, req.FolderID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.internalError(c, err, "could not move them")
		return
	}
	h.publishWS(c, ws.ID, "files")
	if len(req.Pages) > 0 || len(req.Files) > 0 {
		h.publishWS(c, ws.ID, "uploads")
	}
	if len(req.Pages) > 0 {
		h.publishWS(c, ws.ID, "pages")
	}
	c.Status(http.StatusNoContent)
}
