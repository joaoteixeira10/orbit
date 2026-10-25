package api

import (
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Folders: a workspace's sheets, diagrams and whiteboards in folders, which
// go inside folders. Deleting a folder deletes nothing in it: what it held
// moves up to where it was. Moving files doesn't change them (no new
// version), so someone editing one isn't interrupted.

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

// listFolders lists a workspace's folders (all of them; the app makes the tree).
//
//	@Summary	A workspace's folders
//	@Tags		folders
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	models.Folder
//	@Router		/workspaces/{ws}/folders [get]
func (h *Handlers) listFolders(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	out := []models.Folder{}
	h.db.Where("workspace_id = ?", ws.ID).Order("name").Find(&out)
	c.JSON(http.StatusOK, out)
}

type folderRequest struct {
	Name     *string `json:"name" binding:"omitempty,max=200"`
	ParentID *string `json:"parent_id"`
	Color    *string `json:"color"`
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
	if err := h.db.Create(&f).Error; err != nil {
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
		if !h.folderIn(f.WorkspaceID, p) {
			return http.StatusBadRequest, "no such folder here"
		}
		if f.ID != "" && h.loops(f.WorkspaceID, f.ID, p) {
			return http.StatusBadRequest, "a folder can't go inside itself"
		}
		f.ParentID = p
	}
	return 0, ""
}

func (h *Handlers) folderAccess(c *gin.Context, need string) (models.Folder, bool) {
	var f models.Folder
	if err := h.db.First(&f, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return f, false
	}
	_, _, ok := h.wsAccess(c, f.WorkspaceID, need)
	return f, ok
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
	if err := h.db.Model(&models.Folder{}).Where("id = ?", f.ID).Updates(map[string]any{"name": f.Name, "parent_id": f.ParentID, "color": f.Color}).Error; err != nil {
		h.internalError(c, err, "could not save the folder")
		return
	}
	h.publishWS(c, f.WorkspaceID, "files")
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
		for _, m := range []any{&models.Sheet{}, &models.Diagram{}, &models.Drawing{}} {
			if err := tx.Model(m).Where("workspace_id = ? AND folder_id = ?", f.WorkspaceID, f.ID).UpdateColumn("folder_id", f.ParentID).Error; err != nil {
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
	h.publishWS(c, f.WorkspaceID, "files")
	c.Status(http.StatusNoContent)
}

type moveRequest struct {
	FolderID string   `json:"folder_id"` // where to ("" the top)
	Sheets   []string `json:"sheets" binding:"max=500"`
	Diagrams []string `json:"diagrams" binding:"max=500"`
	Drawings []string `json:"drawings" binding:"max=500"`
	Folders  []string `json:"folders" binding:"max=500"`
}

// moveFiles puts sheets, diagrams, whiteboards and folders in a folder
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
			{&models.Folder{}, req.Folders, "parent_id", ""},
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
	c.Status(http.StatusNoContent)
}
