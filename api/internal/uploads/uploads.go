// Package uploads keeps the files people add to pages, tasks and notes: a
// models.Upload each, their bytes by SHA-256 in the instance's store (the
// database by default, so a backup carries them; or a directory, or S3).
// It sniffs what a file is (never trusting the browser's word), says which
// ones a browser may show inline, and deletes a thing's files with it.
package uploads

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"unicode"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Targets are what a file can be on.
const (
	Page = "page"
	Task = "task"
	Note = "note"
	// Chat: a file added to a conversation with the assistant — its owner's
	// alone (no workspace), on the conversation (TargetID; "" until the
	// message goes).
	Chat = "chat"
)

// ErrQuota: the instance's uploads are full.
var ErrQuota = errors.New("the space for uploads on this instance is full")

// inline are the types a browser may show in the page (raster images). The
// rest — SVG included, which can carry script — is only ever downloaded.
var inline = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// Inline reports a type shown in the page (an image) rather than downloaded.
func Inline(typ string) bool { return inline[typ] }

// Sniff is what the bytes are, from their first 512 bytes.
func Sniff(data []byte) string {
	t := http.DetectContentType(data)
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}

// CleanName makes a file name safe to show and to put in a header or a zip:
// the base name only, no control or path characters, at most 120 runes.
func CleanName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsControl(r), strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(strings.TrimSpace(b.String()), ".")
	if r := []rune(out); len(r) > 120 {
		out = string(r[:120])
	}
	if out == "" {
		out = "file"
	}
	return out
}

// Used is the space files take: each set of bytes once, however many files
// share it.
func Used(db *gorm.DB, where string, args ...any) int64 {
	q := db.Model(&models.Upload{}).Select("MAX(size) AS size").Group("sha256")
	if where != "" {
		q = q.Where(where, args...)
	}
	var out struct{ Total int64 }
	db.Table("(?) AS f", q).Select("COALESCE(SUM(f.size), 0) AS total").Scan(&out)
	return out.Total
}

// Sum is the SHA-256 (hex) of data.
func Sum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

// Create stores a file (within the quota; 0 = none). Bytes already kept for
// another file are shared, and take no more space.
func Create(db *gorm.DB, u *models.Upload, data []byte, quota int64) error {
	sum := Sum(data)
	if quota > 0 {
		var have int64
		db.Model(&models.Upload{}).Where("sha256 = ?", sum).Count(&have)
		if have == 0 && Used(db, "")+int64(len(data)) > quota {
			return ErrQuota
		}
	}
	u.Size, u.SHA256, u.Type = int64(len(data)), sum, Sniff(data)
	u.Name = CleanName(u.Name)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := current.Put(tx, sum, data); err != nil {
			return err
		}
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		if u.Lineage == "" { // its own first version
			u.Lineage = u.ID
			if err := tx.Model(&models.Upload{}).Where("id = ?", u.ID).Update("lineage", u.ID).Error; err != nil {
				return err
			}
		}
		if t := TextOf(u.Name, u.Type, data); t != "" { // what it says, for search
			return SetText(tx, u.ID, t)
		}
		return nil
	})
}

// Read is a file's bytes.
func Read(db *gorm.DB, u models.Upload) ([]byte, error) {
	return current.Get(db, u.SHA256)
}

