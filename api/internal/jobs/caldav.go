package jobs

import (
	"context"
	"sync/atomic"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/caldav"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
)

// UseCalDAV gives the scheduler the CalDAV sync: every few minutes each
// workspace calendar linked to a CalDAV calendar is brought in step (a change
// in Orbit also syncs right away; this catches the other app's).
func (s *Scheduler) UseCalDAV(d *caldav.Syncer) *Scheduler {
	s.dav = d
	return s
}

var davRunning atomic.Bool

func (s *Scheduler) syncCalDAV() {
	if s.dav == nil || !feature.Enabled(s.db, feature.CalDAVSync) || !davRunning.CompareAndSwap(false, true) {
		return
	}
	defer davRunning.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s.dav.RunAll(ctx)
}
