package jobs

import (
	"strings"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/events"
	"github.com/joaoteixeira10/orbit/api/internal/inbox"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func plannerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	return db
}

// drain returns what a hub subscription received so far.
func drain(ch <-chan string) []string {
	var out []string
	for {
		select {
		case m := <-ch:
			out = append(out, m)
		default:
			return out
		}
	}
}

// TestReminders: a task due at 09:00 is reminded in its owner's zone at that
// minute and not the next; an event occurrence is reminded its lead time
// before it starts, to every member.
func TestReminders(t *testing.T) {
	db := plannerDB(t)
	hub := events.New()
	s := &Scheduler{db: db, log: zap.NewNop(), hub: hub, inbox: inbox.New(db, hub, nil, nil, zap.NewNop(), "")}

	db.Create(&models.UserPrefs{UserID: "u1", Timezone: "local", DeviceTZ: "Europe/Lisbon"})
	db.Create(&models.WorkspaceMember{WorkspaceID: "w", UserID: "u1", Role: models.WSOwner})
	db.Create(&models.WorkspaceMember{WorkspaceID: "w", UserID: "u2", Role: models.WSViewer})
	due, at := "2026-10-13", "09:00"
	db.Create(&models.Task{WorkspaceID: "w", ListID: "l", ColumnID: "c", Title: "Essay", Due: &due, DueTime: &at, CreatedBy: "u1"})
	ten := 10
	start := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	db.Create(&models.Event{WorkspaceID: "w", Title: "Class", StartsAt: start, EndsAt: start.Add(time.Hour), RemindMinutes: &ten})

	u1, stop1 := hub.Subscribe("u1")
	defer stop1()
	u2, stop2 := hub.Subscribe("u2")
	defer stop2()

	// 09:00 in Lisbon (summer time) is 08:00Z.
	s.remindAt(time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC))
	if got := drain(u1); len(got) != 1 || !strings.Contains(got[0], "Essay") {
		t.Errorf("task reminder at 08:00Z: %v", got)
	}
	s.remindAt(time.Date(2026, 10, 13, 8, 1, 0, 0, time.UTC))
	if got := drain(u1); len(got) != 0 {
		t.Errorf("reminded twice: %v", got)
	}
	// The class at 09:30Z, 10 minutes ahead: 09:20Z, to both members.
	s.remindAt(time.Date(2026, 10, 13, 9, 20, 0, 0, time.UTC))
	for name, ch := range map[string]<-chan string{"u1": u1, "u2": u2} {
		if got := drain(ch); len(got) != 1 || !strings.Contains(got[0], "Class") || !strings.Contains(got[0], "10 min") {
			t.Errorf("%s event reminder: %v", name, got)
		}
	}
	// A note's reminder goes to whoever set it, once.
	when := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	note := models.Note{OwnerID: "u2", Title: "Call the dentist", RemindAt: &when, RemindBy: "u2"}
	db.Create(&note)
	s.remindAt(time.Date(2026, 10, 13, 9, 59, 0, 0, time.UTC))
	if got := drain(u2); len(got) != 0 {
		t.Errorf("note reminded early: %v", got)
	}
	s.remindAt(when)
	if got := drain(u2); len(got) != 1 || !strings.Contains(got[0], "Call the dentist") || !strings.Contains(got[0], "/notes?note=") {
		t.Errorf("note reminder: %v", got)
	}
	s.remindAt(when.Add(time.Minute))
	if got := drain(u2); len(got) != 0 {
		t.Errorf("note reminded twice: %v", got)
	}
	// Each reminder is also in the inbox.
	var n int64
	db.Model(&models.Notification{}).Where("kind = ?", inbox.Reminder).Count(&n)
	if n != 4 {
		t.Errorf("reminders in the inbox: %d, want 4", n)
	}
}

