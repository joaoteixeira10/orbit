// Package uploads keeps the files people add to pages, tasks and notes: in
// the database (models.Upload + models.UploadBlob), so a backup carries them.
// It sniffs what a file is (never trusting the browser's word), says which
// ones a browser may show inline, and deletes a thing's files with it.
package uploads

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path"
	"strings"
	"unicode"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

// Targets are what a file can be on.
const (
	Page = "page"
	Task = "task"
	Note = "note"
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

// Create stores a file (within the quota; 0 = none).
func Create(db *gorm.DB, u *models.Upload, data []byte, quota int64) error {
	if quota > 0 {
		var used struct{ Total int64 }
		db.Model(&models.Upload{}).Select("COALESCE(SUM(size), 0) AS total").Scan(&used)
		if used.Total+int64(len(data)) > quota {
			return ErrQuota
		}
	}
	sum := sha256.Sum256(data)
	u.Size, u.SHA256, u.Type = int64(len(data)), hex.EncodeToString(sum[:]), Sniff(data)
	u.Name = CleanName(u.Name)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		return tx.Create(&models.UploadBlob{ID: u.ID, Data: data}).Error
	})
}

// Delete removes the uploads a query selects (on models.Upload), bytes too.
func Delete(db *gorm.DB, where string, args ...any) error {
	var ids []string
	db.Model(&models.Upload{}).Where(where, args...).Pluck("id", &ids)
	if len(ids) == 0 {
		return nil
	}
	if err := db.Where("id IN ?", ids).Delete(&models.UploadBlob{}).Error; err != nil {
		return err
	}
	return db.Where("id IN ?", ids).Delete(&models.Upload{}).Error
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
