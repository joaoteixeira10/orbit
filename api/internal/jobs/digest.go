package jobs

import (
	"sort"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/inbox"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"
)

// UseInbox gives the scheduler the notification service (reminders, the
// daily summary).
func (s *Scheduler) UseInbox(in *inbox.Service) *Scheduler {
	s.inbox = in
	return s
}

// digestItems caps the lines of a summary.
const digestItems = 12

// digests runs every minute and sends the daily summary to the users whose
// chosen time it is (in their zone) and who haven't had today's yet.
func (s *Scheduler) digests() {
	s.digestAt(time.Now().Truncate(time.Minute))
}

func (s *Scheduler) digestAt(now time.Time) {
	var prefs []models.UserPrefs
	s.db.Where("digest_enabled").Find(&prefs)
	for _, p := range prefs {
		zone := userZone(p)
		local := now.In(zone)
		today := local.Format(planner.DateLayout)
		if local.Format("15:04") != p.DigestTime || p.DigestSent == today {
			continue
		}
		s.db.Model(&models.UserPrefs{}).Where("id = ?", p.ID).Update("digest_sent", today)
		if params, ok := s.dayOf(p.UserID, today, zone); ok {
			s.inbox.Notify(inbox.Message{UserID: p.UserID, Kind: inbox.Digest, Verb: "daily", Params: params, Link: "/"})
		}
	}
}

// digestItem is one line of the summary: an overdue task, a task due today
// (with its time, if any) or an event today.
type digestItem struct {
	K  string `json:"k"`            // overdue | task | event
	T  string `json:"t"`            // title
	At string `json:"at,omitempty"` // HH:MM (local); empty = all day / no time
}

// dayOf gathers a user's day: their open tasks (assigned to them, or to no
// one) that are overdue or due today, and today's events in their
// workspaces. ok is false when there is nothing.
func (s *Scheduler) dayOf(uid, today string, zone *time.Location) (map[string]any, bool) {
	var wsIDs []string
	s.db.Model(&models.WorkspaceMember{}).
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id AND workspaces.archived_at IS NULL").
		Where("workspace_members.user_id = ?", uid).Pluck("workspace_members.workspace_id", &wsIDs)
	if len(wsIDs) == 0 {
		return nil, false
	}
	var tasks []models.Task
	s.db.Where("workspace_id IN ? AND completed_at IS NULL AND parent_id IS NULL AND due <= ? AND "+models.InOpenList+" AND "+models.MineOrUnassigned, wsIDs, today, models.AssigneeArg(uid)).
		Order("due, due_time, rank").Limit(100).Find(&tasks)
	var items []digestItem
	overdue := 0
	for _, t := range tasks {
		if *t.Due < today {
			overdue++
			items = append(items, digestItem{K: "overdue", T: t.Title})
			continue
		}
		it := digestItem{K: "task", T: t.Title}
		if t.DueTime != nil {
			it.At = *t.DueTime
		}
		items = append(items, it)
	}

	d, _ := time.ParseInLocation(planner.DateLayout, today, zone)
	from, to := d, d.AddDate(0, 0, 1)
	var events []digestItem
	var evs []models.Event
	s.db.Where("workspace_id IN ? AND starts_at < ? AND (until IS NULL OR until > ?)", wsIDs, to, from.AddDate(0, 0, -1)).Find(&evs)
	for _, ev := range evs {
		for _, o := range planner.Occurrences(ev, from, to) {
			it := digestItem{K: "event", T: ev.Title}
			if !ev.AllDay {
				it.At = o.In(zone).Format("15:04")
			}
			events = append(events, it)
		}
	}
	var ext []models.ExternalEvent
	s.db.Joins("JOIN calendar_subscriptions ON calendar_subscriptions.id = external_events.subscription_id").
		Where("calendar_subscriptions.workspace_id IN ? AND external_events.starts_at < ? AND external_events.ends_at > ?", wsIDs, to, from).
		Find(&ext)
	for _, e := range ext {
		it := digestItem{K: "event", T: e.Title}
		if !e.AllDay {
			it.At = e.StartsAt.In(zone).Format("15:04")
		}
		events = append(events, it)
	}
	// Events by time (all-day first).
	sort.SliceStable(events, func(i, j int) bool { return events[i].At < events[j].At })

	if len(items)+len(events) == 0 {
		return nil, false
	}
	all := append(events, items...)
	if len(all) > digestItems {
		all = all[:digestItems]
	}
	return map[string]any{"tasks": len(items), "overdue": overdue, "events": len(events), "items": all}, true
}
