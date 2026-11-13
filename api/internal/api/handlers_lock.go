package api

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Locking notes and pages ("Lock" in the app; sealed in the code, apart from
// restricted pages): each person's lock key, kept wrapped by their PIN, all
// of it done in their browser. Orbit keeps the wrapped key and the sealed
// text; without the PIN neither opens — not for an admin, a backup, search
// or an assistant.

// minLockIterations: PBKDF2 rounds at least (the app uses 600 000).
const minLockIterations = 100_000

type lockKeyResponse struct {
	Set bool `json:"set"`
	*models.LockKey
	// How many notes and pages they've locked (all lost if the PIN is).
	Sealed int64 `json:"sealed"`
}

func (h *Handlers) sealedCount(uid string) int64 {
	var n, m int64
	h.db.Model(&models.Note{}).Where("sealed = ? AND sealed_by = ?", true, uid).Count(&n)
	h.db.Model(&models.Page{}).Where("sealed = ? AND sealed_by = ?", true, uid).Count(&m)
	return n + m
}

// getLockKey returns the caller's wrapped lock key (none yet: set false).
//
//	@Summary	My lock key (wrapped)
//	@Tags		lock
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	lockKeyResponse
//	@Router		/me/lock [get]
func (h *Handlers) getLockKey(c *gin.Context) {
	uid := auth.UserID(c)
	var k models.LockKey
	if h.db.Limit(1).Find(&k, "user_id = ?", uid).RowsAffected == 0 {
		c.JSON(http.StatusOK, lockKeyResponse{Sealed: h.sealedCount(uid)})
		return
	}
	c.JSON(http.StatusOK, lockKeyResponse{Set: true, LockKey: &k, Sealed: h.sealedCount(uid)})
}

type lockKeyRequest struct {
	Salt       string `json:"salt" binding:"required"`
	Iterations int    `json:"iterations" binding:"required"`
	IV         string `json:"iv" binding:"required"`
	Wrapped    string `json:"wrapped" binding:"required"`
}

// setLockKey keeps the caller's lock key wrapped by a PIN: the first time, or
// the same key wrapped by a new PIN (the app checks the old one).
//
//	@Summary	Set or change my lock PIN
//	@Tags		lock
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		lockKeyRequest	true	"The key, wrapped"
//	@Success	200		{object}	lockKeyResponse
//	@Router		/me/lock [put]
func (h *Handlers) setLockKey(c *gin.Context) {
	var req lockKeyRequest
	if !bindJSON(c, &req) {
		return
	}
	salt, e1 := base64.StdEncoding.DecodeString(req.Salt)
	iv, e2 := base64.StdEncoding.DecodeString(req.IV)
	wrapped, e3 := base64.StdEncoding.DecodeString(req.Wrapped)
	switch {
	case e1 != nil || e2 != nil || e3 != nil:
		c.JSON(http.StatusBadRequest, gin.H{"error": "salt, iv and wrapped are base64"})
		return
	case len(salt) < 16 || len(iv) != 12 || len(wrapped) != 32+16:
		c.JSON(http.StatusBadRequest, gin.H{"error": "a 16-byte salt, a 12-byte iv and a 256-bit key wrapped with AES-GCM"})
		return
	case req.Iterations < minLockIterations || req.Iterations > 10_000_000:
		c.JSON(http.StatusBadRequest, gin.H{"error": "iterations: 100 000 or more"})
		return
	}
	uid := auth.UserID(c)
	k := models.LockKey{UserID: uid, Salt: req.Salt, Iterations: req.Iterations, IV: req.IV, Wrapped: req.Wrapped, UpdatedAt: time.Now()}
	if err := h.db.Save(&k).Error; err != nil {
		h.internalError(c, err, "could not keep the lock key")
		return
	}
	h.recordCtx(c, "lock.pin_set", "user", uid, nil)
	c.JSON(http.StatusOK, lockKeyResponse{Set: true, LockKey: &k, Sealed: h.sealedCount(uid)})
}

// deleteLockKey forgets the caller's lock key — and, as nothing could open
// them again, deletes what they locked.
//
//	@Summary	Forget my lock PIN (and what it locked)
//	@Tags		lock
//	@Security	BearerAuth
//	@Success	204
//	@Router		/me/lock [delete]
func (h *Handlers) deleteLockKey(c *gin.Context) {
	uid := auth.UserID(c)
	var notes, pages []string
	h.db.Model(&models.Note{}).Where("sealed = ? AND sealed_by = ?", true, uid).Pluck("id", &notes)
	h.db.Model(&models.Page{}).Where("sealed = ? AND sealed_by = ?", true, uid).Pluck("id", &pages)
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if len(notes) > 0 {
			if err := tx.Where("note_id IN ?", notes).Delete(&models.Drawing{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", notes).Delete(&models.Note{}).Error; err != nil {
				return err
			}
		}
		// A page goes to the trash, as any deleted one: its sub-pages (not
		// sealed) can be restored from there; it stays sealed, for good.
		if len(pages) > 0 {
			now := time.Now()
			if err := tx.Model(&models.Page{}).Where("id IN ? AND trashed_at IS NULL", pages).Update("trashed_at", &now).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&models.LockKey{}, "user_id = ?", uid).Error
	})
	if err != nil {
		h.internalError(c, err, "could not forget the lock key")
		return
	}
	h.recordCtx(c, "lock.pin_forget", "user", uid, gin.H{"notes": len(notes), "pages": len(pages)})
	c.Status(http.StatusNoContent)
}

// sealedByOther answers 403 when a sealed note or page is someone else's:
// only who locked it opens or changes it.
func sealedByOther(c *gin.Context, s models.Sealing) bool {
	if s.Sealed && s.SealedBy != auth.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "locked by someone else: only they can open or change it"})
		return true
	}
	return false
}

// hasLockKey: whether uid has set a lock PIN (needed to lock anything).
func (h *Handlers) hasLockKey(uid string) bool {
	var n int64
	h.db.Model(&models.LockKey{}).Where("user_id = ?", uid).Count(&n)
	return n > 0
}

// validCipher: sealed text as the app makes it — JSON, small enough.
func validCipher(s string) bool {
	return s != "" && len(s) <= 4<<20 && s[0] == '{'
}
