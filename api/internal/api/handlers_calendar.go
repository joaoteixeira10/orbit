package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// maxRangeDays bounds one calendar query.
const maxRangeDays = 120

// occurrence is one entry on a calendar: an Orbit event (one occurrence of a
// series) or an event from a subscribed calendar.
type occurrence struct {
	Kind           string    `json:"kind"` // event | external
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	SubscriptionID string    `json:"subscription_id,omitempty"`
	Title          string    `json:"title"`
	Location       string    `json:"location,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	AllDay         bool      `json:"all_day"`
	Color          string    `json:"color"`
	Recurring      bool      `json:"recurring"`
	CourseID       string    `json:"course_id,omitempty"` // a class of a course
}

type calendarResponse struct {
	Events []occurrence   `json:"events"`
	Tasks  []taskResponse `json:"tasks"`
}

// parseRange reads ?from=&to= (RFC 3339) and the floating dates for tasks
// (?from_date=&to_date=, default: the UTC dates of from and to).
func parseRange(c *gin.Context) (from, to time.Time, fromDate, toDate string, ok bool) {
	var err1, err2 error
	from, err1 = time.Parse(time.RFC3339, c.Query("from"))
	to, err2 = time.Parse(time.RFC3339, c.Query("to"))
	if err1 != nil || err2 != nil || !to.After(from) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to must be RFC 3339 times, from before to"})
		return
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ask for at most 120 days at a time"})
		return
	}
	fromDate, toDate = c.Query("from_date"), c.Query("to_date")
	if !planner.ValidDate(fromDate) {
		fromDate = from.UTC().Format(planner.DateLayout)
	}
	if !planner.ValidDate(toDate) {
		toDate = to.UTC().Format(planner.DateLayout)
	}
	return from, to, fromDate, toDate, true
}

// calendarFor gathers the occurrences and due tasks of some workspaces.
func (h *Handlers) calendarFor(wsIDs []string, from, to time.Time, fromDate, toDate string) calendarResponse {
	resp := calendarResponse{Events: []occurrence{}, Tasks: []taskResponse{}}
	if len(wsIDs) == 0 {
		return resp
	}
	colors := map[string]string{}
	var wss []models.Workspace
	h.db.Select("id", "color").Where("id IN ?", wsIDs).Find(&wss)
	for _, w := range wss {
		colors[w.ID] = w.Color
	}

	var events []models.Event
	h.db.Where("workspace_id IN ? AND starts_at < ? AND (until IS NULL OR until > ?)", wsIDs, to, from).Find(&events)
	for _, ev := range events {
		color := ev.Color
		if color == "" {
			color = colors[ev.WorkspaceID]
		}
		dur := ev.EndsAt.Sub(ev.StartsAt)
		for _, s := range planner.Occurrences(ev, from, to) {
			resp.Events = append(resp.Events, occurrence{
				Kind: "event", ID: ev.ID, WorkspaceID: ev.WorkspaceID, Title: ev.Title, Location: ev.Location, Notes: ev.Notes,
				Start: s, End: s.Add(dur), AllDay: ev.AllDay, Color: color, Recurring: ev.RRule != "", CourseID: ev.CourseID,
			})
		}
	}

	if feature.Enabled(h.db, feature.CalendarSubscriptions) {
		var subs []models.CalendarSubscription
		h.db.Where("workspace_id IN ?", wsIDs).Find(&subs)
		if len(subs) > 0 {
			subByID := map[string]models.CalendarSubscription{}
			ids := make([]string, 0, len(subs))
			for _, s := range subs {
				subByID[s.ID] = s
				ids = append(ids, s.ID)
			}
			var ext []models.ExternalEvent
			h.db.Where("subscription_id IN ? AND starts_at < ? AND ends_at > ?", ids, to, from).Order("starts_at").Limit(3000).Find(&ext)
			for _, e := range ext {
				s := subByID[e.SubscriptionID]
				color := s.Color
				if color == "" {
					color = colors[s.WorkspaceID]
				}
				resp.Events = append(resp.Events, occurrence{
					Kind: "external", ID: e.ID, WorkspaceID: s.WorkspaceID, SubscriptionID: s.ID, Title: e.Title,
					Location: e.Location, Notes: e.Notes, Start: e.StartsAt, End: e.EndsAt, AllDay: e.AllDay, Color: color,
				})
			}
		}
	}
	sort.SliceStable(resp.Events, func(i, j int) bool { return resp.Events[i].Start.Before(resp.Events[j].Start) })

	var tasks []models.Task
	h.db.Where("workspace_id IN ? AND due >= ? AND due <= ?", wsIDs, fromDate, toDate).Order("due, due_time, rank").Find(&tasks)
	resp.Tasks = h.counted(toTaskResponses(tasks))
	return resp
}

// workspaceCalendar returns a workspace's calendar for a range: event
// occurrences (series expanded), subscribed events and tasks due.
//
//	@Summary	A workspace's calendar
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws			path		string	true	"Workspace id"
//	@Param		from		query		string	true	"RFC 3339 start"
//	@Param		to			query		string	true	"RFC 3339 end (at most 120 days later)"
//	@Param		from_date	query		string	false	"First date for tasks (YYYY-MM-DD)"
//	@Param		to_date		query		string	false	"Last date for tasks (YYYY-MM-DD)"
//	@Success	200			{object}	calendarResponse
//	@Router		/workspaces/{ws}/calendar [get]
func (h *Handlers) workspaceCalendar(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	from, to, fd, td, ok := parseRange(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.calendarFor([]string{ws.ID}, from, to, fd, td))
}

// myCalendar is the same across all my active workspaces.
//
//	@Summary	My calendar, all workspaces
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		from		query		string	true	"RFC 3339 start"
//	@Param		to			query		string	true	"RFC 3339 end"
//	@Param		from_date	query		string	false	"First date for tasks"
//	@Param		to_date		query		string	false	"Last date for tasks"
//	@Success	200			{object}	calendarResponse
//	@Router		/calendar [get]
func (h *Handlers) myCalendar(c *gin.Context) {
	from, to, fd, td, ok := parseRange(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.calendarFor(h.myWorkspaceIDs(c, true), from, to, fd, td))
}

// ---- Events ---------------------------------------------------------------------

type eventRequest struct {
	Title    *string    `json:"title" binding:"omitempty,max=300"`
	Notes    *string    `json:"notes" binding:"omitempty,max=20000"`
	Location *string    `json:"location" binding:"omitempty,max=300"`
	StartsAt *time.Time `json:"starts_at"`
	EndsAt   *time.Time `json:"ends_at"`
	AllDay   *bool      `json:"all_day"`
	TZ       *string    `json:"tz" binding:"omitempty,max=64"`
	RRule    *string    `json:"rrule"`
	Color    *string    `json:"color"`
	// RemindMinutes before each occurrence; a negative value removes the reminder.
	RemindMinutes *int `json:"remind_minutes" binding:"omitempty,max=40320"`
}

type eventResponse struct {
	models.Event
	ExDates []time.Time `json:"exdates"`
}

func toEventResponse(e models.Event) eventResponse {
	x := planner.DecodeExDates(e.ExDates)
	if x == nil {
		x = []time.Time{}
	}
	return eventResponse{Event: e, ExDates: x}
}

func midnightUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// applyEvent validates a request onto ev; it returns a message for a 400.
func applyEvent(req eventRequest, ev *models.Event) string {
	timingChanged := false
	if req.Title != nil {
		ev.Title = strings.TrimSpace(*req.Title)
	}
	if ev.Title == "" {
		return "give the event a title"
	}
	if req.Notes != nil {
		ev.Notes = *req.Notes
	}
	if req.Location != nil {
		ev.Location = strings.TrimSpace(*req.Location)
	}
	if req.AllDay != nil {
		timingChanged = timingChanged || ev.AllDay != *req.AllDay
		ev.AllDay = *req.AllDay
	}
	if req.StartsAt != nil {
		timingChanged = timingChanged || !ev.StartsAt.Equal(*req.StartsAt)
		ev.StartsAt = req.StartsAt.UTC()
	}
	if req.EndsAt != nil {
		ev.EndsAt = req.EndsAt.UTC()
	}
	if ev.StartsAt.IsZero() {
		return "starts_at is required"
	}
	if ev.AllDay {
		// All-day: whole dates, end exclusive.
		ev.StartsAt = midnightUTC(ev.StartsAt)
		ev.EndsAt = midnightUTC(ev.EndsAt)
		if !ev.EndsAt.After(ev.StartsAt) {
			ev.EndsAt = ev.StartsAt.AddDate(0, 0, 1)
		}
	} else if ev.EndsAt.IsZero() {
		ev.EndsAt = ev.StartsAt.Add(time.Hour)
	}
	if ev.EndsAt.Before(ev.StartsAt) {
		return "an event can't end before it starts"
	}
	if ev.EndsAt.Sub(ev.StartsAt) > 366*24*time.Hour {
		return "an event lasts a year at most"
	}
	if req.TZ != nil {
		if _, err := time.LoadLocation(*req.TZ); err != nil || *req.TZ == "Local" {
			return "tz must be an IANA time zone, like Europe/Lisbon"
		}
		ev.TZ = *req.TZ
	}
	if req.RRule != nil {
		r, err := planner.CleanRRule(*req.RRule)
		if err != nil {
			return err.Error()
		}
		timingChanged = timingChanged || r != ev.RRule
		ev.RRule = r
	}
	if req.Color != nil {
		if *req.Color != "" && !planner.ValidColor(*req.Color) {
			return "color must be #rrggbb"
		}
		ev.Color = *req.Color
	}
	if req.RemindMinutes != nil {
		if *req.RemindMinutes < 0 {
			ev.RemindMinutes = nil
		} else {
			m := *req.RemindMinutes
			ev.RemindMinutes = &m
		}
	}
	if timingChanged {
		// Removed occurrences belong to the old timing.
		ev.ExDates = ""
	}
	ev.Until = planner.SeriesUntil(*ev)
	return ""
}

func (h *Handlers) eventAccess(c *gin.Context, need string) (models.Event, bool) {
	var ev models.Event
	if err := h.db.First(&ev, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return ev, false
	}
	_, _, ok := h.wsAccess(c, ev.WorkspaceID, need)
	return ev, ok
}

// createEvent adds an event (editors).
//
//	@Summary	Create an event
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		eventRequest	true	"Event"
//	@Success	201		{object}	eventResponse
//	@Router		/workspaces/{ws}/events [post]
func (h *Handlers) createEvent(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req eventRequest
	if !bindJSON(c, &req) {
		return
	}
	ev := models.Event{WorkspaceID: ws.ID, CreatedBy: auth.UserID(c), TZ: "UTC"}
	if msg := applyEvent(req, &ev); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if err := h.db.Create(&ev).Error; err != nil {
		h.internalError(c, err, "could not create the event")
		return
	}
	h.publishWS(c, ws.ID, "events")
	h.notifyActivity(c, ws, "event_added", ev.Title, fmt.Sprintf("/w/%s/calendar?date=%s", ws.ID, ev.StartsAt.UTC().Format(planner.DateLayout)))
	c.JSON(http.StatusCreated, toEventResponse(ev))
}

// getEvent returns an event (the whole series).
//
//	@Summary	Get an event
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Event id"
//	@Success	200	{object}	eventResponse
//	@Router		/events/{id} [get]
func (h *Handlers) getEvent(c *gin.Context) {
	ev, ok := h.eventAccess(c, models.WSViewer)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, toEventResponse(ev))
}

// updateEvent changes an event; for a series, every occurrence (editors).
//
//	@Summary	Update an event
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Event id"
//	@Param		body	body		eventRequest	true	"Fields to change"
//	@Success	200		{object}	eventResponse
//	@Router		/events/{id} [put]
func (h *Handlers) updateEvent(c *gin.Context) {
	ev, ok := h.eventAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if ev.CourseID != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "this is a class of a course: change it in Courses"})
		return
	}
	var req eventRequest
	if !bindJSON(c, &req) {
		return
	}
	if msg := applyEvent(req, &ev); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if err := h.db.Save(&ev).Error; err != nil {
		h.internalError(c, err, "could not update the event")
		return
	}
	h.publishWS(c, ev.WorkspaceID, "events")
	c.JSON(http.StatusOK, toEventResponse(ev))
}

// deleteEvent deletes an event, or with ?occurrence=<start> only that
// occurrence of a series (editors).
//
//	@Summary	Delete an event or one occurrence
//	@Tags		calendar
//	@Security	BearerAuth
//	@Param		id			path	string	true	"Event id"
//	@Param		occurrence	query	string	false	"RFC 3339 start of the one occurrence to remove"
//	@Success	204
//	@Router		/events/{id} [delete]
func (h *Handlers) deleteEvent(c *gin.Context) {
	ev, ok := h.eventAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if o := c.Query("occurrence"); o != "" && ev.RRule != "" {
		t, err := time.Parse(time.RFC3339, o)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "occurrence must be an RFC 3339 time"})
			return
		}
		x := append(planner.DecodeExDates(ev.ExDates), t.UTC())
		h.db.Model(&ev).Update("ex_dates", planner.EncodeExDates(x))
	} else if ev.CourseID != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "this is a class of a course: remove it in Courses (or cancel just this one)"})
		return
	} else {
		h.db.Delete(&ev)
	}
	h.publishWS(c, ev.WorkspaceID, "events")
	c.Status(http.StatusNoContent)
}

// ---- Subscriptions --------------------------------------------------------------

type subscriptionRequest struct {
	Name           *string `json:"name" binding:"omitempty,max=120"`
	URL            *string `json:"url" binding:"omitempty,max=2000"`
	Color          *string `json:"color"`
	RefreshMinutes *int    `json:"refresh_minutes" binding:"omitempty,min=15,max=10080"`
}

func (h *Handlers) subsEnabled(c *gin.Context) bool {
	if !feature.Enabled(h.db, feature.CalendarSubscriptions) {
		c.JSON(http.StatusNotFound, gin.H{"error": "calendar subscriptions are turned off on this instance"})
		return false
	}
	return true
}

func (h *Handlers) subAccess(c *gin.Context, need string) (models.CalendarSubscription, bool) {
	var s models.CalendarSubscription
	if err := h.db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return s, false
	}
	_, _, ok := h.wsAccess(c, s.WorkspaceID, need)
	return s, ok
}

// listSubscriptions lists a workspace's subscribed calendars (no addresses).
//
//	@Summary	Subscribed calendars
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	models.CalendarSubscription
//	@Router		/workspaces/{ws}/subscriptions [get]
func (h *Handlers) listSubscriptions(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	subs := []models.CalendarSubscription{}
	if feature.Enabled(h.db, feature.CalendarSubscriptions) {
		h.db.Where("workspace_id = ?", ws.ID).Order("created_at").Find(&subs)
	}
	c.JSON(http.StatusOK, subs)
}

// createSubscription subscribes a workspace to an ICS calendar (editors). The
// calendar is fetched at once, so a wrong address is reported right away.
//
//	@Summary	Subscribe to a calendar
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		body	body		subscriptionRequest	true	"Name, address, colour"
//	@Success	201		{object}	models.CalendarSubscription
//	@Router		/workspaces/{ws}/subscriptions [post]
func (h *Handlers) createSubscription(c *gin.Context) {
	if !h.subsEnabled(c) {
		return
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req subscriptionRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.URL == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enter the calendar's address"})
		return
	}
	u, err := calsync.NormalizeURL(*req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sealed, err := h.calsync.Seal(u)
	if err != nil {
		h.internalError(c, err, "could not store the address")
		return
	}
	sub := models.CalendarSubscription{WorkspaceID: ws.ID, Name: u.Host, URLSealed: sealed, URLHost: u.Host, Color: "#8E8E93", RefreshMinutes: 60, CreatedBy: auth.UserID(c)}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		sub.Name = strings.TrimSpace(*req.Name)
	}
	if req.Color != nil && planner.ValidColor(*req.Color) {
		sub.Color = *req.Color
	}
	if req.RefreshMinutes != nil {
		sub.RefreshMinutes = *req.RefreshMinutes
	}
	// Fetch before saving: a wrong address shouldn't leave a broken row.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	if _, err := h.calsync.Fetch(ctx, u.String()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Create(&sub).Error; err != nil {
		h.internalError(c, err, "could not save the subscription")
		return
	}
	_ = h.calsync.Refresh(ctx, &sub)
	h.recordCtx(c, "calendar.subscribe", "workspace", ws.ID, gin.H{"host": u.Host})
	h.publishWS(c, ws.ID, "subscriptions")
	c.JSON(http.StatusCreated, sub)
}

// updateSubscription renames, recolours or re-points a subscription (editors).
//
//	@Summary	Update a subscription
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Subscription id"
//	@Param		body	body		subscriptionRequest	true	"Fields to change"
//	@Success	200		{object}	models.CalendarSubscription
//	@Router		/subscriptions/{id} [put]
func (h *Handlers) updateSubscription(c *gin.Context) {
	if !h.subsEnabled(c) {
		return
	}
	sub, ok := h.subAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req subscriptionRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		sub.Name = strings.TrimSpace(*req.Name)
	}
	if req.Color != nil {
		if !planner.ValidColor(*req.Color) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "color must be #rrggbb"})
			return
		}
		sub.Color = *req.Color
	}
	if req.RefreshMinutes != nil {
		sub.RefreshMinutes = *req.RefreshMinutes
	}
	refetch := false
	if req.URL != nil && *req.URL != "" {
		u, err := calsync.NormalizeURL(*req.URL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if sub.URLSealed, err = h.calsync.Seal(u); err != nil {
			h.internalError(c, err, "could not store the address")
			return
		}
		sub.URLHost, refetch = u.Host, true
	}
	if err := h.db.Save(&sub).Error; err != nil {
		h.internalError(c, err, "could not update the subscription")
		return
	}
	if refetch {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
		defer cancel()
		_ = h.calsync.Refresh(ctx, &sub)
	}
	h.publishWS(c, sub.WorkspaceID, "subscriptions")
	c.JSON(http.StatusOK, sub)
}

// refreshSubscription fetches a subscription now (editors).
//
//	@Summary	Refresh a subscription now
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Subscription id"
//	@Success	200	{object}	models.CalendarSubscription
//	@Router		/subscriptions/{id}/refresh [post]
func (h *Handlers) refreshSubscription(c *gin.Context) {
	if !h.subsEnabled(c) {
		return
	}
	sub, ok := h.subAccess(c, models.WSEditor)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	_ = h.calsync.Refresh(ctx, &sub) // the error is on the row (last_error)
	h.publishWS(c, sub.WorkspaceID, "subscriptions")
	c.JSON(http.StatusOK, sub)
}

// deleteSubscription removes a subscription and its events (editors).
//
//	@Summary	Unsubscribe
//	@Tags		calendar
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Subscription id"
//	@Success	204
//	@Router		/subscriptions/{id} [delete]
func (h *Handlers) deleteSubscription(c *gin.Context) {
	sub, ok := h.subAccess(c, models.WSEditor)
	if !ok {
		return
	}
	h.db.Where("subscription_id = ?", sub.ID).Delete(&models.ExternalEvent{})
	h.db.Delete(&sub)
	h.publishWS(c, sub.WorkspaceID, "subscriptions")
	c.Status(http.StatusNoContent)
}