// Delete removes the uploads a query selects (on models.Upload); bytes no
// other file uses go too.
func Delete(db *gorm.DB, where string, args ...any) error {
	var gone []models.Upload
	db.Model(&models.Upload{}).Select("id", "sha256").Where(where, args...).Find(&gone)
	if len(gone) == 0 {
		return nil
	}
	ids := make([]string, 0, len(gone))
	sums := map[string]bool{}
	for _, u := range gone {
		ids = append(ids, u.ID)
		sums[u.SHA256] = true
	}
	if err := db.Where("id IN ?", ids).Delete(&models.Upload{}).Error; err != nil {
		return err
	}
	if err := db.Where("id IN ?", ids).Delete(&models.UploadText{}).Error; err != nil {
		return err
	}
	for sum := range sums {
		var left int64
		db.Model(&models.Upload{}).Where("sha256 = ?", sum).Count(&left)
		if left == 0 && sum != "" {
			if err := current.Delete(db, sum); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- Start: the bytes kept the old way, and the store in use ----

// settingStore records where the files are, so starting with another
// UPLOAD_STORE is caught (instead of files gone missing).
const settingStore = "upload_store"

// Migrate moves bytes kept the old way (a row per file, upload_blobs) into
// the store, once; n is told how many so far.
func Migrate(db *gorm.DB, n func(done int)) error {
	done := 0
	for {
		var rows []models.UploadBlob
		if err := db.Limit(20).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			sum := Sum(r.Data)
			if err := current.Put(db, sum, r.Data); err != nil {
				return err
			}
			if err := db.Model(&models.Upload{}).Where("id = ?", r.ID).Update("sha256", sum).Error; err != nil {
				return err
			}
			if err := db.Delete(&models.UploadBlob{}, "id = ?", r.ID).Error; err != nil {
				return err
			}
			done++
		}
		if n != nil {
			n(done)
		}
	}
}

// Recorded is where the files were kept (by the last start or move).
func Recorded(db *gorm.DB) string {
	var row models.Setting
	if db.Where(&models.Setting{Key: settingStore}).First(&row).Error != nil {
		return ""
	}
	return row.Value
}

// Record notes where the files are kept now.
func Record(db *gorm.DB, kind string) error {
	return db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&models.Setting{Key: settingStore, Value: kind}).Error
}

// Check refuses a store other than the one the files are in (when there are
// files); a first start records it.
func Check(db *gorm.DB) error {
	was := Recorded(db)
	if was == "" || was == current.Kind() {
		return Record(db, current.Kind())
	}
	var n int64
	db.Model(&models.Upload{}).Count(&n)
	if n == 0 {
		return Record(db, current.Kind())
	}
	return fmt.Errorf("the files are kept in %q but UPLOAD_STORE is %q: set UPLOAD_STORE=%s, or move them first with `orbit move-files %s`", was, current.Kind(), was, current.Kind())
}

// Move copies every file's bytes from one store to another (each checked by
// its sum), then records the new one; with clean, the old copies go. n is
// told how many so far.
func Move(db *gorm.DB, from, to Store, clean bool, n func(done int)) error {
	var sums []string
	if err := db.Model(&models.Upload{}).Distinct("sha256").Where("sha256 <> ''").Pluck("sha256", &sums).Error; err != nil {
		return err
	}
	for i, sum := range sums {
		data, err := from.Get(db, sum)
		if err != nil {
			return fmt.Errorf("reading %s from %s: %w", sum, from.Kind(), err)
		}
		if Sum(data) != sum {
			return fmt.Errorf("%s in %s doesn't match its sum (damaged?)", sum, from.Kind())
		}
		if err := to.Put(db, sum, data); err != nil {
			return fmt.Errorf("writing %s to %s: %w", sum, to.Kind(), err)
		}
		if n != nil && (i+1)%20 == 0 {
			n(i + 1)
		}
	}
	if err := Record(db, to.Kind()); err != nil {
		return err
	}
	if clean {
		for _, sum := range sums {
			if err := from.Delete(db, sum); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteFor removes the files on some pages, tasks or notes.
func DeleteFor(db *gorm.DB, target string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return Delete(db, "target = ? AND target_id IN ?", target, ids)
}

// URL is how a file is linked from Markdown: /api/v1/uploads/<id>/<name>.
func URL(u models.Upload) string {
	return "/api/v1/uploads/" + u.ID + "/" + escapePath(u.Name)
}

func escapePath(s string) string {
	r := strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "#", "%23", "?", "%3F", "%", "%25")
	return r.Replace(s)
}