// TestDigest: the summary goes out at the chosen local time, once a day,
// with the day's events and the tasks due (overdue ones first marked), and
// not at all when the day is empty.
func TestDigest(t *testing.T) {
	db := plannerDB(t)
	s := &Scheduler{db: db, log: zap.NewNop(), inbox: inbox.New(db, nil, nil, nil, zap.NewNop(), "")}
	db.Create(&models.Workspace{Base: models.Base{ID: "w"}, Name: "Escola"})
	db.Create(&models.WorkspaceMember{WorkspaceID: "w", UserID: "u1", Role: models.WSOwner})
	db.Create(&models.WorkspaceMember{WorkspaceID: "w", UserID: "u2", Role: models.WSEditor})
	db.Create(&models.UserPrefs{UserID: "u1", Timezone: "Europe/Lisbon", DigestEnabled: true, DigestTime: "08:00"})
	db.Create(&models.UserPrefs{UserID: "u2", Timezone: "Europe/Lisbon", DigestEnabled: true, DigestTime: "08:00"})
	late, today, at := "2026-10-12", "2026-10-13", "14:00"
	db.Create(&models.Task{WorkspaceID: "w", ListID: "l", ColumnID: "c", Title: "Essay", Due: &late, CreatedBy: "u1"})
	db.Create(&models.Task{WorkspaceID: "w", ListID: "l", ColumnID: "c", Title: "Lab report", Due: &today, DueTime: &at, CreatedBy: "u1"})
	db.Create(&models.Task{WorkspaceID: "w", ListID: "l", ColumnID: "c", Title: "Bob's", Due: &today, Assignees: `["u2"]`, CreatedBy: "u1"})
	start := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	db.Create(&models.Event{WorkspaceID: "w", Title: "Class", StartsAt: start, EndsAt: start.Add(time.Hour)})

	// 07:59 in Lisbon (06:59Z): nothing yet; 08:00 (07:00Z): sent; again: not.
	s.digestAt(time.Date(2026, 10, 13, 6, 59, 0, 0, time.UTC))
	s.digestAt(time.Date(2026, 10, 13, 7, 0, 0, 0, time.UTC))
	s.digestAt(time.Date(2026, 10, 13, 7, 0, 0, 0, time.UTC))
	var got []models.Notification
	db.Where("kind = ?", inbox.Digest).Find(&got)
	if len(got) != 2 {
		t.Fatalf("summaries: %d, want 2 (one each)", len(got))
	}
	for _, n := range got {
		title, body := inbox.Render("en", inbox.ToView(n))
		switch n.UserID {
		case "u1":
			if title != "Your day: 2 tasks, 1 event" || !strings.Contains(body, "◷ 10:30 Class") || !strings.Contains(body, "⚠ Essay") || !strings.Contains(body, "○ Lab report (14:00)") || strings.Contains(body, "Bob's") {
				t.Errorf("u1: %q / %q", title, body)
			}
		case "u2":
			if !strings.Contains(body, "Bob's") {
				t.Errorf("u2: %q / %q", title, body)
			}
		}
	}
	// An empty day sends nothing (but counts as done).
	db.Where("1 = 1").Delete(&models.Task{})
	db.Where("1 = 1").Delete(&models.Event{})
	s.digestAt(time.Date(2026, 10, 14, 7, 0, 0, 0, time.UTC))
	var n int64
	db.Model(&models.Notification{}).Where("kind = ?", inbox.Digest).Count(&n)
	if n != 2 {
		t.Errorf("an empty day sent a summary")
	}
}

// TestEmptyTrash: pages trashed over 30 days ago go; newer ones stay.
func TestEmptyTrash(t *testing.T) {
	db := plannerDB(t)
	s := &Scheduler{db: db, log: zap.NewNop()}
	old, recent := time.Now().AddDate(0, 0, -31), time.Now().AddDate(0, 0, -2)
	db.Create(&models.Page{WorkspaceID: "w", Title: "old", TrashedAt: &old})
	db.Create(&models.Page{WorkspaceID: "w", Title: "recent", TrashedAt: &recent})
	db.Create(&models.Page{WorkspaceID: "w", Title: "live"})
	s.emptyTrash()
	var titles []string
	db.Model(&models.Page{}).Order("title").Pluck("title", &titles)
	if strings.Join(titles, ",") != "live,recent" {
		t.Errorf("left: %v", titles)
	}
}

// TestHabitReminders: at its time, on a day it counts and not done yet; a
// weekly one until its target for the week is met.
func TestHabitReminders(t *testing.T) {
	db := plannerDB(t)
	hub := events.New()
	s := &Scheduler{db: db, log: zap.NewNop(), hub: hub, inbox: inbox.New(db, hub, nil, nil, zap.NewNop(), "")}
	db.Create(&models.UserPrefs{UserID: "u1", Timezone: "Europe/Lisbon"})
	// Tuesday 1 December 2026, 21:00 in Lisbon (UTC in winter).
	at := time.Date(2026, 12, 1, 21, 0, 0, 0, time.UTC)
	weekdays := models.Habit{WorkspaceID: "w", OwnerID: "u1", Title: "Read", Schedule: "daily", Days: "1111100", Target: 1, RemindAt: "21:00"}
	weekends := models.Habit{WorkspaceID: "w", OwnerID: "u1", Title: "Hike", Schedule: "daily", Days: "0000011", Target: 1, RemindAt: "21:00"}
	weekly := models.Habit{WorkspaceID: "w", OwnerID: "u1", Title: "Gym", Schedule: "weekly", Days: "1111111", Target: 2, RemindAt: "21:00"}
	for _, hb := range []*models.Habit{&weekdays, &weekends, &weekly} {
		db.Create(hb)
	}
	ch, stop := hub.Subscribe("u1")
	defer stop()
	s.remindAt(at)
	got := strings.Join(drain(ch), "\n")
	if !strings.Contains(got, "Read") || strings.Contains(got, "Hike") || !strings.Contains(got, "Gym") {
		t.Fatalf("first: %s", got)
	}
	// Read done today; Gym twice this week (Monday and today): nothing.
	db.Create(&models.HabitCheck{HabitID: weekdays.ID, Date: "2026-12-01"})
	db.Create(&models.HabitCheck{HabitID: weekly.ID, Date: "2026-11-30"})
	db.Create(&models.HabitCheck{HabitID: weekly.ID, Date: "2026-12-01"})
	s.remindAt(at.AddDate(0, 0, 0))
	if got := drain(ch); len(got) != 0 {
		t.Fatalf("reminded though done: %v", got)
	}
	// Wednesday: Read again; Gym's week is done.
	s.remindAt(at.AddDate(0, 0, 1))
	got = strings.Join(drain(ch), "\n")
	if !strings.Contains(got, "Read") || strings.Contains(got, "Gym") {
		t.Fatalf("wednesday: %s", got)
	}
}
