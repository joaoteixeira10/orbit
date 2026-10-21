package caldav

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Syncer keeps links in step. One sync at a time per link; a change while
// one runs asks for another after it.
//
// Each sync compares the server's listing (ETags) and each event's
// UpdatedAt with the stamp of when the two last agreed:
//
//   - an object new or changed on the server is read into its event (made if
//     new); changed on both sides, the later change wins;
//   - an object gone from the server takes its event with it (unless the
//     event changed in Orbit since: then it goes back up);
//   - an event changed in Orbit is merged into its object (only what changed
//     is rewritten) and PUT if the object is still that version;
//   - an event deleted in Orbit deletes its object (if still that version);
//   - an event new in Orbit becomes a new object.
//
// Course classes stay out (their course keeps them); objects that aren't
// events (to-dos) are left alone.
type Syncer struct {
	db     *gorm.DB
	sealer *keystore.Sealer
	log    *zap.Logger
	hc     *http.Client
	now    func() time.Time
	delay  time.Duration // before a sync a change asked for

	// Changed is told which workspace's events a sync changed.
	Changed func(wsID string)

	mu      sync.Mutex
	running map[string]bool // link id → again after this one
}

// New builds a Syncer with the guarded client (LAN servers allowed: a
// Nextcloud at home is the point).
func New(db *gorm.DB, sealer *keystore.Sealer, log *zap.Logger) *Syncer {
	return &Syncer{db: db, sealer: sealer, log: log, hc: HTTPClient(safedial.DialContext), now: time.Now, delay: 2 * time.Second, running: map[string]bool{}}
}

// UseClient replaces the HTTP client (tests reach httptest on loopback).
func (s *Syncer) UseClient(c *http.Client) *Syncer {
	s.hc = c
	return s
}

// NotifyHub tells a workspace's members when a sync changed its events.
func (s *Syncer) NotifyHub(hub *events.Hub) *Syncer {
	if hub == nil {
		return s
	}
	s.Changed = func(wsID string) {
		b, _ := json.Marshal(map[string]string{"type": "ws", "ws": wsID, "what": "events"})
		var ids []string
		s.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ?", wsID).Pluck("user_id", &ids)
		for _, id := range ids {
			hub.Publish(id, string(b))
		}
	}
	return s
}

// Client opens a link's account.
func (s *Syncer) Client(link models.CalDAVLink) (*Client, error) {
	pass, err := s.sealer.Unseal(link.PasswordSealed)
	if err != nil {
		return nil, errors.New("the saved password can't be read (was the master key changed?)")
	}
	return NewClient(s.hc, link.Username, string(pass)), nil
}

// Account is a client for an account being set up.
func (s *Syncer) Account(username, password string) *Client {
	return NewClient(s.hc, username, password)
}

// Seal stores a password sealed with the master key.
func (s *Syncer) Seal(password string) ([]byte, error) { return s.sealer.Seal([]byte(password)) }

// Kick syncs a workspace's link soon (after a change in Orbit), in the
// background.
func (s *Syncer) Kick(wsID string) {
	var link models.CalDAVLink
	if s.db.Where("workspace_id = ?", wsID).First(&link).Error != nil {
		return
	}
	go func() {
		time.Sleep(s.delay)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		_ = s.Sync(ctx, link.ID)
	}()
}

// RunAll syncs every link (the timer).
func (s *Syncer) RunAll(ctx context.Context) {
	var ids []string
	s.db.Model(&models.CalDAVLink{}).Pluck("id", &ids)
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		_ = s.Sync(ctx, id)
	}
}

// Sync runs one sync of a link now (or, if one is running, right after it).
// The error is also kept on the link.
func (s *Syncer) Sync(ctx context.Context, linkID string) error {
	s.mu.Lock()
	if _, busy := s.running[linkID]; busy {
		s.running[linkID] = true
		s.mu.Unlock()
		return nil
	}
	s.running[linkID] = false
	s.mu.Unlock()
	var err error
	for {
		err = s.sync(ctx, linkID)
		s.mu.Lock()
		again := s.running[linkID]
		if !again || ctx.Err() != nil {
			delete(s.running, linkID)
			s.mu.Unlock()
			return err
		}
		s.running[linkID] = false
		s.mu.Unlock()
	}
}

// result counts what a sync did.
type result struct {
	changedLocal bool
}

