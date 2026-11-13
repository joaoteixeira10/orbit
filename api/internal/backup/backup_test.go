package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbpkg "github.com/joaoteixeira10/orbit/api/internal/db"
	"github.com/joaoteixeira10/orbit/api/internal/keystore"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/uploads"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const masterKey = "backup-test-master-key-0123456789-abcdef"

func testDB(t *testing.T, path string) (*gorm.DB, *keystore.Sealer) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := gdb.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := dbpkg.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	s, err := dbpkg.OpenSealer(gdb, masterKey)
	if err != nil {
		t.Fatal(err)
	}
	return gdb, s
}

func seed(t *testing.T, gdb *gorm.DB, s *keystore.Sealer) {
	t.Helper()
	tok, _ := s.Seal([]byte("https://calendar.example/secret.ics"))
	if err := gdb.Create(&models.CalendarSubscription{WorkspaceID: "ws", Name: "School", URLSealed: tok}).Error; err != nil {
		t.Fatal(err)
	}
	gdb.Create(&models.User{Username: "olivia", PasswordHash: "x"})
}

// TestMasterKeyRoundTrip: a master-key archive restores, with the same
// ORBIT_MASTER_KEY, into a database whose sealed secrets open.
func TestMasterKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	gdb, s := testDB(t, filepath.Join(dir, "live.db"))
	seed(t, gdb, s)
	archive, err := Export(gdb, s, "", "1.2.3", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var env Envelope
	_ = json.Unmarshal(archive, &env)
	if env.Mode != modeMaster || env.AppVersion != "1.2.3" || env.KDFSalt == "" || strings.Contains(string(archive), "olivia") {
		t.Fatalf("envelope: %+v", env)
	}
	if _, _, err := Decrypt(archive, "", "another-master-key-0123456789-abcdefgh"); err == nil || !strings.Contains(err.Error(), "doesn't match") {
		t.Errorf("another master key: %v", err)
	}
	target := filepath.Join(dir, "restored", "orbit.db")
	if _, kept, err := Restore(archive, "", masterKey, target, false, time.Now()); err != nil || kept != "" {
		t.Fatalf("restore: %v %q", err, kept)
	}
	rdb, rs := testDB(t, target)
	var p models.CalendarSubscription
	if err := rdb.First(&p, "name = ?", "School").Error; err != nil {
		t.Fatal("subscription restored:", err)
	}
	if tok, err := rs.Unseal(p.URLSealed); err != nil || string(tok) != "https://calendar.example/secret.ics" {
		t.Errorf("the sealed calendar address opens after the restore: %q %v", tok, err)
	}
	var n int64
	rdb.Model(&models.User{}).Where("username = ?", "olivia").Count(&n)
	if n != 1 {
		t.Error("user restored")
	}
}

// TestPassphraseArchive: portable; the passphrase opens it; restoring over an
// existing database needs --force and keeps a copy.
func TestPassphraseArchive(t *testing.T) {
	dir := t.TempDir()
	gdb, s := testDB(t, filepath.Join(dir, "live.db"))
	seed(t, gdb, s)
	if _, err := Export(gdb, s, "short", "1", time.Now()); err == nil {
		t.Error("a short passphrase is refused")
	}
	archive, err := Export(gdb, s, "correct horse battery", "1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	env, _ := Open(archive)
	if !env.NeedsPassphrase() {
		t.Fatal("passphrase mode")
	}
	if _, _, err := Decrypt(archive, "", masterKey); err == nil {
		t.Error("needs the passphrase, not the master key")
	}
	if _, _, err := Decrypt(archive, "wrong horse battery", ""); err == nil || !strings.Contains(err.Error(), "wrong passphrase") {
		t.Errorf("wrong passphrase: %v", err)
	}
	target := filepath.Join(dir, "target.db")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Restore(archive, "correct horse battery", "", target, false, time.Now()); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("existing file without --force: %v", err)
	}
	_, kept, err := Restore(archive, "correct horse battery", "", target, true, time.Now())
	if err != nil || kept == "" {
		t.Fatalf("force: %v %q", err, kept)
	}
	if b, _ := os.ReadFile(kept); string(b) != "old" {
		t.Error("the old database is kept")
	}
	if b, _ := os.ReadFile(target); !strings.HasPrefix(string(b), "SQLite format 3") {
		t.Error("restored database")
	}
	if _, _, err := Decrypt([]byte(`{"format":"pki-management-backup","version":1}`), "", masterKey); err == nil {
		t.Error("another program's archive is refused")
	}
}

