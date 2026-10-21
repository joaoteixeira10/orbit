package search

import (
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func db(t *testing.T) *gorm.DB {
	t.Helper()
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
	return g
}

func kinds(h []Hit) map[string]string {
	m := map[string]string{}
	for _, x := range h {
		m[x.Kind] = x.Ref
	}
	return m
}

// TestIndex: rows made before Setup are indexed by the rebuild, later ones
// by the triggers (insert, update, delete); accents and prefixes match;
// only the workspaces given (and the user's own notes) are searched.
func TestIndex(t *testing.T) {
	g := db(t)
	g.Create(&models.Page{WorkspaceID: "w1", Title: "Plano de ação", ContentText: "Reunião sobre orçamento"})
	if !Setup(g) {
		t.Fatal("no FTS5")
	}
	task := models.Task{WorkspaceID: "w1", ListID: "l", ColumnID: "c", Title: "Quarterly report", Notes: "numbers"}
	g.Create(&task)
	g.Create(&models.Note{WorkspaceID: "", OwnerID: "u1", Title: "Mine", PlainText: "secret report"})
	g.Create(&models.Note{WorkspaceID: "", OwnerID: "u2", Title: "Theirs", PlainText: "other report"})
	g.Create(&models.Event{WorkspaceID: "w2", Title: "Dentist", Location: "Rua Augusta", StartsAt: time.Now(), EndsAt: time.Now()})
	g.Create(&models.TaskComment{TaskID: task.ID, WorkspaceID: "w1", AuthorID: "u1", Body: "Approved by finance"})

	find := func(q string, ws ...string) map[string]string {
		h, err := Find(g, q, ws, "u1", 20)
		if err != nil {
			t.Fatal(err)
		}
		return kinds(h)
	}
	if k := find("acao", "w1"); k["page"] == "" {
		t.Fatalf("accents: %v", k)
	}
	if k := find("orcam", "w1"); k["page"] == "" {
		t.Fatalf("prefix in the body: %v", k)
	}
	k := find("report", "w1")
	if k["task"] != task.ID || k["note"] == "" || len(k) != 2 {
		t.Fatalf("report in w1 + my notes: %v", k)
	}
	if k := find("augusta", "w1"); len(k) != 0 {
		t.Fatalf("another workspace's event: %v", k)
	}
	if k := find("finance", "w1"); k["comment"] != task.ID {
		t.Fatalf("a comment opens its task: %v", k)
	}
	// Changed, then deleted.
	g.Model(&task).Update("title", "Annual summary")
	if k := find("quarterly", "w1"); len(k) != 0 {
		t.Fatalf("old title still found: %v", k)
	}
	if k := find("annual", "w1"); k["task"] == "" {
		t.Fatalf("new title not found: %v", k)
	}
	g.Delete(&task)
	if k := find("annual", "w1"); len(k) != 0 {
		t.Fatalf("deleted task still found: %v", k)
	}
	if Match(`"OR" -x (a`) != `"OR"* "x"* "a"*` {
		t.Fatalf("match %q", Match(`"OR" -x (a`))
	}
}
