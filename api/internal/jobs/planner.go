package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"go.uber.org/zap"
)

// TrashDays is how long a trashed page waits before it is deleted for good.
const TrashDays = 30

// UsePlanner turns on the planner's background work: refreshing subscribed
// calendars, reminders and emptying old trash.
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
		{"@daily", s.emptyTrash, "trash"},
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

// reminder is one message for one user.
type reminder struct {
	userID, title, body, ws, kind, id string
}

// reminders runs every minute and sends what falls due in this minute: tasks
// with a due time (to their assignee, else their creator) and event
// occurrences with a reminder (to every member of the workspace).
func (s *Scheduler) reminders() {
	s.remindAt(time.Now().Truncate(time.Minute))
}

func (s *Scheduler) remindAt(minute time.Time) {
	end := minute.Add(time.Minute)
	var out []reminder
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
		out = append(out, reminder{userID: uid, title: t.Title, body: "Due now", ws: t.WorkspaceID, kind: "task", id: t.ID})
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
			body := "Starting now"
			if lead > 0 {
				body = fmt.Sprintf("In %s", humanLead(lead))
			}
			var members []string
			s.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ?", ev.WorkspaceID).Pluck("user_id", &members)
			for _, uid := range members {
				out = append(out, reminder{userID: uid, title: ev.Title, body: body, ws: ev.WorkspaceID, kind: "event", id: ev.ID})
			}
		}
	}

	for _, r := range out {
		s.deliver(r, prefs(r.userID))
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

// deliver shows a reminder in the user's open sessions and, if they turned
// notifications on, sends it through their chosen channels.
func (s *Scheduler) deliver(r reminder, p models.UserPrefs) {
	if s.hub != nil {
		b, _ := json.Marshal(map[string]string{"type": "reminder", "title": r.title, "body": r.body, "ws": r.ws, "kind": r.kind, "id": r.id})
		s.hub.Publish(r.userID, string(b))
	}
	if !p.NotifyEvents || s.notifier == nil {
		return
	}
	var channels []string
	_ = json.Unmarshal([]byte(p.NotifyChannels), &channels)
	if len(channels) == 0 {
		return
	}
	var u models.User
	if s.db.Select("id", "email", "disabled").First(&u, "id = ?", r.userID).Error != nil || u.Disabled {
		return
	}
	go s.notifier.Send(channels, u.Email, "Orbit: "+r.title, r.title+" — "+r.body)
}
