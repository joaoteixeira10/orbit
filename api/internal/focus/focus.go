// Package focus is the focus timer's bookkeeping: a session ends (stopped,
// or its time up) once, and its minutes go on its task.
package focus

import (
	"math"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

// End closes a running session at `at` (no later than its planned end) and
// adds the minutes to its task. It reports false when it had ended already.
func End(db *gorm.DB, s *models.FocusSession, at time.Time) bool {
	planned := s.StartedAt.Add(time.Duration(s.Minutes) * time.Minute)
	if at.After(planned) {
		at = planned
	}
	if at.Before(s.StartedAt) {
		at = s.StartedAt
	}
	spent := int(math.Round(at.Sub(s.StartedAt).Minutes()))
	res := db.Model(&models.FocusSession{}).Where("id = ? AND ended_at IS NULL", s.ID).
		Updates(map[string]any{"ended_at": at, "spent": spent})
	if res.RowsAffected == 0 {
		return false
	}
	s.EndedAt, s.Spent = &at, spent
	if s.TaskID != "" && spent > 0 {
		db.Model(&models.Task{}).Where("id = ?", s.TaskID).UpdateColumn("focus_minutes", gorm.Expr("focus_minutes + ?", spent))
	}
	return true
}

// Due lists running sessions whose time is up.
func Due(db *gorm.DB, now time.Time) []models.FocusSession {
	var running []models.FocusSession
	db.Where("ended_at IS NULL").Find(&running)
	out := running[:0]
	for _, s := range running {
		if !s.StartedAt.Add(time.Duration(s.Minutes) * time.Minute).After(now) {
			out = append(out, s)
		}
	}
	return out
}
