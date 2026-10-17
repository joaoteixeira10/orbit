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
