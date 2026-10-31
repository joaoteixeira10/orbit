// Package collab keeps who is where right now (presence) and short locks
// for starting a document's change log, in memory: they matter only while
// people are looking, and a restart forgets them (the browsers say again
// within seconds). With several replicas, each knows its own visitors.
package collab

import (
	"sort"
	"sync"
	"time"
)

// TTL is how long a visitor counts as here after their last word.
const TTL = 40 * time.Second

// Person is someone on a document: one per browser tab (Client).
type Person struct {
	Client  string    `json:"client"`
	UserID  string    `json:"user_id"`
	Name    string    `json:"name"`
	Editing bool      `json:"editing"`
	Since   time.Time `json:"since"`
	seen    time.Time
}

// Room is everyone on every document.
type Room struct {
	mu    sync.Mutex
	docs  map[string]map[string]*Person // doc key → client → person
	seeds map[string]seed
	now   func() time.Time
}

type seed struct {
	client string
	until  time.Time
}

// New makes an empty room.
func New() *Room {
	return &Room{docs: map[string]map[string]*Person{}, seeds: map[string]seed{}, now: time.Now}
}

// Here says someone (a tab) is on a document — editing or looking — and
// returns everyone there now, and whether that's news (someone came, or
// started or stopped editing) rather than a heartbeat.
func (r *Room) Here(doc string, p Person) ([]Person, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	set := r.docs[doc]
	if set == nil {
		set = map[string]*Person{}
		r.docs[doc] = set
	}
	news := true
	if old, ok := set[p.Client]; ok && now.Sub(old.seen) <= TTL {
		p.Since = old.Since
		news = old.Editing != p.Editing || old.Name != p.Name
	} else {
		p.Since = now
	}
	p.seen = now
	set[p.Client] = &p
	return r.listLocked(doc), news
}

// Leave says a tab left a document; it returns who's still there.
func (r *Room) Leave(doc, client string) []Person {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.docs[doc], client)
	return r.listLocked(doc)
}

// People are those on a document now (each tab once; oldest first).
func (r *Room) People(doc string) []Person {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listLocked(doc)
}

// Users are the accounts on a document now (to send its changes to).
func (r *Room) Users(doc string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range r.People(doc) {
		if !seen[p.UserID] {
			seen[p.UserID] = true
			out = append(out, p.UserID)
		}
	}
	return out
}

func (r *Room) listLocked(doc string) []Person {
	now := r.now()
	out := []Person{}
	for id, p := range r.docs[doc] {
		if now.Sub(p.seen) > TTL {
			delete(r.docs[doc], id)
			continue
		}
		out = append(out, *p)
	}
	if len(r.docs[doc]) == 0 {
		delete(r.docs, doc)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Since.Equal(out[j].Since) {
			return out[i].Since.Before(out[j].Since)
		}
		return out[i].Client < out[j].Client
	})
	return out
}

// Seed lets one tab start an empty document's log (the others wait for its
// changes): true for the first to ask, or the same one again, for a while.
func (r *Room) Seed(doc, client string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if s, ok := r.seeds[doc]; ok && s.client != client && now.Before(s.until) {
		return false
	}
	r.seeds[doc] = seed{client: client, until: now.Add(15 * time.Second)}
	return true
}

// Seeded ends a document's start (its log has changes now).
func (r *Room) Seeded(doc string) {
	r.mu.Lock()
	delete(r.seeds, doc)
	r.mu.Unlock()
}
