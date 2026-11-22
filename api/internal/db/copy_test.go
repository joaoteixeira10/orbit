package db

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

func copyDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := g.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	return g
}

// TestCopy: a whole Orbit into another database — every row as it was (ids,
// times, a false kept false where the column's default is true, a file's
// bytes, numbered rows), found by search there; an Orbit already there is
// replaced only when asked.
func TestCopy(t *testing.T) {
	src := copyDB(t, "copysrc")
	if err := Migrate(src); err != nil {
		t.Fatal(err)
	}
	made := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	u := models.User{Username: "ana"}
	u.CreatedAt = made
	src.Create(&u)
	src.Create(&models.UserPrefs{UserID: u.ID, Timezone: "Europe/Lisbon"})
	src.Model(&models.UserPrefs{}).Where("user_id = ?", u.ID).Update("holiday_effects", false)
	ws := models.Workspace{Name: "Team"}
	src.Create(&ws)
	src.Create(&models.Page{WorkspaceID: ws.ID, Title: "Onboarding", Content: "Welcome aboard", ContentText: "Welcome aboard"})
	src.Create(&models.Blob{SHA256: "ab", Data: bytes.Repeat([]byte{7}, 100000)})
	src.Create(&models.CollabUpdate{DocKey: "page:1", Seq: 1, Data: []byte("x")})

	dst := copyDB(t, "copydst")
	var out bytes.Buffer
	if err := Copy(src, dst, false, &out); err != nil {
		t.Fatalf("copy: %v\n%s", err, out.String())
	}
	var got models.User
	if err := dst.Preload("Prefs").First(&got, "id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Username != "ana" || !got.CreatedAt.Equal(made) || got.Prefs.Timezone != "Europe/Lisbon" || got.Prefs.HolidayEffects {
		t.Errorf("user: %+v (prefs %+v)", got, got.Prefs)
	}
	var blob models.Blob
	dst.First(&blob, "sha256 = ?", "ab")
	if len(blob.Data) != 100000 {
		t.Errorf("file bytes: %d", len(blob.Data))
	}
	var ids []string
	dst.Raw(`SELECT id FROM search_index WHERE search_index MATCH 'aboard'`).Scan(&ids)
	if len(ids) != 1 {
		t.Errorf("search on the copy: %v", ids)
	}
	// A new numbered row goes on after the copied ones.
	next := models.CollabUpdate{DocKey: "page:1", Seq: 2, Data: []byte("y")}
	if err := dst.Create(&next).Error; err != nil || next.ID < 2 {
		t.Errorf("numbering: %v %d", err, next.ID)
	}

	// Again: refused, unless asked to replace it.
	if err := Copy(src, dst, false, &out); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("into an Orbit: %v", err)
	}
	if err := Copy(src, dst, true, &out); err != nil {
		t.Fatalf("forced: %v", err)
	}
	var n int64
	dst.Model(&models.CollabUpdate{}).Count(&n)
	if n != 1 {
		t.Errorf("after replacing: %d numbered rows", n)
	}
}

// TestCopyPostgres: SQLite into PostgreSQL and back, nothing lost on the
// way (only with ORBIT_TEST_PG, a PostgreSQL address to use up).
func TestCopyPostgres(t *testing.T) {
	pg := os.Getenv("ORBIT_TEST_PG")
	if pg == "" {
		t.Skip("ORBIT_TEST_PG not set")
	}
	src := copyDB(t, "pgsrc")
	if err := Migrate(src); err != nil {
		t.Fatal(err)
	}
	u := models.User{Username: "ana"}
	src.Create(&u)
	src.Create(&models.UserPrefs{UserID: u.ID, Timezone: "Europe/Lisbon"})
	src.Model(&models.UserPrefs{}).Where("user_id = ?", u.ID).Update("holiday_effects", false)
	ws := models.Workspace{Name: "Equipa ção"}
	src.Create(&ws)
	src.Create(&models.Page{WorkspaceID: ws.ID, Title: "Onboarding", Content: "Bem-vindo a bordo", ContentText: "Bem-vindo a bordo"})
	src.Create(&models.Blob{SHA256: "ab", Data: bytes.Repeat([]byte{0, 255}, 50000)})
	src.Create(&models.CollabUpdate{DocKey: "page:1", Seq: 1, Data: []byte("x")})

	mid, err := OpenAt("postgres", pg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Copy(src, mid, true, &out); err != nil {
		t.Fatalf("into postgres: %v\n%s", err, out.String())
	}
	next := models.CollabUpdate{DocKey: "page:1", Seq: 2, Data: []byte("y")}
	if err := mid.Create(&next).Error; err != nil || next.ID < 2 {
		t.Errorf("postgres numbering: %v %d", err, next.ID)
	}
	mid.Delete(&next)
	back := copyDB(t, "pgback")
	if err := Copy(mid, back, false, &out); err != nil {
		t.Fatalf("back to sqlite: %v\n%s", err, out.String())
	}
	var p models.UserPrefs
	back.First(&p, "user_id = ?", u.ID)
	var w models.Workspace
	back.First(&w, "id = ?", ws.ID)
	var b models.Blob
	back.First(&b, "sha256 = ?", "ab")
	if p.HolidayEffects || p.Timezone != "Europe/Lisbon" || w.Name != "Equipa ção" || !bytes.Equal(b.Data, bytes.Repeat([]byte{0, 255}, 50000)) {
		t.Errorf("after the round trip: %+v / %q / %d bytes", p, w.Name, len(b.Data))
	}
	var hits int64
	mid.Raw(`SELECT count(*) FROM search_index WHERE kind = 'page'`).Scan(&hits)
	if hits != 1 {
		t.Errorf("postgres search index: %d", hits)
	}
}
