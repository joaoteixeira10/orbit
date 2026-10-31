// Package events provides a tiny in-memory pub/sub hub used to push
// Server-Sent Events to a user's open browser sessions — e.g. "your privileges
// changed, re-fetch /auth/me". It also mints short-lived, single-use tickets so
// the browser EventSource API (which cannot send an Authorization header) can
// authenticate the stream without putting a bearer token in the URL.
//
// Subscriptions are process-local: on restart, streams drop and clients
// reconnect with a fresh ticket. That's fine — privilege state lives in the DB,
// not here. Tickets can be moved to a shared store (UseTicketStore) so one
// minted on one replica is redeemable on another; with several replicas, a
// push only reaches streams held by the replica that handled the change (the
// SPA still re-syncs on reconnect and on its next /auth/me).
package events

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// TicketTTL bounds how long an issued SSE ticket stays valid before redemption.
const TicketTTL = 30 * time.Second

// Hub fans messages out to per-user subscribers and mints stream tickets.
type Hub struct {
	mu      sync.Mutex
	subs    map[string]map[chan string]struct{}
	tickets map[string]ticket
	store   TicketStore      // shared ticket store; nil = the in-memory map
	now     func() time.Time // injectable for tests
}

// TicketStore is a shared, single-use token store (satisfied by
// *ephemeral.Store) used instead of the in-memory ticket map when set.
type TicketStore interface {
	Put(kind string, value any, ttl time.Duration) (string, error)
	Take(kind, id string, out any) (bool, error)
}

// ticketKind namespaces SSE tickets in a shared TicketStore.
const ticketKind = "sse_ticket"

// UseTicketStore makes the hub mint and redeem tickets through a shared store
// (so any replica can redeem them). Call once at wiring time.
func (h *Hub) UseTicketStore(ts TicketStore) *Hub {
	h.store = ts
	return h
}

type ticket struct {
	userID  string
	expires time.Time
}

// New builds an empty Hub.
func New() *Hub {
	return &Hub{
		subs:    map[string]map[chan string]struct{}{},
		tickets: map[string]ticket{},
		now:     time.Now,
	}
}

// Subscribe registers a subscriber for a user and returns its receive channel
// plus an unsubscribe func the caller MUST defer. The channel is buffered and
// closed on unsubscribe.
func (h *Hub) Subscribe(userID string) (<-chan string, func()) {
	ch := make(chan string, 64) // room for a burst (editing together); full, a message is dropped
	h.mu.Lock()
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan string]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		set := h.subs[userID]
		if set == nil {
			return
		}
		if _, ok := set[ch]; ok {
			delete(set, ch)
			close(ch)
		}
		if len(set) == 0 {
			delete(h.subs, userID)
		}
	}
}

// Publish delivers a message to every live subscriber of a user. It never
// blocks: a subscriber whose buffer is full simply misses this message (it will
// re-sync on its next reconnect), so one stuck client can't stall a mutation.
func (h *Hub) Publish(userID string, msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[userID] {
		select {
		case ch <- msg:
		default:
		}
	}
}

// Broadcast delivers a message to every live subscriber across all users. Used
// for shared-resource changes (e.g. a new certificate) where each recipient's
// client re-fetches its own permission-filtered view.
func (h *Hub) Broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for ch := range set {
			select {
			case ch <- msg:
			default:
			}
		}
	}
}

// IssueTicket mints a single-use, short-lived ticket bound to a user. The
// browser passes it as ?ticket=… when opening the event stream.
func (h *Hub) IssueTicket(userID string) (string, error) {
	if h.store != nil {
		return h.store.Put(ticketKind, userID, TicketTTL)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	h.mu.Lock()
	h.sweepLocked()
	h.tickets[tok] = ticket{userID: userID, expires: h.now().Add(TicketTTL)}
	h.mu.Unlock()
	return tok, nil
}

// Redeem consumes a ticket, returning its user. A ticket is valid at most once
// and only before it expires.
func (h *Hub) Redeem(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	if h.store != nil {
		var uid string
		ok, err := h.store.Take(ticketKind, tok, &uid)
		return uid, ok && err == nil && uid != ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.tickets[tok]
	if !ok {
		return "", false
	}
	delete(h.tickets, tok) // single-use
	if h.now().After(t.expires) {
		return "", false
	}
	return t.userID, true
}

// sweepLocked drops expired tickets. Caller holds the lock.
func (h *Hub) sweepLocked() {
	now := h.now()
	for k, t := range h.tickets {
		if now.After(t.expires) {
			delete(h.tickets, k)
		}
	}
}
