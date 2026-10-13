// Package jobs runs the in-process background scheduler: update checks, the
// sweeps of expired API tokens and single-use pre-auth tokens, scheduled
// backups, and the planner's work (calendar subscriptions, reminders, trash).
package jobs

import (
	"context"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/backup"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/version"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Scheduler owns the cron runner.
type Scheduler struct {
	sealer   *keystore.Sealer // scheduled backups (nil: none)
	db       *gorm.DB
	notifier *notify.Service
	updates  *version.Checker
	log      *zap.Logger
	cron     *cron.Cron
	calsync  *calsync.Syncer // planner jobs (nil: off)
	hub      *events.Hub     // reminders to open sessions
}

// NewScheduler builds a Scheduler.
func NewScheduler(db *gorm.DB, notifier *notify.Service, updates *version.Checker, log *zap.Logger) *Scheduler {
	return &Scheduler{db: db, notifier: notifier, updates: updates, log: log, cron: cron.New()}
}

// UseBackups turns on the hourly check of the owner's backup schedule.
func (s *Scheduler) UseBackups(sealer *keystore.Sealer) *Scheduler {
	s.sealer = sealer
	return s
}

// Start registers jobs and launches the cron runner. The update check also
// runs once right away, so a fresh instance knows its update status.
func (s *Scheduler) Start() {
	if s.updates != nil && s.updates.Enabled() {
		if _, err := s.cron.AddFunc("@every 6h", s.checkUpdates); err != nil {
			s.log.Error("schedule update check failed", zap.Error(err))
		}
	}
	if _, err := s.cron.AddFunc("@daily", s.sweepAPITokens); err != nil {
		s.log.Error("schedule API-token sweep failed", zap.Error(err))
	}
	if _, err := s.cron.AddFunc("@every 15m", s.sweepEphemeral); err != nil {
		s.log.Error("schedule ephemeral-token sweep failed", zap.Error(err))
	}
	if s.sealer != nil {
		if _, err := s.cron.AddFunc("@every 1h", s.scheduledBackup); err != nil {
			s.log.Error("schedule backup job failed", zap.Error(err))
		}
	}
	s.startPlanner()
	s.cron.Start()
	go s.checkUpdates() // prime update status at startup
	s.log.Info("scheduler started", zap.Bool("notifications", s.notifier.AnyEnabled()))
}

// checkUpdates polls the configured releases API for a newer version.
func (s *Scheduler) checkUpdates() {
	if s.updates == nil || !s.updates.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.updates.Check(ctx); err != nil {
		s.log.Warn("update check failed", zap.Error(err))
		return
	}
	s.log.Info("update check completed",
		zap.String("latest", s.updates.Latest()),
		zap.Bool("update_available", s.updates.UpdateAvailable()))
}

// sweepAPITokens deletes expired API tokens.
func (s *Scheduler) sweepAPITokens() {
	if n, err := auth.SweepExpiredAPITokens(s.db); err != nil {
		s.log.Warn("API-token sweep failed", zap.Error(err))
	} else if n > 0 {
		s.log.Info("swept expired API tokens", zap.Int64("count", n))
	}
}

// sweepEphemeral deletes expired single-use pre-auth tokens (OIDC state,
// WebAuthn challenges, SSO codes, SSE tickets) that were never redeemed.
func (s *Scheduler) sweepEphemeral() {
	if n, err := ephemeral.New(s.db).Sweep(); err != nil {
		s.log.Warn("ephemeral-token sweep failed", zap.Error(err))
	} else if n > 0 {
		s.log.Debug("swept expired ephemeral tokens", zap.Int64("count", n))
	}
}

// scheduledBackup writes the owner's automatic backup when it is due.
func (s *Scheduler) scheduledBackup() {
	r := backup.RunDueScheduled(s.db, s.sealer, version.Current().Version, time.Now())
	switch {
	case r == nil:
	case r.OK:
		s.log.Info("scheduled backup written", zap.String("where", r.Where))
	default:
		s.log.Warn("scheduled backup failed (retried in an hour)", zap.String("error", r.Error))
	}
}

// Stop halts the scheduler, waiting for running jobs.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
