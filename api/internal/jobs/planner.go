package jobs

import (
	"context"
	"fmt"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"go.uber.org/zap"
)

// TrashDays is how long a trashed page waits before it is deleted for good.
const TrashDays = 30

// UsePlanner turns on the planner's background work: refreshing subscribed
// calendars, reminders, the daily summary and emptying old trash.
func (s *Scheduler) UsePlanner(sealer *keystore.Sealer, hub *events.Hub) *Scheduler {
	s.calsync = calsync.New(s.db, sealer, s.log)
	s.hub = hub
	return s
}

func (s *Scheduler) startPlanner() {
	if s.calsync == nil {
		return
	}
	jobs := []struct {
		spec string
		fn   func()
		name string
	}{
		{"@every 5m", s.syncCalendars, "calendar subscriptions"},
		{"* * * * *", s.reminders, "reminders"}, // on each minute boundary
		{"* * * * *", s.digests, "daily summaries"},
		{"@daily", s.emptyTrash, "trash"},
		{"@daily", s.inbox.Cleanup, "old notifications"},
	}
	for _, j := range jobs {
		if _, err := s.cron.AddFunc(j.spec, j.fn); err != nil {
			s.log.Error("schedule job failed", zap.String("job", j.name), zap.Error(err))
		}
	}
}

// syncCalendars refreshes the subscriptions that are due.
func (s *Scheduler) syncCalendars() {
	if !feature.Enabled(s.db, feature.CalendarSubscriptions) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	s.calsync.RunDue(ctx)
}

// emptyTrash deletes pages that have been in the trash for TrashDays.
func (s *Scheduler) emptyTrash() {
	cutoff := time.Now().AddDate(0, 0, -TrashDays)
	var ids []string
	s.db.Model(&models.Page{}).Where("trashed_at IS NOT NULL AND trashed_at < ?", cutoff).Pluck("id", &ids)
	if len(ids) == 0 {
		return
	}
	s.db.Where("page_id IN ?", ids).Delete(&models.PageFavorite{})
	res := s.db.Where("id IN ?", ids).Delete(&models.Page{})
	s.log.Info("emptied old trash", zap.Int64("pages", res.RowsAffected))
}

// userZone is where a user's floating due times are read: their chosen zone,
// else the browser's last reported one, else UTC.
func userZone(p models.UserPrefs) *time.Location {
	for _, tz := range []string{p.Timezone, p.DeviceTZ} {
		if tz == "" || tz == "local" {
			continue
		}
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.UTC
}

// reminders runs every minute and sends what falls due in this minute: tasks
// with a due time (to their assignee, else their creator) and event
// occurrences with a reminder (to every member of the workspace).
func (s *Scheduler) reminders() {
	s.remindAt(time.Now().Truncate(time.Minute))
}

func (s *Scheduler) remindAt(minute time.Time) {
	end := minute.Add(time.Minute)
	prefsCache := map[string]models.UserPrefs{}
	prefs := func(uid string) models.UserPrefs {
		if p, ok := prefsCache[uid]; ok {
			return p
		}
		var p models.UserPrefs
		s.db.Where("user_id = ?", uid).First(&p)
		prefsCache[uid] = p
		return p
	}
	wsNames := map[string]string{}
	wsName := func(id string) string {
		if n, ok := wsNames[id]; ok {
			return n
		}
		var ws models.Workspace
		s.db.Select("id", "name").First(&ws, "id = ?", id)
		wsNames[id] = ws.Name
		return ws.Name
	}

	// Tasks: due dates are floating, so look at the dates around today
	// everywhere and place each in its recipient's zone.
	var tasks []models.Task
	days := []string{
		minute.AddDate(0, 0, -1).UTC().Format(planner.DateLayout),
		minute.UTC().Format(planner.DateLayout),
		minute.AddDate(0, 0, 1).UTC().Format(planner.DateLayout),
	}
	s.db.Where("completed_at IS NULL AND due IN ? AND due_time IS NOT NULL", days).Find(&tasks)
	for _, t := range tasks {
		uid := t.CreatedBy
		if t.AssigneeID != nil {
			uid = *t.AssigneeID
		}
		at, err := time.ParseInLocation(planner.DateLayout+" 15:04", *t.Due+" "+*t.DueTime, userZone(prefs(uid)))
		if err != nil || at.Before(minute) || !at.Before(end) {
			continue
		}
		s.inbox.Notify(inbox.Message{
			UserID: uid, Kind: inbox.Reminder, Verb: "task_due", WorkspaceID: t.WorkspaceID,
			Params: map[string]any{"title": t.Title, "ws": wsName(t.WorkspaceID), "task": t.ID},
			Link:   fmt.Sprintf("/w/%s/tasks/%s?task=%s", t.WorkspaceID, t.ListID, t.ID),
		})
	}

	// Events: an occurrence starting RemindMinutes after this minute.
	var evs []models.Event
	s.db.Where("remind_minutes IS NOT NULL AND starts_at < ? AND (until IS NULL OR until > ?)", end.AddDate(0, 0, 8), minute).Find(&evs)
	for _, ev := range evs {
		lead := time.Duration(*ev.RemindMinutes) * time.Minute
		for _, o := range planner.Occurrences(ev, minute.Add(lead), end.Add(lead)) {
			if at := o.Add(-lead); at.Before(minute) || !at.Before(end) {
				continue
			}
			verb, params := "event_now", map[string]any{"title": ev.Title, "ws": wsName(ev.WorkspaceID), "start": o.UTC().Format(time.RFC3339)}
			if lead > 0 {
				verb, params["lead"] = "event_soon", humanLead(lead)
			}
			var members []string
			s.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ?", ev.WorkspaceID).Pluck("user_id", &members)
			for _, uid := range members {
				day := o.In(userZone(prefs(uid))).Format(planner.DateLayout)
				s.inbox.Notify(inbox.Message{
					UserID: uid, Kind: inbox.Reminder, Verb: verb, WorkspaceID: ev.WorkspaceID, Params: params,
					Link: fmt.Sprintf("/w/%s/calendar?date=%s", ev.WorkspaceID, day),
				})
			}
		}
	}
}

func humanLead(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%d d", int(d.Hours()/24))
	case d%time.Hour == 0:
		return fmt.Sprintf("%d h", int(d.Hours()))
	default:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
}
