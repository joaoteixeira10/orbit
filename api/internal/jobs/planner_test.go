package jobs

import (
	"strings"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

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
	s := &Scheduler{db: db, log: zap.NewNop(), hub: hub}

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
