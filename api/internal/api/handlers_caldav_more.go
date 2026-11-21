package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/google"
	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// More calendars of the same account in one workspace. The first linked
// calendar is the workspace's own Events; each extra one is a calendar of
// its own there (its name and colour), synced both ways — or only into
// Orbit, when the account may only read it (Google's birthdays, a calendar
// shared read-only). An event is moved between them by choosing its
// calendar. Taking an extra calendar away takes its events out of Orbit
// (they're that calendar's); the main one's events stay, as before.

type accountCalendar struct {
	Key      string `json:"key"` // Google: its id; CalDAV: its address
	Name     string `json:"name"`
	Color    string `json:"color"`
	Writable bool   `json:"writable"`
	Linked   bool   `json:"linked"` // synced in this workspace already
}

// linkOfWS is one of the workspace's calendar links (404 if not).
func (h *Handlers) linkOfWS(c *gin.Context, wsID string) (models.CalDAVLink, bool) {
	var link models.CalDAVLink
	if h.db.First(&link, "id = ? AND workspace_id = ?", c.Param("link"), wsID).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "that calendar isn't synced here"})
		return link, false
	}
	return link, true
}

// accountCalendars are the calendars of a link's account.
func (h *Handlers) accountCalendars(ctx context.Context, wsID string, link models.CalDAVLink) ([]accountCalendar, error) {
	linked := map[string]bool{}
	var urls []string
	h.db.Model(&models.CalDAVLink{}).Where("workspace_id = ?", wsID).Pluck("calendar_url", &urls)
	for _, u := range urls {
		linked[u] = true
	}
	out := []accountCalendar{}
	if link.Provider == "google" {
		access, err := h.googleAccess(ctx, link.ID, link.PasswordSealed)
		if err != nil {
			return nil, err
		}
		cals, err := google.Calendars(ctx, access)
		if err != nil {
			return nil, err
		}
		for _, cl := range cals {
			out = append(out, accountCalendar{Key: cl.ID, Name: cl.Name, Color: cl.Color, Writable: cl.Writable, Linked: linked[google.CalDAVURL(cl.ID)]})
		}
		return out, nil
	}
	cl, err := h.dav.Client(link)
	if err != nil {
		return nil, err
	}
	cals, err := cl.Calendars(ctx, link.CalendarURL)
	if err != nil {
		return nil, err
	}
	for _, x := range cals {
		out = append(out, accountCalendar{Key: x.URL, Name: x.Name, Color: x.Color, Writable: true, Linked: linked[x.URL]})
	}
	return out, nil
}