// TestScheduleDir: due runs write to the directory and prune to Keep; not due
// = nothing; bad schedules are refused.
func TestScheduleDir(t *testing.T) {
	dir := t.TempDir()
	gdb, s := testDB(t, filepath.Join(dir, "live.db"))
	out := filepath.Join(dir, "backups")
	if err := (Schedule{Enabled: true, IntervalHours: 24, Dir: "relative"}).Validate(); err == nil {
		t.Error("a relative directory is refused")
	}
	if err := (Schedule{Enabled: true, IntervalHours: 24, Dest: "s3"}).Validate(); err == nil {
		t.Error("S3 needs a bucket")
	}
	if err := SaveSchedule(gdb, Schedule{Enabled: true, IntervalHours: 1, Dest: "dir", Dir: out, Keep: 2}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		r := RunDueScheduled(gdb, s, "1", now.Add(time.Duration(i)*time.Hour))
		if r == nil || !r.OK {
			t.Fatalf("run %d: %+v", i, r)
		}
	}
	if r := RunDueScheduled(gdb, s, "1", now.Add(3*time.Hour+10*time.Minute)); r != nil {
		t.Error("not due within the interval")
	}
	files, _ := filepath.Glob(filepath.Join(out, filePrefix+"*.json"))
	if len(files) != 2 {
		t.Errorf("keep 2: %v", files)
	}
	if last := LoadLastRun(gdb); !last.OK || !strings.HasPrefix(last.Where, out) {
		t.Errorf("last run: %+v", last)
	}
}

// TestScheduleCopiesFiles: with files kept in a directory (so not in the
// archive), each run copies the ones not copied yet beside the archives.
func TestScheduleCopiesFiles(t *testing.T) {
	dir := t.TempDir()
	gdb, s := testDB(t, filepath.Join(dir, "live.db"))
	uploads.Use(uploads.DirStore{Root: filepath.Join(dir, "files")})
	t.Cleanup(func() { uploads.Use(uploads.DBStore{}) })
	add := func(body string) {
		u := models.Upload{OwnerID: "u", Target: "note", TargetID: "n", Name: body + ".txt"}
		if err := uploads.Create(gdb, &u, []byte(body), 0); err != nil {
			t.Fatal(err)
		}
	}
	add("one")
	add("one") // the same bytes: one file
	add("two")
	out := filepath.Join(dir, "backups")
	sch := Schedule{Enabled: true, IntervalHours: 1, Dest: "dir", Dir: out, Keep: 2}
	now := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	if r := RunNow(gdb, s, sch, "1", now); !r.OK || r.Files != 2 {
		t.Fatalf("first run: %+v", r)
	}
	add("three")
	if r := RunNow(gdb, s, sch, "1", now.Add(time.Hour)); !r.OK || r.Files != 1 {
		t.Fatalf("second run copies only the new one: %+v", r)
	}
	copied, _ := filepath.Glob(filepath.Join(out, "files", "*", "*"))
	if len(copied) != 3 {
		t.Fatalf("copied: %v", copied)
	}
	b, err := os.ReadFile(filepath.Join(out, "files", uploads.Sum([]byte("two"))[:2], uploads.Sum([]byte("two"))))
	if err != nil || string(b) != "two" {
		t.Fatalf("a copy: %q %v", b, err)
	}
}
