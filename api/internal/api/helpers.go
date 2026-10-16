package api

import (
	"net/http"
	"strconv"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// recordCtx writes an audit entry for an authenticated request, deriving the
// actor, client IP, and (when present) the impersonator from the context. Use
// this for every privileged mutation so impersonated actions are attributed to
// the real admin behind them. Pre-auth flows (login, etc.) still use
// audit.Record directly with an explicit user id.
func (h *Handlers) recordCtx(c *gin.Context, action, targetType, targetID string, detail any) {
	var imp *string
	if id := auth.ImpersonatorID(c); id != "" {
		imp = &id
	}
	h.audit.RecordImpersonated(auth.UserID(c), imp, action, targetType, targetID, c.ClientIP(), detail)
}

// internalError logs the underlying error server-side and returns a generic
// message to the client, so DB/driver internals never leak in 500 responses.
func (h *Handlers) internalError(c *gin.Context, err error, msg string) {
	h.log.Error(msg, zap.String("path", c.Request.URL.Path), zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}

// bindJSON binds and validates the request body, writing a 400 on failure.
// Returns false if the caller should stop.
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	return true
}

// idParam returns the :id path parameter (a UUID string), writing a 400 if absent.
func idParam(c *gin.Context) (string, bool) {
	return pathParam(c, "id")
}

// pathParam returns a named path parameter, writing a 400 if it's empty.
func pathParam(c *gin.Context, name string) (string, bool) {
	v := c.Param(name)
	if v == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return "", false
	}
	return v, true
}

// pageParams parses 1-based page + per_page query params, clamping per_page to
// [1, max] (defaulting to def) and page to >= 1.
func pageParams(c *gin.Context, def, max int) (page, perPage int) {
	page = 1
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 1 {
		page = v
	}
	perPage = def
	if v, err := strconv.Atoi(c.Query("per_page")); err == nil && v > 0 {
		perPage = v
	}
	if perPage > max {
		perPage = max
	}
	return page, perPage
}

// The reload helpers read a row into a fresh value. Reading into the struct
// that was just updated would keep old values where the column is now NULL
// (GORM leaves such fields alone), e.g. a reopened task's completed_at.

func reloadPage(h *Handlers, id string) (p models.Page) {
	h.db.First(&p, "id = ?", id)
	return p
}

func reloadTask(h *Handlers, id string) (t models.Task) {
	h.db.First(&t, "id = ?", id)
	return t
}

func reloadTaskList(h *Handlers, id string) (l models.TaskList) {
	h.db.First(&l, "id = ?", id)
	return l
}

func reloadWorkspace(h *Handlers, id string) (ws models.Workspace) {
	h.db.First(&ws, "id = ?", id)
	return ws
}

// notImpersonating refuses what would outlast an impersonation (its token
// expires in minutes): new sessions or API tokens, password and two-factor
// changes, passkeys, git accounts, push devices, calendar feeds — all made as
// the impersonated user and theirs to keep.
func notImpersonating() gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth.ImpersonatorID(c) != "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not while acting as another user"})
			return
		}
		c.Next()
	}
}
