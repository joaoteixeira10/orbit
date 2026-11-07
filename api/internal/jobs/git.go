package jobs

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/feature"
	"github.com/joaoteixeira10/orbit/api/internal/gitsync"
)

// UseGit gives the scheduler the git sync: every few minutes it pulls the
// linked lists' issues and the state of tasks linked by hand, and sends
// again the changes that didn't reach git.
func (s *Scheduler) UseGit(g *gitsync.Engine) *Scheduler {
	s.git = g
	return s
}

var gitRunning atomic.Bool

func (s *Scheduler) syncGit() {
	if s.git == nil || !feature.Enabled(s.db, feature.GitIntegration) || !gitRunning.CompareAndSwap(false, true) {
		return
	}
	defer gitRunning.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s.git.SyncAll(ctx)
}
