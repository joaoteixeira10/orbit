package api

import (
	"net/http"
	"strconv"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Page history: each save of a page's text or title is kept as a version,
// the newest ones a page (how many: the owner's setting, default
// defaultVersionsKept); any of them can be put back (a new save, kept as a
// version too).

const (
	defaultVersionsKept = 50
	keyVersionsKept     = "page_versions_kept"
)

// versionsKept is how many versions a page keeps (5–1000).
func versionsKept(db *gorm.DB) int {
	var s models.Setting
	if db.Where(&models.Setting{Key: keyVersionsKept}).First(&s).Error == nil {
		if n, err := strconv.Atoi(s.Value); err == nil && n >= 5 && n <= 1000 {
			return n
		}
	}
	return defaultVersionsKept
}

// recordVersion keeps p as it is now, saved by uid (a version per save). prev is the page before
// the save: with no history yet, it's kept first (what the page was before
// history began).
func recordVersion(db *gorm.DB, p models.Page, uid string, prev *models.Page, restored int) {
	var last models.PageVersion
	found := db.Where("page_id = ?", p.ID).Order("created_at DESC").Limit(1).Find(&last).RowsAffected == 1
	if !found && prev != nil && (prev.Content != "" || prev.Title != "") && (prev.Content != p.Content || prev.Title != p.Title) {
		at := prev.UpdatedAt
		db.Create(&models.PageVersion{Base: models.Base{CreatedAt: at, UpdatedAt: at}, PageID: p.ID, WorkspaceID: p.WorkspaceID, Version: prev.Version, Title: prev.Title, Content: prev.Content, UserID: prev.UpdatedBy})
	}
	if found && last.Title == p.Title && last.Content == p.Content && restored == 0 {
		return // nothing new (a save of the icon, say)
	}
	db.Create(&models.PageVersion{PageID: p.ID, WorkspaceID: p.WorkspaceID, Version: p.Version, Title: p.Title, Content: p.Content, UserID: uid, Restored: restored})
	// The oldest go beyond what's kept.
	n := versionsKept(db)
	var keep []string
	db.Model(&models.PageVersion{}).Where("page_id = ?", p.ID).Order("created_at DESC").Limit(n).Pluck("id", &keep)
	if len(keep) == n {
		db.Where("page_id = ? AND id NOT IN ?", p.ID, keep).Delete(&models.PageVersion{})
	}
}

type pageVersionSummary struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Title     string `json:"title"`
	UserID    string `json:"user_id"`
	Restored  int    `json:"restored,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Chars     int    `json:"chars"` // its text's length
}

// listPageVersions lists a page's history, newest first (no text).
//
//	@Summary	A page's history
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Page id"
//	@Success	200	{array}	pageVersionSummary
//	@Router		/pages/{id}/versions [get]
func (h *Handlers) listPageVersions(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSViewer)
	if !ok {
		return
	}
	var vs []models.PageVersion
	h.db.Select("id", "version", "title", "user_id", "restored", "created_at", "updated_at", "length(content) AS content").Where("page_id = ?", p.ID).Order("created_at DESC").Find(&vs)
	out := make([]pageVersionSummary, 0, len(vs))
	for _, v := range vs {
		out = append(out, pageVersionSummary{ID: v.ID, Version: v.Version, Title: v.Title, UserID: v.UserID, Restored: v.Restored, CreatedAt: stamp(v.CreatedAt), UpdatedAt: stamp(v.UpdatedAt), Chars: atoiSafe(v.Content)})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) versionAccess(c *gin.Context, need string) (models.PageVersion, models.Page, bool) {
	var v models.PageVersion
	var p models.Page
	if h.db.First(&v, "id = ?", c.Param("id")).Error != nil || h.db.First(&p, "id = ?", v.PageID).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return v, p, false
	}
	if _, _, ok := h.wsAccess(c, p.WorkspaceID, need); !ok {
		return v, p, false
	}
	if !h.pageVisible(c, p) {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return v, p, false
	}
	return v, p, true
}

// getPageVersion returns one version of a page, with its text.
//
//	@Summary	A version of a page
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Version id"
//	@Success	200	{object}	models.PageVersion
//	@Router		/page-versions/{id} [get]
func (h *Handlers) getPageVersion(c *gin.Context) {
	v, _, ok := h.versionAccess(c, models.WSViewer)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, v)
}

// restorePageVersion puts a version back (editors): the page's title and
// text become that version's, saved as a new version.
//
//	@Summary	Put a version back
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Version id"
//	@Success	200	{object}	pageResponse
//	@Router		/page-versions/{id}/restore [post]
func (h *Handlers) restorePageVersion(c *gin.Context) {
	v, p, ok := h.versionAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if p.TrashedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the page is in the trash; restore it first"})
		return
	}
	uid := auth.UserID(c)
	prev := p
	err := h.db.Model(&models.Page{}).Where("id = ?", p.ID).Updates(map[string]any{
		"title": v.Title, "content": v.Content, "content_text": planner.DocText(v.Content), "version": gorm.Expr("version + 1"), "updated_by": uid,
	}).Error
	if err != nil {
		h.internalError(c, err, "could not put the version back")
		return
	}
	p = reloadPage(h, p.ID)
	recordVersion(h.db, p, uid, &prev, v.Version)
	h.publishWS(c, p.WorkspaceID, "pages")
	c.JSON(http.StatusOK, pageOut(p, false, true))
}
