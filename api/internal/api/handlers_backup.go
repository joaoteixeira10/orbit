package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/backup"
	"github.com/joaoteixeira10/orbit/api/internal/uploads"
	"github.com/joaoteixeira10/orbit/api/internal/version"

	"github.com/gin-gonic/gin"
)

type backupExportRequest struct {
	// Passphrase (12+ characters): a portable archive, opened with it instead
	// of ORBIT_MASTER_KEY. Empty = sealed with the master key.
	Passphrase string `json:"passphrase"`
}

// exportBackup downloads an encrypted archive of the whole database (owner).
//
//	@Summary	Export an encrypted backup
//	@Tags		backup
//	@Accept		json
//	@Produce	application/json
//	@Security	BearerAuth
//	@Param		body	body	backupExportRequest	false	"Optional passphrase"
//	@Success	200		{file}	binary
//	@Router		/backup/export [post]
func (h *Handlers) exportBackup(c *gin.Context) {
	var req backupExportRequest
	_ = c.ShouldBindJSON(&req) // the body is optional
	if !backup.Supported(h.db) {
		c.JSON(http.StatusConflict, gin.H{"error": backup.ErrNotSQLite.Error()})
		return
	}
	archive, err := backup.Export(h.db, h.sealer, req.Passphrase, version.Current().Version, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mode := "masterkey"
	if req.Passphrase != "" {
		mode = "passphrase"
	}
	h.recordCtx(c, "backup.export", "backup", mode, gin.H{"mode": mode, "bytes": len(archive)})
	h.securityAlert(c, "backup_export", "An encrypted backup was exported ("+mode+")")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="orbit-backup-%s-%s.json"`, mode, time.Now().UTC().Format("20060102-150405")))
	c.Data(http.StatusOK, "application/json", archive)
}

type backupScheduleView struct {
	backup.Schedule
	Last backup.LastRun `json:"last"`
	// Supported: backups can be made here (SQLite); on PostgreSQL they're
	// made with its own tools.
	Supported bool `json:"supported"`
	// FilesStore is where files' bytes are kept (db, dir or s3): only in
	// the database are they in the archive.
	FilesStore string `json:"files_store"`
}

// getBackupSchedule returns the automatic backup and how its last run went (owner).
//
//	@Summary	Scheduled backups
//	@Tags		backup
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	backupScheduleView
//	@Router		/backup/schedule [get]
func (h *Handlers) getBackupSchedule(c *gin.Context) {
	c.JSON(http.StatusOK, backupScheduleView{backup.LoadSchedule(h.db), backup.LoadLastRun(h.db), backup.Supported(h.db), uploads.Current().Kind()})
}

// updateBackupSchedule sets the automatic backup (owner).
//
//	@Summary	Set scheduled backups
//	@Tags		backup
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		backup.Schedule	true	"Schedule"
//	@Success	200		{object}	backupScheduleView
//	@Router		/backup/schedule [put]
func (h *Handlers) updateBackupSchedule(c *gin.Context) {
	var s backup.Schedule
	if !bindJSON(c, &s) {
		return
	}
	if s.Dest == "" {
		s.Dest = "dir"
	}
	if err := s.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := backup.SaveSchedule(h.db, s); err != nil {
		h.internalError(c, err, "could not save the schedule")
		return
	}
	h.recordCtx(c, "backup.schedule_update", "backup", "", gin.H{"enabled": s.Enabled, "interval_hours": s.IntervalHours, "dest": s.Dest, "keep": s.Keep})
	h.getBackupSchedule(c)
}

// runBackupNow writes one scheduled-style backup now, to test the destination (owner).
//
//	@Summary	Run the scheduled backup now
//	@Tags		backup
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	backup.LastRun
//	@Router		/backup/run [post]
func (h *Handlers) runBackupNow(c *gin.Context) {
	s := backup.LoadSchedule(h.db)
	if s.Dest == "" {
		s.Dest = "dir"
	}
	s.Enabled = true
	if s.IntervalHours == 0 {
		s.IntervalHours = 24
	}
	if err := s.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "set the destination first: " + err.Error()})
		return
	}
	r := backup.RunNow(h.db, h.sealer, s, version.Current().Version, time.Now())
	h.recordCtx(c, "backup.run", "backup", "", gin.H{"ok": r.OK, "where": r.Where, "error": r.Error})
	status := http.StatusOK
	if !r.OK {
		status = http.StatusBadGateway
	}
	c.JSON(status, r)
}
