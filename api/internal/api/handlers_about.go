package api

import (
	"encoding/json"
	"net/http"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/version"

	"github.com/gin-gonic/gin"
)

// settingVersionHistory is the Setting key storing the install history.
const settingVersionHistory = "version_history"

// VersionHistoryEntry records when a version started running.
type VersionHistoryEntry struct {
	Version     string `json:"version"`
	InstalledAt string `json:"installed_at"` // RFC3339
}

// about returns build info, update status, and version history.
//
//	@Summary	About / version info
//	@Tags		system
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]any
//	@Router		/about [get]
func (h *Handlers) about(c *gin.Context) {
	b := version.Current()
	latest := ""
	updateAvailable := false
	repoURL := version.RepositoryURL
	if h.updates != nil {
		latest = h.updates.Latest()
		updateAvailable = h.updates.UpdateAvailable()
		repoURL = h.updates.RepoURL()
	}
	c.JSON(http.StatusOK, gin.H{
		"version":              b.Version,
		"go_version":           b.GoVersion,
		"commit":               b.Commit,
		"build_time":           b.BuildTime,
		"repository_url":       repoURL,
		"db_driver":            h.cfg.DBDriver,
		"latest_version":       latest,
		"update_available":     updateAvailable,
		"update_check_enabled": h.updates != nil && h.updates.Enabled(),
		"version_history":      loadVersionHistory(h),
	})
}

func loadVersionHistory(h *Handlers) []VersionHistoryEntry {
	var row models.Setting
	if err := h.db.Where(&models.Setting{Key: settingVersionHistory}).First(&row).Error; err != nil {
		return []VersionHistoryEntry{}
	}
	var hist []VersionHistoryEntry
	if err := json.Unmarshal([]byte(row.Value), &hist); err != nil {
		return []VersionHistoryEntry{}
	}
	return hist
}
