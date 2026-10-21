package api

import (
	"io"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Instant updates (webhooks) and milestones of git-linked lists. See
// internal/gitsync/hooks.go.

// gitHookURL is the address the git server posts a list's events to.
func (h *Handlers) gitHookURL(c *gin.Context, linkID string) string {
	return strings.TrimRight(h.baseURL(c), "/") + "/api/v1/git/hook/" + linkID
}

// gitHook receives a repository's webhook (public; signed): a valid post
// syncs the list at once.
//
//	@Summary	A git server's webhook
//	@Tags		git
//	@Accept		json
//	@Param		link	path	string	true	"The list's link id"
//	@Success	202
//	@Router		/git/hook/{link} [post]
func (h *Handlers) gitHook(c *gin.Context) {
	if !h.gitOn() {
		c.Status(http.StatusNotFound)
		return
	}
	var link models.GitLink
	if h.db.First(&link, "id = ?", c.Param("link")).Error != nil {
		c.Status(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4<<20))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := h.git.VerifyHook(link, body, c.Request.Header); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	h.db.Model(&link).Update("hook_at", &now)
	// A ping (the server testing the hook) only says it works.
	if ev := c.GetHeader("X-GitHub-Event") + c.GetHeader("X-Gitea-Event"); !strings.Contains(ev, "ping") {
		h.git.Kick(link.ID)
	}
	c.Status(http.StatusAccepted)
}

// gitHookSetup turns instant updates on for a linked list: Orbit makes the
// webhook with the caller's account when it may (repository admin);
// otherwise it answers the address and secret to add by hand (shown once).
//
//	@Summary	Turn on instant updates (a webhook)
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"List id"
//	@Success	200	{object}	gitsync.HookSetup
//	@Router		/lists/{id}/git/hook [post]
func (h *Handlers) gitHookOn(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	l, ok := h.listAccess(c, models.WSEditor)
	if !ok {
		return
	}
	link := h.git.ListLink(l.ID)
	if link == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "this list isn't linked to a repository"})
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	setup, err := h.git.EnableHook(ctx, auth.UserID(c), link, h.gitHookURL(c, link.ID))
	if err != nil {
		h.internalError(c, err, "could not turn on instant updates")
		return
	}
	h.recordCtx(c, "git.hook_on", "list", l.ID, gin.H{"auto": setup.Auto})
	h.publishWS(c, l.WorkspaceID, "lists")
	c.JSON(http.StatusOK, setup)
}

// gitHookOff turns instant updates off (removing the webhook Orbit made).
//
//	@Summary	Turn off instant updates
//	@Tags		git
//	@Security	BearerAuth
//	@Param		id	path	string	true	"List id"
//	@Success	204
//	@Router		/lists/{id}/git/hook [delete]
func (h *Handlers) gitHookOff(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	l, ok := h.listAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if link := h.git.ListLink(l.ID); link != nil {
		ctx, cancel := gitCtx(c)
		defer cancel()
		h.git.DisableHook(ctx, auth.UserID(c), link)
		h.recordCtx(c, "git.hook_off", "list", l.ID, nil)
		h.publishWS(c, l.WorkspaceID, "lists")
	}
	c.Status(http.StatusNoContent)
}

// gitMilestones lists the open milestones of a linked list's repository.
//
//	@Summary	A linked list's milestones
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"List id"
//	@Success	200	{array}	gitsync.MilestoneChoice
//	@Router		/lists/{id}/git/milestones [get]
func (h *Handlers) gitMilestones(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	l, ok := h.listAccess(c, models.WSViewer)
	if !ok {
		return
	}
	link := h.git.ListLink(l.ID)
	if link == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "this list isn't linked to a repository"})
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	ms, err := h.git.Milestones(ctx, *link)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ms)
}
