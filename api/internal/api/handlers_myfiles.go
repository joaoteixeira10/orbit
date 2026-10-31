package api

import (
	"net/http"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// My files: someone's own drive, outside any workspace — their Quick Draw
// sketches, files from the computer, and their Quick Notes' attachments, in
// folders of their own. Only they see it.

func (h *Handlers) writable(c *gin.Context) bool {
	if auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return false
	}
	return true
}

// listMyFolders lists the caller's own folders, each with what it holds.
//
//	@Summary	My folders
//	@Tags		folders
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	folderResponse
//	@Router		/me/folders [get]
func (h *Handlers) listMyFolders(c *gin.Context) {
	uid := auth.UserID(c)
	var folders []models.Folder
	h.db.Where("workspace_id = '' AND owner_id = ?", uid).Order("name").Find(&folders)
	type row struct {
		F string
		N int64
	}
	var boards, subs []row
	h.db.Model(&models.Drawing{}).Select("folder_id AS f, COUNT(*) AS n").Where("workspace_id = '' AND note_id = '' AND owner_id = ? AND folder_id <> ''", uid).Group("folder_id").Scan(&boards)
	h.db.Model(&models.Folder{}).Select("parent_id AS f, COUNT(*) AS n").Where("workspace_id = '' AND owner_id = ? AND parent_id <> ''", uid).Group("parent_id").Scan(&subs)
	nb, ns, nf := map[string]int64{}, map[string]int64{}, map[string]int64{}
	for _, r := range boards {
		nb[r.F] = r.N
	}
	for _, r := range subs {
		ns[r.F] = r.N
	}
	for _, x := range h.filesOf(c, "", false) {
		nf[x.FolderID]++
	}
	out := make([]folderResponse, 0, len(folders))
	for _, f := range folders {
		out = append(out, folderResponse{Folder: f, Counts: folderCounts{Boards: nb[f.ID], Files: nf[f.ID], Folders: ns[f.ID]}})
	}
	c.JSON(http.StatusOK, out)
}

// createMyFolder makes one of the caller's own folders.
//
//	@Summary	New folder of my own
//	@Tags		folders
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		folderRequest	true	"Name, where"
//	@Success	201		{object}	models.Folder
//	@Router		/me/folders [post]
func (h *Handlers) createMyFolder(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	var req folderRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	f := models.Folder{OwnerID: uid, CreatedBy: uid}
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
	h.publishFolders(c, f)
	c.JSON(http.StatusCreated, f)
}

// moveMyFiles puts the caller's sketches, files and folders in one of their
// folders ("" the top).
//
//	@Summary	Move into a folder of my own
//	@Tags		folders
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	moveRequest	true	"What (drawings, files, folders), where"
//	@Success	204
//	@Router		/me/files/move [post]
func (h *Handlers) moveMyFiles(c *gin.Context) {
	if !h.writable(c) {
		return
	}
	var req moveRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	if !h.myFolderIn(uid, req.FolderID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no such folder here"})
		return
	}
	for _, id := range req.Folders {
		if h.loops("", id, req.FolderID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a folder can't go inside itself"})
			return
		}
	}
	mine := "workspace_id = '' AND owner_id = ?"
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if len(req.Drawings) > 0 {
			if err := tx.Model(&models.Drawing{}).Where(mine+" AND note_id = '' AND id IN ?", uid, req.Drawings).UpdateColumn("folder_id", req.FolderID).Error; err != nil {
				return err
			}
		}
		if len(req.Folders) > 0 {
			if err := tx.Model(&models.Folder{}).Where(mine+" AND id IN ?", uid, req.Folders).UpdateColumn("parent_id", req.FolderID).Error; err != nil {
				return err
			}
		}
		if len(req.Files) > 0 {
			var lineages []string
			tx.Model(&models.Upload{}).Where(mine+" AND id IN ? AND target = ?", uid, req.Files, uploads.File).Pluck("lineage", &lineages)
			if err := tx.Model(&models.Upload{}).Where(mine+" AND lineage IN ?", uid, append(lineages, "-")).UpdateColumn("folder_id", req.FolderID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.internalError(c, err, "could not move them")
		return
	}
	h.events.Publish(uid, `{"type":"ws","ws":"me","what":"files"}`)
	c.Status(http.StatusNoContent)
}

// listMyFiles lists the caller's own files: from the computer, and their
// Quick Notes' attachments; ?trash=1 those in the trash.
//
//	@Summary	My files
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Param		trash	query	bool	false	"The trash"
//	@Success	200		{array}	fileResponse
//	@Router		/me/files [get]
func (h *Handlers) listMyFiles(c *gin.Context) {
	c.JSON(http.StatusOK, h.filesOf(c, "", c.Query("trash") == "1"))
}

// myFilesUsage says how much space the caller's own files take.
//
//	@Summary	My space used
//	@Tags		files
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	fileUsage
//	@Router		/me/files/usage [get]
func (h *Handlers) myFilesUsage(c *gin.Context) {
	uid := auth.UserID(c)
	var n int64
	h.db.Model(&models.Upload{}).Where("workspace_id = '' AND owner_id = ? AND "+uploads.Shown, uid).Count(&n)
	c.JSON(http.StatusOK, fileUsage{Used: uploads.Used(h.db, "workspace_id = '' AND owner_id = ?", uid), Files: n, Quota: h.cfg.UploadQuotaBytes, All: uploads.Used(h.db, "")})
}
