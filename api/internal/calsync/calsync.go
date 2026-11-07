// Package calsync fetches subscribed ICS calendars into ExternalEvent rows:
// on demand (when a subscription is added or refreshed by hand) and on a
// timer for those due (jobs runs RunDue every few minutes).
//
// Calendar addresses are user-supplied URLs, so every fetch goes through the
// SSRF-guarded dialer (no loopback, link-local or metadata addresses, checked
// on the IP actually dialed), with a size cap and a timeout. Unlike the
// notification client it follows up to three redirects — calendar hosts use
// them — each hop dialed through the same guard.
package calsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/keystore"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"
	"github.com/joaoteixeira10/orbit/api/internal/safedial"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MaxBytes caps one calendar download.
const MaxBytes = 8 << 20

// Window is how much of a calendar is kept: a quarter back, a year ahead.
var (
	WindowBack  = 90 * 24 * time.Hour
	WindowAhead = 400 * 24 * time.Hour
)

// Syncer refreshes subscriptions.
type Syncer struct {
	db     *gorm.DB
	sealer *keystore.Sealer
	log    *zap.Logger
	client *http.Client
	now    func() time.Time
}

// New builds a Syncer with the guarded client.
func New(db *gorm.DB, sealer *keystore.Sealer, log *zap.Logger) *Syncer {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirected to a non-web address")
			}
			return nil
		},
		Transport: &http.Transport{
			DialContext:           safedial.DialContext,
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: 15 * time.Second,
		},
	}
	return &Syncer{db: db, sealer: sealer, log: log, client: client, now: time.Now}
}

// UseClient replaces the HTTP client (tests reach httptest on loopback).
func (s *Syncer) UseClient(c *http.Client) *Syncer {
	s.client = c
	return s
}

// NormalizeURL accepts http(s) and webcal(s) addresses (webcal is how
// "subscribe" links are shared) and returns the https/http form.
func NormalizeURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errors.New("enter the calendar's full address (https://… or webcal://…)")
	}
	switch strings.ToLower(u.Scheme) {
	case "webcal", "webcals":
		u.Scheme = "https"
	case "http", "https":
	default:
		return nil, errors.New("the address must start with https://, http:// or webcal://")
	}
	if u.User != nil {
		return nil, errors.New("put no user name or password in the address")
	}
	return u, nil
}

// Seal / Open store a calendar address sealed with the master key.
func (s *Syncer) Seal(u *url.URL) ([]byte, error) { return s.sealer.Seal([]byte(u.String())) }

func (s *Syncer) Open(sub models.CalendarSubscription) (string, error) {
	b, err := s.sealer.Unseal(sub.URLSealed)
	return string(b), err
}

// Fetch downloads and parses a calendar address.
func (s *Syncer) Fetch(ctx context.Context, address string) ([]planner.ExternalOccurrence, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/calendar, */*;q=0.5")
	req.Header.Set("User-Agent", "Orbit-Calendar/1")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach the calendar: %s", plainErr(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the calendar's server answered %s", resp.Status)
	}
	body := io.LimitReader(resp.Body, MaxBytes+1)
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("couldn't download the calendar: %s", plainErr(err))
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("the calendar is larger than %d MiB", MaxBytes>>20)
	}
	now := s.now()
	return planner.ParseFeed(strings.NewReader(string(data)), now.Add(-WindowBack), now.Add(WindowAhead), time.UTC)
}

// plainErr strips Go's "Get \"<url>\":" prefix, which would echo the secret
// address into an error shown on the page.
func plainErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// Refresh fetches one subscription and replaces its events. A failure keeps
// the events from the last good fetch and records the error.
func (s *Syncer) Refresh(ctx context.Context, sub *models.CalendarSubscription) error {
	address, err := s.Open(*sub)
	if err != nil {
		return s.fail(sub, errors.New("the stored address can't be opened (was ORBIT_MASTER_KEY changed?)"))
	}
	occ, err := s.Fetch(ctx, address)
	if err != nil {
		return s.fail(sub, err)
	}
	now := s.now()
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", sub.ID).Delete(&models.ExternalEvent{}).Error; err != nil {
			return err
		}
		rows := make([]models.ExternalEvent, 0, len(occ))
		for _, o := range occ {
			rows = append(rows, models.ExternalEvent{
				SubscriptionID: sub.ID, UID: clip(o.UID, 255), Title: clip(o.Title, 300), Location: clip(o.Location, 300),
				Notes: clip(o.Notes, 4000), StartsAt: o.Start, EndsAt: o.End, AllDay: o.AllDay,
			})
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(rows, 200).Error; err != nil {
				return err
			}
		}
		return tx.Model(sub).Updates(map[string]any{"fetched_at": &now, "last_error": "", "event_count": len(rows)}).Error
	})
	if err != nil {
		return s.fail(sub, err)
	}
	sub.FetchedAt, sub.LastError, sub.EventCount = &now, "", len(occ)
	return nil
}

func (s *Syncer) fail(sub *models.CalendarSubscription, err error) error {
	now := s.now()
	msg := clip(err.Error(), 500)
	// FetchedAt moves on even on failure, so a broken calendar is retried at
	// its interval rather than every run.
	s.db.Model(sub).Updates(map[string]any{"fetched_at": &now, "last_error": msg})
	sub.FetchedAt, sub.LastError = &now, msg
	return err
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	for len(string(r)) > n {
		r = r[:len(r)-1]
	}
	return string(r)
}

// RunDue refreshes the subscriptions whose interval has passed.
func (s *Syncer) RunDue(ctx context.Context) {
	var subs []models.CalendarSubscription
	s.db.Find(&subs)
	now := s.now()
	for i := range subs {
		sub := &subs[i]
		every := time.Duration(max(15, sub.RefreshMinutes)) * time.Minute
		if sub.FetchedAt != nil && now.Sub(*sub.FetchedAt) < every {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if err := s.Refresh(ctx, sub); err != nil {
			s.log.Info("calendar subscription refresh failed", zap.String("subscription", sub.ID), zap.String("error", sub.LastError))
		}
	}
}
