package events

import (
	"testing"
	"time"
)

func TestPublishReachesSubscriber(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe("7")
	defer cancel()

	h.Publish("7", "privileges")
	select {
	case msg := <-ch:
		if msg != "privileges" {
			t.Fatalf("got %q, want %q", msg, "privileges")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive the message")
	}
}

func TestPublishIsolatedPerUser(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe("1")
	defer cancel()

	h.Publish("2", "privileges") // different user
	select {
	case <-ch:
		t.Fatal("user 1 should not receive user 2's message")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroadcastReachesAllUsers(t *testing.T) {
	h := New()
	a, ca := h.Subscribe("1")
	defer ca()
	b, cb := h.Subscribe("2")
	defer cb()

	h.Broadcast(`{"type":"certificates"}`)
	for i, ch := range []<-chan string{a, b} {
		select {
		case msg := <-ch:
			if msg != `{"type":"certificates"}` {
				t.Fatalf("subscriber %d got %q", i, msg)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d received nothing", i)
		}
	}
}

func TestUnsubscribeStops(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe("3")
	cancel()
	if _, open := <-ch; open {
		t.Fatal("channel should be closed after unsubscribe")
	}
	h.Publish("3", "privileges") // must not panic on a closed/removed subscriber
}

func TestTicketSingleUse(t *testing.T) {
	h := New()
	tok, err := h.IssueTicket("42")
	if err != nil {
		t.Fatal(err)
	}
	if uid, ok := h.Redeem(tok); !ok || uid != "42" {
		t.Fatalf("first redeem = (%q,%v), want (42,true)", uid, ok)
	}
	if _, ok := h.Redeem(tok); ok {
		t.Fatal("second redeem of the same ticket must fail")
	}
	if _, ok := h.Redeem("nope"); ok {
		t.Fatal("unknown ticket must fail")
	}
}

func TestTicketExpiry(t *testing.T) {
	now := time.Unix(0, 0)
	h := New()
	h.now = func() time.Time { return now }

	tok, _ := h.IssueTicket("9")
	now = now.Add(TicketTTL + time.Second) // jump past expiry
	if _, ok := h.Redeem(tok); ok {
		t.Fatal("expired ticket must not redeem")
	}
}
