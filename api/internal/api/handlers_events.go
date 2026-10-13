package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"

	"github.com/gin-gonic/gin"
)

// sseHeartbeat keeps the stream (and any intermediate proxy) alive between events.
const sseHeartbeat = 25 * time.Second

// eventsTicket mints a short-lived, single-use ticket the SPA uses to open the
// event stream. The browser EventSource API cannot send an Authorization
// header, so the stream authenticates with this ticket instead of the bearer
// token (which we never want in a URL).
func (h *Handlers) eventsTicket(c *gin.Context) {
	tok, err := h.events.IssueTicket(auth.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue stream ticket"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": tok})
}

// eventStream is the Server-Sent Events stream. It authenticates by redeeming a
// ticket (?ticket=…), then holds the connection open and pushes a named event
// whenever the user's effective privileges may have changed — prompting the SPA
// to re-fetch /auth/me so privilege-gated UI updates without a page reload.
func (h *Handlers) eventStream(c *gin.Context) {
	uid, ok := h.events.Redeem(c.Query("ticket"))
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired stream ticket"})
		return
	}
	ch, unsubscribe := h.events.Subscribe(uid)
	defer unsubscribe()

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no") // ask nginx not to buffer the stream
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(c.Writer, ": connected\n\n")
	c.Writer.Flush()

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done(): // client disconnected
			return
		case msg, open := <-ch:
			if !open {
				return
			}
			// msg is a JSON payload like {"type":"privileges"} — the SPA's live
			// bus dispatches on the "type" field.
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", msg)
			c.Writer.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		}
	}
}

// liveEvent serializes a live-bus message: {"type":...} plus any extra fields.
func liveEvent(typ string) string {
	b, _ := json.Marshal(gin.H{"type": typ})
	return string(b)
}

// notifyPrivilegeChange pushes a "privileges" event to each affected user's open
// sessions, prompting a re-fetch of /auth/me. Best-effort: a user with no live
// stream simply gets nothing.
func (h *Handlers) notifyPrivilegeChange(userIDs ...string) {
	msg := liveEvent("privileges")
	for _, id := range userIDs {
		h.events.Publish(id, msg)
	}
}

// notifyRolesChanged broadcasts that the role set changed, so any open role
// admin view re-fetches its list. Reading roles is gated by the list endpoint.
func (h *Handlers) notifyRolesChanged() {
	h.events.Broadcast(liveEvent("roles"))
}

// notifyUsersChanged broadcasts that the user set changed, so any open user
// admin view re-fetches its list. Reading users is gated by the list endpoint.
func (h *Handlers) notifyUsersChanged() {
	h.events.Broadcast(liveEvent("users"))
}

// usersWithRole returns the IDs of users currently assigned a given role.
func (h *Handlers) usersWithRole(roleID string) []string {
	var ids []string
	h.db.Table("user_roles").Where("role_id = ?", roleID).Pluck("user_id", &ids)
	return ids
}
