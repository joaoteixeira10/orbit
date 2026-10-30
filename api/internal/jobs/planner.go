package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/focus"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"go.uber.org/zap"
	"gorm.io/gorm"
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
		{"* * * * *", s.notifications, "snoozed and held notifications"},
		{"* * * * *", s.digests, "daily summaries"},
		{"@daily", s.emptyTrash, "trash"},
		{"@daily", s.inbox.Cleanup, "old notifications"},
		{"@daily", s.oldActivity, "old workspace activity"},
		{"@every 3m", s.syncGit, "git-linked lists"},
		{"@every 5m", s.syncCalDAV, "CalDAV calendars"},
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
	s.db.Where("page_id IN ?", ids).Delete(&models.PageVersion{})
	s.db.Where("page_id IN ?", ids).Delete(&models.ShareLink{})
	s.db.Where("page_id IN ?", ids).Delete(&models.Form{})
	_ = uploads.DeleteFor(s.db, uploads.Page, ids)
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
		to := t.AssigneeList()
		if len(to) == 0 {
			to = []string{t.CreatedBy}
		}
		for _, uid := range to {
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
	}

	// Notes: a one-off reminder, to whoever set it (missed minutes too).
	var notes []models.Note
	s.db.Where("remind_at IS NOT NULL AND remind_at < ?", end).Find(&notes)
	for _, n := range notes {
		s.db.Model(&models.Note{}).Where("id = ?", n.ID).Updates(map[string]any{"remind_at": nil, "remind_by": ""})
		to := n.RemindBy
		if to == "" {
			to = n.OwnerID
		}
		title := n.Title
		if title == "" {
			title = firstLine(n.PlainText, 80)
		}
		link := "/notes?note=" + n.ID
		if n.WorkspaceID != "" {
			link = fmt.Sprintf("/w/%s/notes?note=%s", n.WorkspaceID, n.ID)
		}
		s.inbox.Notify(inbox.Message{
			UserID: to, Kind: inbox.Reminder, Verb: "note_reminder", WorkspaceID: n.WorkspaceID,
			Params: map[string]any{"title": title, "ws": wsName(n.WorkspaceID), "note": n.ID}, Link: link,
		})
	}

	// Habits: at their time on the days they count, unless done already.
	var habits []models.Habit
	s.db.Where("remind_at <> '' AND archived_at IS NULL").Find(&habits)
	for _, hb := range habits {
		local := minute.In(userZone(prefs(hb.OwnerID)))
		if local.Format("15:04") != hb.RemindAt || !habitDue(s.db, hb, local) {
			continue
		}
		s.inbox.Notify(inbox.Message{
			UserID: hb.OwnerID, Kind: inbox.Reminder, Verb: "habit_due", WorkspaceID: hb.WorkspaceID,
			Params: map[string]any{"title": strings.TrimSpace(hb.Icon + " " + hb.Title), "ws": wsName(hb.WorkspaceID), "habit": hb.ID},
			Link:   fmt.Sprintf("/w/%s/habits", hb.WorkspaceID),
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

// notifications brings back snoozed notifications and, for those whose
// quiet hours just ended, sends the summary of what waited.
func (s *Scheduler) notifications() {
	now := time.Now()
	s.inbox.Resurface(now)
	s.inbox.ReleaseHeld(now)
	s.focusDone(now)
}

// focusDone ends the focus sessions whose time is up and tells their person
// (on their devices too: the tab may be closed).
func (s *Scheduler) focusDone(now time.Time) {
	for _, f := range focus.Due(s.db, now) {
		if !focus.End(s.db, &f, now) {
			continue
		}
		title := ""
		if f.TaskID != "" {
			var t models.Task
			if s.db.Select("title").First(&t, "id = ?", f.TaskID).Error == nil {
				title = t.Title
			}
		}
		s.inbox.Notify(inbox.Message{
			UserID: f.UserID, Kind: inbox.Reminder, Verb: "focus_done", WorkspaceID: f.WorkspaceID,
			Params: map[string]any{"title": title, "minutes": f.Spent},
			Link:   fmt.Sprintf("/w/%s/focus", f.WorkspaceID),
		})
		if s.hub != nil {
			s.hub.Publish(f.UserID, `{"type":"focus"}`)
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

// oldActivity drops workspace feed lines older than 90 days.
func (s *Scheduler) oldActivity() {
	s.db.Where("created_at < ?", time.Now().AddDate(0, 0, -90)).Delete(&models.Activity{})
}

// firstLine is a text's first line, at most n runes.
func firstLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(strings.TrimSpace(s)); len(r) > n {
		return string(r[:n]) + "…"
	}
	return strings.TrimSpace(s)
}

// habitDue reports whether a habit still wants doing on a local day: it
// counts that day (daily: its weekday; weekly: its target isn't met yet this
// week, Monday first) and it isn't done yet.
func habitDue(db *gorm.DB, hb models.Habit, local time.Time) bool {
	day := local.Format(planner.DateLayout)
	var done int64
	db.Model(&models.HabitCheck{}).Where("habit_id = ? AND date = ?", hb.ID, day).Count(&done)
	if done > 0 {
		return false
	}
	if hb.Schedule == "weekly" {
		wd := (int(local.Weekday()) + 6) % 7 // Monday 0
		monday := local.AddDate(0, 0, -wd).Format(planner.DateLayout)
		var week int64
		db.Model(&models.HabitCheck{}).Where("habit_id = ? AND date >= ? AND date <= ?", hb.ID, monday, day).Count(&week)
		return int(week) < hb.Target
	}
	wd := (int(local.Weekday()) + 6) % 7
	return len(hb.Days) == 7 && hb.Days[wd] == '1'
}
