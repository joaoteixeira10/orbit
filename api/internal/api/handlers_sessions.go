package api

import (
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"

	"github.com/gin-gonic/gin"
)

// sessionsEnabled gates the self-service session endpoints behind the feature
// flag; when off they 404 so the instance behaves exactly as before.
func (h *Handlers) sessionsEnabled(c *gin.Context) bool {
	if feature.Enabled(h.db, feature.Sessions) {
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	return false
}

type sessionView struct {
	auth.Session
	Current bool `json:"current"`
}

// listSessions returns the caller's active device/login sessions.
//
//	@Summary	List my active sessions
//	@Tags		sessions
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]interface{}
//	@Router		/me/sessions [get]
func (h *Handlers) listSessions(c *gin.Context) {
	if !h.sessionsEnabled(c) {
		return
	}
	sessions, err := h.auth.ListSessions(auth.UserID(c), time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list sessions"})
		return
	}
	cur := auth.Sid(c)
	out := make([]sessionView, len(sessions))
	for i, s := range sessions {
		out[i] = sessionView{Session: s, Current: s.ID != "" && s.ID == cur}
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

// revokeSession signs out one of the caller's sessions.
//
//	@Summary	Revoke one of my sessions
//	@Tags		sessions
//	@Security	BearerAuth
//	@Param		sid	path	string	true	"Session id"
//	@Success	204
//	@Router		/me/sessions/{sid} [delete]
func (h *Handlers) revokeSession(c *gin.Context) {
	if !h.sessionsEnabled(c) {
		return
	}
	sid := c.Param("sid")
	if err := h.auth.RevokeSession(auth.UserID(c), sid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not revoke session"})
		return
	}
	h.recordCtx(c, "session.revoke", "session", sid, nil)
	c.Status(http.StatusNoContent)
}

// revokeOtherSessions signs out every session except the caller's current one.
//
//	@Summary	Sign out all other sessions
//	@Tags		sessions
//	@Security	BearerAuth
//	@Success	204
//	@Router		/me/sessions/revoke-others [post]
func (h *Handlers) revokeOtherSessions(c *gin.Context) {
	if !h.sessionsEnabled(c) {
		return
	}
	if err := h.auth.RevokeOtherSessions(auth.UserID(c), auth.Sid(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not revoke sessions"})
		return
	}
	h.recordCtx(c, "session.revoke_others", "user", auth.UserID(c), nil)
	c.Status(http.StatusNoContent)
}