func (s *Syncer) sync(ctx context.Context, linkID string) error {
	var link models.CalDAVLink
	if err := s.db.First(&link, "id = ?", linkID).Error; err != nil {
		return err
	}
	cl, err := s.Client(link)
	if err == nil {
		var res result
		res, err = s.pass(ctx, cl, &link)
		if res.changedLocal && s.Changed != nil {
			s.Changed(link.WorkspaceID)
		}
	}
	now := s.now()
	upd := map[string]any{"synced_at": now, "last_error": ""}
	if err != nil {
		upd["last_error"] = clip(err.Error(), 500)
		s.log.Info("caldav sync failed", zap.String("link", link.ID), zap.Error(err))
	}
	var n int64
	s.db.Model(&models.CalDAVItem{}).Where("link_id = ? AND event_id <> ''", link.ID).Count(&n)
	upd["event_count"] = n
	s.db.Model(&models.CalDAVLink{}).Where("id = ?", link.ID).Updates(upd)
	return err
}

// stamp is a time as kept and compared (no monotonic reading, µs).
func stamp(t time.Time) time.Time { return t.Round(0).UTC().Truncate(time.Microsecond) }

func changedSince(ev models.Event, it models.CalDAVItem) bool {
	return stamp(ev.UpdatedAt).After(stamp(it.Stamp))
}

func (s *Syncer) pass(ctx context.Context, cl *Client, link *models.CalDAVLink) (result, error) {
	var res result
	remote, err := cl.List(ctx, link.CalendarURL)
	if err != nil {
		return res, err
	}
	var items []models.CalDAVItem
	s.db.Where("link_id = ?", link.ID).Find(&items)
	byURL := map[string]*models.CalDAVItem{}
	byEvent := map[string]*models.CalDAVItem{}
	for i := range items {
		byURL[items[i].URL] = &items[i]
		if items[i].EventID != "" {
			byEvent[items[i].EventID] = &items[i]
		}
	}
	var evs []models.Event
	s.db.Where("workspace_id = ? AND (course_id = '' OR course_id IS NULL)", link.WorkspaceID).Find(&evs)
	byID := map[string]*models.Event{}
	for i := range evs {
		byID[evs[i].ID] = &evs[i]
	}

	// 1. New and changed on the server.
	var fetch []string
	for u, etag := range remote {
		if it := byURL[u]; it == nil || it.ETag != etag {
			fetch = append(fetch, u)
		}
	}
	data := map[string][]byte{} // what was fetched, for merges below
	if len(fetch) > 0 {
		objs, err := cl.Get(ctx, link.CalendarURL, fetch)
		if err != nil {
			return res, err
		}
		for _, o := range objs {
			data[o.URL] = o.Data
			// The listing's ETag: what the next listing is compared with.
			etag := remote[o.URL]
			if etag == "" {
				etag = o.ETag
			}
			if s.pull(link, o, etag, byURL, byID) {
				res.changedLocal = true
			}
		}
	}

	// 2. Gone from the server.
	for u, it := range byURL {
		if _, ok := remote[u]; ok {
			continue
		}
		if ev := byID[it.EventID]; ev != nil {
			if changedSince(*ev, *it) {
				// Changed here since: it goes back up as a new object.
				s.db.Delete(it)
				delete(byEvent, it.EventID)
				delete(byURL, u)
				continue
			}
			s.db.Delete(ev)
			delete(byID, ev.ID)
			res.changedLocal = true
		}
		s.db.Delete(it)
		delete(byEvent, it.EventID)
		delete(byURL, u)
	}

	// 3. Changed or deleted in Orbit.
	for u, it := range byURL {
		if it.EventID == "" {
			continue
		}
		ev := byID[it.EventID]
		if ev == nil {
			if err := cl.Delete(ctx, u, it.ETag); err != nil && !errors.Is(err, ErrPrecondition) {
				return res, err
			} else if err == nil {
				s.db.Delete(it)
			}
			continue
		}
		if !changedSince(*ev, *it) {
			continue
		}
		cur, ok := data[u]
		if !ok {
			objs, err := cl.Get(ctx, link.CalendarURL, []string{u})
			if err != nil {
				return res, err
			}
			if len(objs) == 0 {
				continue
			}
			cur = objs[0].Data
		}
		merged, changed, err := planner.MergeObject(cur, feedEvent(*ev), time.UTC, s.now())
		if err != nil {
			continue
		}
		etag := it.ETag
		if changed {
			etag, err = cl.Put(ctx, u, it.ETag, merged)
			if errors.Is(err, ErrPrecondition) {
				continue // changed there meanwhile: the next sync reads it
			}
			if err != nil {
				return res, err
			}
		}
		s.db.Model(it).Updates(map[string]any{"e_tag": etag, "stamp": stamp(ev.UpdatedAt)})
	}

	// 4. New in Orbit.
	base := link.CalendarURL
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	for _, ev := range evs {
		if byEvent[ev.ID] != nil || byID[ev.ID] == nil {
			continue
		}
		u := base + url.PathEscape(ev.ID) + ".ics"
		etag, err := cl.Put(ctx, u, "", planner.BuildObject(feedEvent(ev), s.now()))
		if errors.Is(err, ErrPrecondition) {
			continue
		}
		if err != nil {
			return res, err
		}
		it := models.CalDAVItem{LinkID: link.ID, EventID: ev.ID, URL: u, ETag: etag, Stamp: stamp(ev.UpdatedAt)}
		s.db.Create(&it)
	}
	return res, nil
}