// caldavAccountCalendars lists the other calendars of a synced calendar's
// account, to add (owners).
//
//	@Summary	The calendars of a synced calendar's account
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path	string	true	"Workspace id"
//	@Param		link	path	string	true	"A synced calendar"
//	@Success	200		{array}	accountCalendar
//	@Router		/workspaces/{ws}/caldav/{link}/calendars [get]
func (h *Handlers) caldavAccountCalendars(c *gin.Context) {
	ws, ok := h.caldavOwner(c)
	if !ok {
		return
	}
	link, ok := h.linkOfWS(c, ws.ID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	cals, err := h.accountCalendars(ctx, ws.ID, link)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "listing the account's calendars: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, cals)
}

type moreCalendarRequest struct {
	Key   string `json:"key" binding:"required,max=1000"`
	Name  string `json:"name" binding:"max=200"`
	Color string `json:"color" binding:"max=9"`
}

// caldavAddCalendar syncs another calendar of the same account in the
// workspace, as a calendar of its own there (owners).
//
//	@Summary	Sync another calendar of the account
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		link	path		string				true	"A synced calendar of that account"
//	@Param		body	body		moreCalendarRequest	true	"The calendar"
//	@Success	200		{object}	caldavLinkResponse
//	@Router		/workspaces/{ws}/caldav/{link}/calendars [post]
func (h *Handlers) caldavAddCalendar(c *gin.Context) {
	ws, ok := h.caldavOwner(c)
	if !ok {
		return
	}
	from, ok := h.linkOfWS(c, ws.ID)
	if !ok {
		return
	}
	var req moreCalendarRequest
	if !bindJSON(c, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	cals, err := h.accountCalendars(ctx, ws.ID, from)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "listing the account's calendars: " + err.Error()})
		return
	}
	var pick *accountCalendar
	for i := range cals {
		if cals[i].Key == req.Key {
			pick = &cals[i]
		}
	}
	if pick == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that isn't one of the account's calendars"})
		return
	}
	if pick.Linked {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that calendar is synced here already"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = pick.Name
	}
	color := pick.Color
	if req.Color != "" && validHexColor(req.Color) {
		color = req.Color
	}
	if !validHexColor(color) {
		color = ""
	}
	addr := pick.Key
	if from.Provider == "google" {
		addr = google.CalDAVURL(pick.Key)
	}
	link := models.CalDAVLink{
		WorkspaceID: ws.ID, Provider: from.Provider, CalendarURL: addr, CalendarName: name, Host: from.Host,
		Username: from.Username, PasswordSealed: from.PasswordSealed, CreatedBy: auth.UserID(c),
		Extra: true, Color: color, ReadOnly: !pick.Writable,
	}
	if err := h.db.Create(&link).Error; err != nil {
		h.internalError(c, err, "could not save the calendar sync")
		return
	}
	_ = h.dav.Sync(ctx, link.ID) // the error is on the link
	h.db.First(&link, "id = ?", link.ID)
	h.googleWatch(ctx, link)
	h.recordCtx(c, "workspace.caldav_link", "workspace", ws.ID, gin.H{"host": link.Host, "calendar": link.CalendarName, "provider": link.Provider, "extra": true})
	h.publishWS(c, ws.ID, "events")
	c.JSON(http.StatusOK, h.caldavLinks(ws.ID))
}

// caldavRemoveCalendar stops syncing one calendar: an extra one's events go
// with it (they're that calendar's); the main one's stay (owners).
//
//	@Summary	Stop syncing one calendar
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string	true	"Workspace id"
//	@Param		link	path		string	true	"The synced calendar"
//	@Success	200		{object}	caldavLinkResponse
//	@Router		/workspaces/{ws}/caldav/{link} [delete]
func (h *Handlers) caldavRemoveCalendar(c *gin.Context) {
	ws, ok := h.caldavOwner(c)
	if !ok {
		return
	}
	link, ok := h.linkOfWS(c, ws.ID)
	if !ok {
		return
	}
	h.unwatchLink(link)
	h.dropLink(link)
	h.recordCtx(c, "workspace.caldav_unlink", "workspace", ws.ID, gin.H{"calendar": link.CalendarName, "extra": link.Extra})
	h.publishWS(c, ws.ID, "events")
	c.JSON(http.StatusOK, h.caldavLinks(ws.ID))
}

// dropLink removes a link (and, for an extra calendar, its events).
func (h *Handlers) dropLink(link models.CalDAVLink) {
	h.db.Where("link_id = ?", link.ID).Delete(&models.CalDAVItem{})
	if link.Extra {
		h.db.Where("workspace_id = ? AND calendar_id = ?", link.WorkspaceID, link.ID).Delete(&models.Event{})
	}
	h.db.Delete(&link)
}

// eventCalendar checks a calendar an event is put in: "" (the workspace's
// own Events) or an extra one of the workspace that may be written.
func (h *Handlers) eventCalendar(c *gin.Context, wsID, calID string) bool {
	if calID == "" {
		return true
	}
	var link models.CalDAVLink
	if h.db.First(&link, "id = ? AND workspace_id = ? AND extra = ?", calID, wsID, true).Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no such calendar in this workspace"})
		return false
	}
	if link.ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "“" + link.CalendarName + "” may only be read: change it where it comes from"})
		return false
	}
	return true
}

// readOnlyCalendar: is an event in a calendar that may only be read? (It's
// answered as such, and not changed.)
func (h *Handlers) readOnlyCalendar(c *gin.Context, ev models.Event) bool {
	if ev.CalendarID == "" {
		return false
	}
	var link models.CalDAVLink
	if h.db.Select("read_only", "calendar_name").First(&link, "id = ?", ev.CalendarID).Error == nil && link.ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "“" + link.CalendarName + "” may only be read: change it where it comes from"})
		return true
	}
	return false
}
