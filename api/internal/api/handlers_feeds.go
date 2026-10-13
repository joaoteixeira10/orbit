package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// feedTokenPrefix marks Orbit calendar-feed tokens (for secret scanners).
const feedTokenPrefix = "ocf_"

func newFeedToken() (plain, hash, prefix string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	plain = feedTokenPrefix + hex.EncodeToString(b)
	return plain, hashFeedToken(plain), plain[:len(feedTokenPrefix)+6]
}

func hashFeedToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

// baseURL is where people reach Orbit: PUBLIC_URL, else this request's host.
func (h *Handlers) baseURL(c *gin.Context) string {
	if h.cfg.PublicURL != "" {
		return h.cfg.PublicURL
	}
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

type feedResponse struct {
	models.CalendarFeed
	WorkspaceIDs []string `json:"workspace_ids"`
}

func toFeedResponse(f models.CalendarFeed) feedResponse {
	return feedResponse{CalendarFeed: f, WorkspaceIDs: decodeList(f.WorkspaceIDs)}
}

func (h *Handlers) feedsEnabled(c *gin.Context) bool {
	if !feature.Enabled(h.db, feature.CalendarFeeds) {
		c.JSON(http.StatusNotFound, gin.H{"error": "calendar feeds are turned off on this instance"})
		return false
	}
	return true
}

// listFeeds lists my calendar feeds (no addresses: they're shown once).
//
//	@Summary	My calendar feeds
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	feedResponse
//	@Router		/me/feeds [get]
func (h *Handlers) listFeeds(c *gin.Context) {
	var feeds []models.CalendarFeed
	h.db.Where("user_id = ?", auth.UserID(c)).Order("created_at").Find(&feeds)
	out := make([]feedResponse, 0, len(feeds))
	for _, f := range feeds {
		out = append(out, toFeedResponse(f))
	}
	c.JSON(http.StatusOK, out)
}

type createFeedRequest struct {
	Name         string   `json:"name" binding:"max=120"`
	WorkspaceIDs []string `json:"workspace_ids"` // empty = all of mine
	IncludeTasks *bool    `json:"include_tasks"`
}

// createFeed makes a private ICS address of my events (and due tasks) for
// other calendar apps. The address is returned once.
//
//	@Summary	Create a calendar feed
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		createFeedRequest	true	"What it shows"
//	@Success	201		{object}	map[string]any
//	@Router		/me/feeds [post]
func (h *Handlers) createFeed(c *gin.Context) {
	if !h.feedsEnabled(c) {
		return
	}
	if auth.ViaAPIToken(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "create calendar feeds in Settings"})
		return
	}
	var req createFeedRequest
	if !bindJSON(c, &req) {
		return
	}
	mine := map[string]bool{}
	for _, id := range h.myWorkspaceIDs(c, false) {
		mine[id] = true
	}
	for _, id := range req.WorkspaceIDs {
		if !mine[id] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "you can only publish your own workspaces"})
			return
		}
	}
	plain, hash, prefix := newFeedToken()
	f := models.CalendarFeed{UserID: auth.UserID(c), Name: strings.TrimSpace(req.Name), TokenHash: hash, Prefix: prefix, WorkspaceIDs: encodeList(req.WorkspaceIDs), IncludeTasks: req.IncludeTasks == nil || *req.IncludeTasks}
	if f.Name == "" {
		f.Name = "Orbit"
	}
	if err := h.db.Create(&f).Error; err != nil {
		h.internalError(c, err, "could not create the feed")
		return
	}
	h.recordCtx(c, "calendar.feed_create", "calendar_feed", f.ID, nil)
	u := h.baseURL(c) + "/ical/" + plain + ".ics"
	c.JSON(http.StatusCreated, gin.H{"feed": toFeedResponse(f), "url": u, "webcal": "webcal://" + strings.SplitN(u, "://", 2)[1]})
}

// deleteFeed revokes a calendar feed: its address stops working.
//
//	@Summary	Revoke a calendar feed
//	@Tags		calendar
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Feed id"
//	@Success	204
//	@Router		/me/feeds/{id} [delete]
func (h *Handlers) deleteFeed(c *gin.Context) {
	res := h.db.Where("id = ? AND user_id = ?", c.Param("id"), auth.UserID(c)).Delete(&models.CalendarFeed{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}
	h.recordCtx(c, "calendar.feed_revoke", "calendar_feed", c.Param("id"), nil)
	c.Status(http.StatusNoContent)
}

// serveFeed answers GET /ical/<token>.ics for calendar apps: the feed owner's
// events (series as RRULEs) and open tasks with a due date, from the
// workspaces they still belong to. Unknown tokens get 404.
func (h *Handlers) serveFeed(c *gin.Context) {
	if !feature.Enabled(h.db, feature.CalendarFeeds) {
		c.Status(http.StatusNotFound)
		return
	}
	token := strings.TrimSuffix(c.Param("file"), ".ics")
	var f models.CalendarFeed
	if !strings.HasPrefix(token, feedTokenPrefix) || h.db.Where("token_hash = ?", hashFeedToken(token)).First(&f).Error != nil {
		c.Status(http.StatusNotFound)
		return
	}
	var owner models.User
	if h.db.First(&owner, "id = ?", f.UserID).Error != nil || owner.Disabled {
		c.Status(http.StatusNotFound)
		return
	}
	// Only workspaces the owner is still in (and not archived).
	var member []string
	h.db.Model(&models.WorkspaceMember{}).
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id").
		Where("workspace_members.user_id = ? AND workspaces.archived_at IS NULL", f.UserID).
		Pluck("workspace_members.workspace_id", &member)
	wsIDs := member
	if want := decodeList(f.WorkspaceIDs); len(want) > 0 {
		in := map[string]bool{}
		for _, id := range member {
			in[id] = true
		}
		wsIDs = nil
		for _, id := range want {
			if in[id] {
				wsIDs = append(wsIDs, id)
			}
		}
	}
	now := time.Now()
	var events []models.Event
	var tasks []models.Task
	if len(wsIDs) > 0 {
		h.db.Where("workspace_id IN ? AND (until IS NULL OR until > ?)", wsIDs, now.AddDate(0, 0, -60)).Order("starts_at").Limit(5000).Find(&events)
		if f.IncludeTasks {
			h.db.Where("workspace_id IN ? AND completed_at IS NULL AND due >= ?", wsIDs, now.AddDate(0, 0, -30).Format(planner.DateLayout)).Order("due").Limit(5000).Find(&tasks)
		}
	}
	host := c.Request.Host
	fe := make([]planner.FeedEvent, 0, len(events))
	for _, e := range events {
		fe = append(fe, planner.FeedEvent{UID: e.ID + "@" + host, Title: e.Title, Notes: e.Notes, Location: e.Location, Start: e.StartsAt, End: e.EndsAt, AllDay: e.AllDay, TZ: e.TZ, RRule: e.RRule, ExDates: planner.DecodeExDates(e.ExDates), Updated: e.UpdatedAt})
	}
	ft := make([]planner.FeedTask, 0, len(tasks))
	for _, t := range tasks {
		ft = append(ft, planner.FeedTask{UID: "task-" + t.ID + "@" + host, Title: t.Title, Notes: t.Notes, Due: *t.Due, DueTime: deref(t.DueTime), Updated: t.UpdatedAt})
	}
	if f.LastUsedAt == nil || now.Sub(*f.LastUsedAt) > 10*time.Minute {
		h.db.Model(&f).Update("last_used_at", &now)
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", planner.BuildFeed(f.Name, fe, ft, now))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
