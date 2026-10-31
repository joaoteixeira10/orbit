package db

import (
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestMigrateAssignees: a task's single assignee (the old column) becomes
// its assignees list, once.
func TestMigrateAssignees(t *testing.T) {
	g, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := g.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := g.Exec("CREATE TABLE tasks (id text PRIMARY KEY, workspace_id text, list_id text, column_id text, title text, assignee_id text)").Error; err != nil {
		t.Fatal(err)
	}
	g.Exec("INSERT INTO tasks (id, workspace_id, list_id, column_id, title, assignee_id) VALUES ('t1','w','l','c','Mine','u1'), ('t2','w','l','c','Nobody''s',NULL)")
	if err := Migrate(g); err != nil {
		t.Fatal(err)
	}
	var t1, t2 models.Task
	g.First(&t1, "id = ?", "t1")
	g.First(&t2, "id = ?", "t2")
	if !t1.AssignedTo("u1") || len(t2.AssigneeList()) != 0 {
		t.Fatalf("after: %q, %q", t1.Assignees, t2.Assignees)
	}
	// Unassigning afterwards sticks: a second migration doesn't bring u1 back.
	g.Model(&models.Task{}).Where("id = ?", "t1").Update("assignees", "[]")
	if err := Migrate(g); err != nil {
		t.Fatal(err)
	}
	g.First(&t1, "id = ?", "t1")
	if len(t1.AssigneeList()) != 0 {
		t.Fatalf("came back: %q", t1.Assignees)
	}
}

// TestMigrateListFolders: once, every list gets its folder and Files turns
// on where things live in folders (not in a habits-only workspace); an
// unjoined folder afterwards stays unjoined.
func TestMigrateListFolders(t *testing.T) {
	g, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := g.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := g.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	g.Create(&models.Workspace{Base: models.Base{ID: "w1"}, Name: "Work", Modules: `["home","tasks"]`})
	g.Create(&models.Workspace{Base: models.Base{ID: "w2"}, Name: "Habits", Modules: `["home","habits"]`})
	g.Create(&models.TaskList{Base: models.Base{ID: "l1"}, WorkspaceID: "w1", Name: "Launch", Color: "#ff9500"})
	if err := Migrate(g); err != nil {
		t.Fatal(err)
	}
	var f models.Folder
	if g.First(&f, "list_id = ?", "l1").Error != nil || f.Name != "Launch" || f.WorkspaceID != "w1" {
		t.Fatalf("the list's folder: %+v", f)
	}
	var w1, w2 models.Workspace
	g.First(&w1, "id = ?", "w1")
	g.First(&w2, "id = ?", "w2")
	if w1.Modules != `["home","tasks","files"]` || w2.Modules != `["home","habits"]` {
		t.Fatalf("modules: %s / %s", w1.Modules, w2.Modules)
	}
	g.Model(&models.Folder{}).Where("id = ?", f.ID).Update("list_id", "")
	if err := Migrate(g); err != nil {
		t.Fatal(err)
	}
	var n int64
	g.Model(&models.Folder{}).Where("list_id = ?", "l1").Count(&n)
	if n != 0 {
		t.Fatal("a second start joined the folder again")
	}
}
