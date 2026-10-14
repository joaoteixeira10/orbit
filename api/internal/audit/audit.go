// Package audit records mutating actions for traceability. Every create /
// update / delete across the API should funnel through Record.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Recorder writes audit entries.
type Recorder struct {
	db  *gorm.DB
	log *zap.Logger
	// mu serializes writes so the tamper-evidence hash chain links entries in a
	// well-defined order (the app is single-instance behind a proxy).
	mu sync.Mutex
	// forward, when set (ForwardTo), carries committed entries to an external
	// sink such as syslog.
	forward chan models.AuditLog
	// onRecord are called with each committed entry (the integration webhook).
	onRecord []func(models.AuditLog)
}

// OnRecord registers a function called with every committed entry. It runs
// on the request's goroutine, so it must return quickly (hand work off).
func (r *Recorder) OnRecord(fn func(models.AuditLog)) {
	r.onRecord = append(r.onRecord, fn)
}

// New constructs a Recorder.
func New(db *gorm.DB, log *zap.Logger) *Recorder {
	return &Recorder{db: db, log: log}
}

// chainContent is the canonical, deterministic serialization of an entry's
// fields hashed into the chain. Changing any field (or the timestamp) breaks
// verification. Shared by the recorder and the verifier so they agree exactly.
func chainContent(e *models.AuditLog) string {
	imp := ""
	if e.ImpersonatorID != nil {
		imp = *e.ImpersonatorID
	}
	return fmt.Sprintf("%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s",
		e.UserID, imp, e.Action, e.TargetType, e.TargetID, e.IP, e.Detail,
		e.CreatedAt.UTC().Format(time.RFC3339Nano))
}

// chainHash computes Hash = sha256(prevHash || canonical(entry)).
func chainHash(prevHash string, e *models.AuditLog) string {
	sum := sha256.Sum256([]byte(prevHash + "\x1e" + chainContent(e)))
	return hex.EncodeToString(sum[:])
}

// Record persists an audit entry. Failures are logged, never propagated — an
// audit write must not break the user-facing operation it describes.
func (r *Recorder) Record(userID string, action, targetType, targetID, ip string, detail any) {
	r.RecordImpersonated(userID, nil, action, targetType, targetID, ip, detail)
}

// RecordImpersonated is Record with an optional impersonator: when
// impersonatorID is non-nil, the action was performed by that admin while
// impersonating userID.
func (r *Recorder) RecordImpersonated(userID string, impersonatorID *string, action, targetType, targetID, ip string, detail any) {
	var detailJSON string
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	entry := models.AuditLog{
		UserID:         userID,
		ImpersonatorID: impersonatorID,
		Action:         action,
		TargetType:     targetType,
		TargetID:       targetID,
		IP:             ip,
		Detail:         detailJSON,
	}
	// Serialize so the hash chain links entries deterministically.
	r.mu.Lock()
	defer r.mu.Unlock()
	entry.CreatedAt = time.Now() // fixed before hashing so verify can recompute
	var prev models.AuditLog
	prevHash := ""
	if err := r.db.Order("id DESC").First(&prev).Error; err == nil {
		prevHash = prev.Hash
	}
	entry.PrevHash = prevHash
	entry.Hash = chainHash(prevHash, &entry)
	if err := r.db.Create(&entry).Error; err != nil {
		r.log.Error("audit write failed",
			zap.String("action", action), zap.Error(err))
		return
	}
	r.enqueueForward(entry)
	for _, fn := range r.onRecord {
		fn(entry)
	}
}

// VerifyResult reports the outcome of an audit-chain integrity check.
type VerifyResult struct {
	OK       bool   `json:"ok"`
	Checked  int    `json:"checked"`
	BrokenAt string `json:"broken_at,omitempty"` // id of the first bad entry ("" if OK)
	Reason   string `json:"reason,omitempty"`
}

// Verify walks the audit chain in id order and recomputes each hash, reporting
// the first entry whose stored hash or back-link doesn't match (i.e. a row that
// was altered, inserted, or deleted after the fact).
func (r *Recorder) Verify() VerifyResult {
	const batch = 500
	var (
		lastHash string
		checked  int
		lastID   string
	)
	for {
		var rows []models.AuditLog
		if err := r.db.Where("id > ?", lastID).Order("id ASC").Limit(batch).Find(&rows).Error; err != nil {
			return VerifyResult{Reason: "could not read audit log"}
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			e := &rows[i]
			if e.PrevHash != lastHash {
				return VerifyResult{Checked: checked, BrokenAt: e.ID, Reason: "broken chain link"}
			}
			if chainHash(e.PrevHash, e) != e.Hash {
				return VerifyResult{Checked: checked, BrokenAt: e.ID, Reason: "entry content altered"}
			}
			lastHash = e.Hash
			lastID = e.ID
			checked++
		}
	}
	return VerifyResult{OK: true, Checked: checked}
}

// BackfillChain initializes Hash/PrevHash for any pre-existing entries that
// lack them (one-time, on upgrade), so the chain is continuous going forward.
// It can't detect tampering that predates the chain — it establishes the
// baseline. Idempotent.
func (r *Recorder) BackfillChain() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var lastHashed models.AuditLog
	prevHash := ""
	if err := r.db.Where("hash <> ?", "").Order("id DESC").First(&lastHashed).Error; err == nil {
		prevHash = lastHashed.Hash
	}
	const batch = 500
	var lastID string
	if prevHash != "" {
		lastID = lastHashed.ID
	}
	for {
		var rows []models.AuditLog
		if err := r.db.Where("id > ? AND (hash = '' OR hash IS NULL)", lastID).
			Order("id ASC").Limit(batch).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			e := &rows[i]
			e.PrevHash = prevHash
			e.Hash = chainHash(prevHash, e)
			if err := r.db.Model(e).Updates(map[string]any{"prev_hash": e.PrevHash, "hash": e.Hash}).Error; err != nil {
				return err
			}
			prevHash = e.Hash
			lastID = e.ID
		}
	}
}
