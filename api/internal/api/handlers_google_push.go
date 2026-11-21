package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/joaoteixeira10/orbit/api/internal/feature"
	"github.com/joaoteixeira10/orbit/api/internal/google"
	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// Instant updates from Google Calendar. A calendar synced with Google is
// also watched: Google calls /api/v1/google/push on every change, and Orbit
// syncs it at once instead of at the next 5-minute run (which goes on, so a
// missed call is caught up). A watch lapses after about a week; the 5-minute
// run renews it a day before. It needs Orbit's public https address
// (PUBLIC_URL), where Google can reach it; without one, or when Google says
// no, the link says why and the 5-minute sync is all there is.

const (
	watchRenewBefore = 24 * time.Hour   // renew a watch this long before it lapses
	watchRetryAfter  = 6 * time.Hour    // after a failure, try again this much later
	watchTimeout     = 20 * time.Second // each call to Google about watching
)

// googlePushAddress is where Google calls ("" when Orbit has no public https
// address: Google calls only https).
func (h *Handlers) googlePushAddress() string {
	base := strings.TrimRight(h.cfg.PublicURL, "/")
	u, err := url.Parse(base)
	if base == "" || err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return base + "/api/v1/google/push"
}

// googleWatch makes sure a Google link is watched: a new watch when it has
// none or its watch lapses within a day (then the old one is stopped).
// Failures are kept on the link and tried again after a while.
func (h *Handlers) googleWatch(ctx context.Context, link models.CalDAVLink) {
	h.watchGoogle(ctx, link, false)
}

// watchGoogle is googleWatch; again (Sync now) tries at once after a failure.
func (h *Handlers) watchGoogle(ctx context.Context, link models.CalDAVLink, again bool) {
	if link.Provider != "google" {
		return
	}
	now := time.Now()
	if link.WatchChannel != "" && link.WatchUntil != nil && link.WatchUntil.After(now.Add(watchRenewBefore)) {
		return
	}
	if !again && link.WatchError != "" && link.WatchTriedAt != nil && now.Sub(*link.WatchTriedAt) < watchRetryAfter {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, watchTimeout)
	defer cancel()
	fail := func(msg string) {
		h.db.Model(&models.CalDAVLink{}).Where("id = ?", link.ID).Updates(map[string]any{"watch_error": truncate(msg, 500), "watch_tried_at": &now})
	}
	address := h.googlePushAddress()
	if address == "" {
		fail("instant updates need Orbit's public https address (PUBLIC_URL), where Google can reach it")
		return
	}
	calID := google.CalendarIDOf(link.CalendarURL)
	if calID == "" {
		fail("this link's Google calendar can't be told from its address")
		return
	}
	access, err := h.googleAccess(ctx, link.ID, link.PasswordSealed)
	if err != nil {
		fail(err.Error())
		return
	}
	token := randomString()
	ch, err := google.Watch(ctx, access, calID, uuid.NewString(), address, token)
	if err != nil {
		h.log.Warn("google calendar watch failed", zap.String("link", link.ID), zap.Error(err))
		fail(err.Error())
		return
	}
	res := h.db.Model(&models.CalDAVLink{}).Where("id = ?", link.ID).Updates(map[string]any{
		"watch_channel": ch.ID, "watch_resource": ch.Resource, "watch_token_hash": linkHash(token),
		"watch_until": &ch.Expiry, "watch_error": "", "watch_tried_at": &now,
	})
	if res.RowsAffected == 0 {
		// The link went meanwhile: no one to tell.
		_ = google.StopWatch(ctx, access, ch.ID, ch.Resource)
		return
	}
	if link.WatchChannel != "" && link.WatchResource != "" {
		_ = google.StopWatch(ctx, access, link.WatchChannel, link.WatchResource)
	}
}

// unwatchGoogle stops the watches of a workspace's Google links — its main
// one, or all of them — in the background (before they're replaced or
// removed). If Google can't be told, its calls go unanswered until the
// watch lapses.
func (h *Handlers) unwatchGoogle(wsID string, all bool) {
	q := h.db.Where("workspace_id = ? AND provider = ? AND watch_channel <> ''", wsID, "google")
	if !all {
		q = q.Where("extra = ?", false)
	}
	var links []models.CalDAVLink
	q.Find(&links)
	for _, l := range links {
		h.unwatchLink(l)
	}
}

// unwatchLink stops one Google link's watch, in the background.
func (h *Handlers) unwatchLink(link models.CalDAVLink) {
	if link.Provider != "google" || link.WatchChannel == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
		defer cancel()
		access, err := h.googleAccess(ctx, link.ID, link.PasswordSealed)
		if err == nil {
			err = google.StopWatch(ctx, access, link.WatchChannel, link.WatchResource)
		}
		if err != nil {
			h.log.Info("google calendar watch not stopped (it lapses by itself)", zap.String("link", link.ID), zap.Error(err))
		}
	}()
}

// googlePush is where Google calls on a change of a watched calendar (no
// sign-in: the channel's token proves it). It syncs that calendar now.
//
//	@Summary	A change in a watched Google calendar (called by Google)
//	@Tags		calendar
//	@Param		X-Goog-Channel-ID		header	string	true	"The watch"
//	@Param		X-Goog-Channel-Token	header	string	true	"The watch's secret"
//	@Param		X-Goog-Resource-State	header	string	true	"sync, exists or not_exists"
//	@Success	200
//	@Failure	403
//	@Router		/google/push [post]
func (h *Handlers) googlePush(c *gin.Context) {
	id := c.GetHeader("X-Goog-Channel-ID")
	if id == "" || len(id) > 64 {
		c.Status(http.StatusBadRequest)
		return
	}
	var link models.CalDAVLink
	if h.db.Where("watch_channel = ?", id).First(&link).Error != nil {
		// A watch since renewed or stopped: nothing to do (and no retries).
		c.Status(http.StatusOK)
		return
	}
	if subtle.ConstantTimeCompare([]byte(linkHash(c.GetHeader("X-Goog-Channel-Token"))), []byte(link.WatchTokenHash)) != 1 {
		c.Status(http.StatusForbidden)
		return
	}
	// "sync" only says the watch began; anything else is a change.
	if c.GetHeader("X-Goog-Resource-State") != "sync" {
		now := time.Now()
		h.db.Model(&models.CalDAVLink{}).Where("id = ?", link.ID).Update("pushed_at", &now)
		if h.dav != nil && feature.Enabled(h.db, feature.CalDAVSync) {
			h.dav.Kick(link.WorkspaceID)
		}
	}
	c.Status(http.StatusOK)
}