// pull reads one fetched object into Orbit; true when an event changed.
func (s *Syncer) pull(link *models.CalDAVLink, o Object, etag string, byURL map[string]*models.CalDAVItem, byID map[string]*models.Event) bool {
	it := byURL[o.URL]
	r, err := planner.ParseObject(o.Data, time.UTC)
	if err != nil {
		// Not an event (or unreadable): remember it, to skip it until it changes.
		if it == nil {
			it = &models.CalDAVItem{LinkID: link.ID, URL: o.URL, ETag: etag, Stamp: s.now()}
			s.db.Create(it)
			byURL[o.URL] = it
		} else {
			s.db.Model(it).Update("e_tag", etag)
		}
		return false
	}
	var ev *models.Event
	if it != nil {
		ev = byID[it.EventID]
	}
	if ev != nil && changedSince(*ev, *it) && !r.Modified.IsZero() && r.Modified.Before(ev.UpdatedAt) {
		// Changed on both sides, Orbit's later: keep it (it goes up below).
		s.db.Model(it).Update("e_tag", etag)
		it.ETag = etag
		return false
	}
	isNew := ev == nil
	if isNew {
		ev = &models.Event{WorkspaceID: link.WorkspaceID, CreatedBy: link.CreatedBy}
	}
	if !apply(ev, r) && !isNew {
		s.db.Model(it).Updates(map[string]any{"e_tag": etag, "stamp": stamp(ev.UpdatedAt)})
		it.ETag, it.Stamp = etag, stamp(ev.UpdatedAt)
		return false
	}
	if err := s.db.Save(ev).Error; err != nil {
		return false
	}
	byID[ev.ID] = ev
	if it == nil {
		it = &models.CalDAVItem{LinkID: link.ID, URL: o.URL}
		byURL[o.URL] = it
	}
	it.EventID, it.ETag, it.Stamp = ev.ID, etag, stamp(ev.UpdatedAt)
	s.db.Save(it)
	return true
}

// apply copies a server's event onto an Orbit one; false when nothing differs.
func apply(ev *models.Event, r planner.RemoteEvent) bool {
	if r.End.Sub(r.Start) > 366*24*time.Hour {
		r.End = r.Start.Add(366 * 24 * time.Hour)
	}
	tz := r.TZ
	if tz == "" && !r.AllDay {
		tz = ev.TZ // a time in UTC: the zone it was typed in stays
	}
	exdates := ""
	if r.RRule != "" && len(r.ExDates) > 0 {
		exdates = planner.EncodeExDates(r.ExDates)
	}
	next := *ev
	next.Title, next.Location, next.Notes = clip(r.Title, 300), clip(r.Location, 300), clip(r.Notes, 20000)
	next.StartsAt, next.EndsAt, next.AllDay, next.TZ, next.RRule, next.ExDates = r.Start, r.End, r.AllDay, tz, r.RRule, exdates
	next.Until = planner.SeriesUntil(next)
	same := next.Title == ev.Title && next.Location == ev.Location && next.Notes == ev.Notes &&
		next.StartsAt.Equal(ev.StartsAt) && next.EndsAt.Equal(ev.EndsAt) && next.AllDay == ev.AllDay &&
		next.TZ == ev.TZ && next.RRule == ev.RRule && next.ExDates == ev.ExDates
	*ev = next
	return !same
}

func feedEvent(e models.Event) planner.FeedEvent {
	return planner.FeedEvent{
		UID: e.ID, Title: e.Title, Notes: e.Notes, Location: e.Location, Start: e.StartsAt, End: e.EndsAt,
		AllDay: e.AllDay, TZ: e.TZ, RRule: e.RRule, ExDates: planner.DecodeExDates(e.ExDates), Updated: e.UpdatedAt,
	}
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
