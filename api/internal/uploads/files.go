package uploads

import (
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

// Files in folders. A file is one of its own (added in Files, in a folder)
// or an attachment (on a page, task or note: it shows where that is — a
// task's in its list's folder, a page's in the page's). A file of the same
// name added to the same place is its next version. Deleting puts a file
// (all its versions) in the trash for TrashDays; a page, task or note going
// for good sends its files there too, as files of their own, where they
// showed.

// File is the target of a file of its own.
const File = "file"

// Me is a personal file's TargetID (Quick Notes' and Quick Draw's drive).
const Me = "me"

// TrashDays is how long a file stays in the trash.
const TrashDays = 30

// Shown is the condition for the files shown: the newest version, not in
// the trash.
const Shown = "replaced_at IS NULL AND trashed_at IS NULL"

// LineageOf is the id shared by a file's versions.
func LineageOf(u models.Upload) string {
	if u.Lineage != "" {
		return u.Lineage
	}
	return u.ID
}

// Add stores a file as Create does; one of the same name in the same place
// (the same page, task or note; for a file of its own, the same folder)
// becomes its new version.
func Add(db *gorm.DB, u *models.Upload, data []byte, quota int64) error {
	u.Name = CleanName(u.Name)
	q := db.Where("workspace_id = ? AND target = ? AND target_id = ? AND name = ? AND "+Shown, u.WorkspaceID, u.Target, u.TargetID, u.Name)
	if u.Target == File {
		q = q.Where("folder_id = ?", u.FolderID)
		if u.WorkspaceID == "" {
			q = q.Where("owner_id = ?", u.OwnerID)
		}
	}
	var prev models.Upload
	if q.Limit(1).Find(&prev).RowsAffected == 0 {
		return Create(db, u, data, quota)
	}
	u.Lineage = LineageOf(prev)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := Create(tx, u, data, quota); err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&models.Upload{}).Where("lineage = ? AND id <> ? AND replaced_at IS NULL", u.Lineage, u.ID).Update("replaced_at", &now).Error
	})
}

// Newest is the current version of a file's lineage (ok false: none).
func Newest(db *gorm.DB, u models.Upload) (models.Upload, bool) {
	var cur models.Upload
	ok := db.Where("lineage = ? AND replaced_at IS NULL", LineageOf(u)).Order("created_at DESC").Limit(1).Find(&cur).RowsAffected == 1
	return cur, ok
}

// Versions are a file's versions, newest first.
func Versions(db *gorm.DB, u models.Upload) []models.Upload {
	var out []models.Upload
	db.Where("lineage = ?", LineageOf(u)).Order("created_at DESC").Find(&out)
	return out
}

// MakeCurrent makes an older version the newest again (a new version with
// its bytes, which are kept already).
func MakeCurrent(db *gorm.DB, old models.Upload, by string) (models.Upload, error) {
	u := models.Upload{
		WorkspaceID: old.WorkspaceID, OwnerID: by, Target: old.Target, TargetID: old.TargetID, Name: old.Name, Type: old.Type,
		Size: old.Size, SHA256: old.SHA256, FolderID: old.FolderID, Lineage: LineageOf(old),
	}
	if cur, ok := Newest(db, old); ok {
		u.Name, u.FolderID = cur.Name, cur.FolderID
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&models.Upload{}).Where("lineage = ? AND replaced_at IS NULL", u.Lineage).Update("replaced_at", &now).Error; err != nil {
			return err
		}
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		var text models.UploadText
		if tx.Limit(1).Find(&text, "id = ?", old.ID).RowsAffected == 1 && text.Text != "" {
			return SetText(tx, u.ID, text.Text)
		}
		return nil
	})
	return u, err
}

// Trash puts a file (all its versions) in the trash; the folder it showed
// in is kept, to come back to.
func Trash(db *gorm.DB, u models.Upload, folder string) error {
	now := time.Now()
	return db.Model(&models.Upload{}).Where("lineage = ? AND trashed_at IS NULL", LineageOf(u)).Updates(map[string]any{"trashed_at": &now, "folder_id": folder}).Error
}

// Untrash takes a file out of the trash. One whose page, task or note is
// gone comes back as a file of its own.
func Untrash(db *gorm.DB, u models.Upload) error {
	up := map[string]any{"trashed_at": nil}
	if u.Target != File && !targetExists(db, u.Target, u.TargetID) {
		tid := u.WorkspaceID
		if tid == "" {
			tid = Me
		}
		up["target"], up["target_id"] = File, tid
	}
	return db.Model(&models.Upload{}).Where("lineage = ?", LineageOf(u)).Updates(up).Error
}

