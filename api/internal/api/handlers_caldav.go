package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/caldav"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Two-way CalDAV: a workspace's calendar kept in step with a calendar on a
// CalDAV server (iCloud, Nextcloud, Fastmail…). Owners set it up.

type caldavLinkResponse struct {
	Link *models.CalDAVLink `json:"link"`
}

// caldavOwner checks the feature and that the caller owns the workspace.
func (h *Handlers) caldavOwner(c *gin.Context) (models.Workspace, bool) {
	if !feature.Enabled(h.db, feature.CalDAVSync) {
		c.JSON(http.StatusForbidden, gin.H{"error": "calendar sync is turned off on this instance"})
		return models.Workspace{}, false
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	return ws, ok
}

// getCalDAV returns a workspace's CalDAV link (null when none; members).
//
//	@Summary	A workspace's CalDAV sync
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	caldavLinkResponse
//	@Router		/workspaces/{ws}/caldav [get]
func (h *Handlers) getCalDAV(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var link models.CalDAVLink
	if h.db.Where("workspace_id = ?", ws.ID).First(&link).Error != nil {
		c.JSON(http.StatusOK, caldavLinkResponse{})
		return
	}
	c.JSON(http.StatusOK, caldavLinkResponse{Link: &link})
}

type caldavAccountRequest struct {
	URL      string `json:"url" binding:"required,max=1000"`
	Username string `json:"username" binding:"required,max=255"`
	Password string `json:"password" binding:"required,max=500"`
}

// webURL checks an address someone typed (https assumed when left out).
func webURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return "", false
	}
	return u.String(), true
}

// caldavCalendars lists an account's calendars, to pick one (owners).
//
//	@Summary	Find a CalDAV account's calendars
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path	string					true	"Workspace id"
//	@Param		body	body	caldavAccountRequest	true	"Server, username, (app) password"
//	@Success	200		{array}	caldav.Calendar
//	@Router		/workspaces/{ws}/caldav/calendars [post]
func (h *Handlers) caldavCalendars(c *gin.Context) {
	if _, ok := h.caldavOwner(c); !ok {
		return
	}
	var req caldavAccountRequest
	if !bindJSON(c, &req) {
		return
	}
	addr, ok := webURL(req.URL)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the server's address must be a web address (https://…)"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	cals, err := h.dav.Account(strings.TrimSpace(req.Username), req.Password).Calendars(ctx, addr)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, caldav.ErrAuth) {
			code = http.StatusUnauthorized
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cals)
}

type caldavLinkRequest struct {
	caldavAccountRequest
	CalendarURL  string `json:"calendar_url" binding:"required,max=1000"`
	CalendarName string `json:"calendar_name" binding:"max=200"`
}

// setCalDAV links the workspace's calendar to one on a CalDAV server (or
// to another one) and runs the first sync: both sides' events meet (owners).
//
//	@Summary	Sync a workspace's calendar with a CalDAV calendar
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		body	body		caldavLinkRequest	true	"The account and the calendar picked"
//	@Success	200		{object}	caldavLinkResponse
//	@Router		/workspaces/{ws}/caldav [put]
func (h *Handlers) setCalDAV(c *gin.Context) {
	ws, ok := h.caldavOwner(c)
	if !ok {
		return
	}
	var req caldavLinkRequest
	if !bindJSON(c, &req) {
		return
	}
	cal, ok1 := webURL(req.CalendarURL)
	if !ok1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pick one of the account's calendars"})
		return
	}
	sealed, err := h.dav.Seal(req.Password)
	if err != nil {
		h.internalError(c, err, "could not keep the password")
		return
	}
	u, _ := url.Parse(cal)
	link := models.CalDAVLink{
		WorkspaceID: ws.ID, CalendarURL: cal, CalendarName: strings.TrimSpace(req.CalendarName), Host: u.Hostname(),
		Username: strings.TrimSpace(req.Username), PasswordSealed: sealed, CreatedBy: auth.UserID(c),
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var old models.CalDAVLink
		if tx.Where("workspace_id = ?", ws.ID).First(&old).Error == nil {
			if err := tx.Where("link_id = ?", old.ID).Delete(&models.CalDAVItem{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}
		return tx.Create(&link).Error
	})
	if err != nil {
		h.internalError(c, err, "could not save the calendar sync")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	_ = h.dav.Sync(ctx, link.ID) // the error is on the link
	h.db.First(&link, "id = ?", link.ID)
	h.recordCtx(c, "workspace.caldav_link", "workspace", ws.ID, gin.H{"host": link.Host, "calendar": link.CalendarName})
	h.publishWS(c, ws.ID, "events")
	c.JSON(http.StatusOK, caldavLinkResponse{Link: &link})
}

// syncCalDAV syncs now (editors: after a change in the other app).
//
//	@Summary	Sync with the CalDAV calendar now
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	caldavLinkResponse
//	@Router		/workspaces/{ws}/caldav/sync [post]
func (h *Handlers) syncCalDAV(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var link models.CalDAVLink
	if h.db.Where("workspace_id = ?", ws.ID).First(&link).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "this workspace's calendar isn't synced"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	_ = h.dav.Sync(ctx, link.ID)
	h.db.First(&link, "id = ?", link.ID)
	c.JSON(http.StatusOK, caldavLinkResponse{Link: &link})
}

// deleteCalDAV stops the sync; the events stay on both sides (owners).
//
//	@Summary	Stop syncing with the CalDAV calendar
//	@Tags		calendar
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	204
//	@Router		/workspaces/{ws}/caldav [delete]
func (h *Handlers) deleteCalDAV(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var links []string
	h.db.Model(&models.CalDAVLink{}).Where("workspace_id = ?", ws.ID).Pluck("id", &links)
	h.db.Where("link_id IN ?", append(links, "")).Delete(&models.CalDAVItem{})
	h.db.Where("workspace_id = ?", ws.ID).Delete(&models.CalDAVLink{})
	h.recordCtx(c, "workspace.caldav_unlink", "workspace", ws.ID, nil)
	c.Status(http.StatusNoContent)
}

// caldavKick syncs a workspace's calendar soon after a change in Orbit.
func (h *Handlers) caldavKick(wsID string) {
	if h.dav != nil && feature.Enabled(h.db, feature.CalDAVSync) {
		h.dav.Kick(wsID)
	}
}