func targetExists(db *gorm.DB, target, id string) bool {
	var n int64
	switch target {
	case Page:
		db.Model(&models.Page{}).Where("id = ?", id).Count(&n)
	case Task:
		db.Model(&models.Task{}).Where("id = ?", id).Count(&n)
	case Note:
		db.Model(&models.Note{}).Where("id = ?", id).Count(&n)
	}
	return n > 0
}

// EmptyTrash deletes the files in the trash since before.
func EmptyTrash(db *gorm.DB, before time.Time) error {
	return Delete(db, "trashed_at IS NOT NULL AND trashed_at < ?", before)
}

// Place is where a file shows: its folder, and what it's on.
type Place struct {
	FolderID string `json:"folder_id"`
	OnKind   string `json:"on_kind,omitempty"` // page | task | note
	OnID     string `json:"on_id,omitempty"`
	OnTitle  string `json:"on_title,omitempty"`
}

// Places says where each file shows (by its id).
func Places(db *gorm.DB, files []models.Upload) map[string]Place {
	ids := map[string][]string{}
	for _, u := range files {
		if u.Target != File {
			ids[u.Target] = append(ids[u.Target], u.TargetID)
		}
	}
	on := map[string]Place{} // target + id → its place
	if len(ids[Task]) > 0 {
		var tasks []models.Task
		db.Select("id", "title", "list_id").Where("id IN ?", ids[Task]).Find(&tasks)
		lists := []string{}
		for _, t := range tasks {
			lists = append(lists, t.ListID)
		}
		var folders []models.Folder
		db.Select("id", "list_id").Where("list_id IN ?", append(lists, "-")).Find(&folders)
		of := map[string]string{}
		for _, f := range folders {
			of[f.ListID] = f.ID
		}
		for _, t := range tasks {
			on[Task+t.ID] = Place{FolderID: of[t.ListID], OnKind: Task, OnID: t.ID, OnTitle: t.Title}
		}
	}
	if len(ids[Page]) > 0 {
		var pages []models.Page
		db.Select("id", "title", "folder_id").Where("id IN ?", ids[Page]).Find(&pages)
		for _, p := range pages {
			on[Page+p.ID] = Place{FolderID: p.FolderID, OnKind: Page, OnID: p.ID, OnTitle: p.Title}
		}
	}
	if len(ids[Note]) > 0 {
		var notes []models.Note
		db.Select("id", "title", "body").Where("id IN ?", ids[Note]).Find(&notes)
		for _, n := range notes {
			on[Note+n.ID] = Place{OnKind: Note, OnID: n.ID, OnTitle: noteTitle(n)}
		}
	}
	out := make(map[string]Place, len(files))
	for _, u := range files {
		if u.Target == File || u.TrashedAt != nil {
			out[u.ID] = Place{FolderID: u.FolderID}
			continue
		}
		out[u.ID] = on[u.Target+u.TargetID] // gone: at the top
	}
	return out
}

func noteTitle(n models.Note) string {
	if t := strings.TrimSpace(n.Title); t != "" {
		return t
	}
	line, _, _ := strings.Cut(strings.TrimSpace(n.Body), "\n")
	if r := []rune(line); len(r) > 60 {
		line = string(r[:60]) + "…"
	}
	return line
}

// TrashFor puts the files on some pages, tasks or notes going for good in
// the trash, as files of their own, where they showed, saying what they
// were on.
func TrashFor(db *gorm.DB, target string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var files []models.Upload
	db.Where("target = ? AND target_id IN ?", target, ids).Find(&files)
	if len(files) == 0 {
		return nil
	}
	places := Places(db, files)
	now := time.Now()
	for _, u := range files {
		p := places[u.ID]
		tid := u.WorkspaceID
		if tid == "" {
			tid = Me
		}
		up := map[string]any{"target": File, "target_id": tid, "folder_id": p.FolderID, "was_kind": target, "was_title": truncate(p.OnTitle, 300)}
		if u.TrashedAt == nil {
			up["trashed_at"] = &now
		}
		if err := db.Model(&models.Upload{}).Where("id = ?", u.ID).Updates(up).Error; err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
