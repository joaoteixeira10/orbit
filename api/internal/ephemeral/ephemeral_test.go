package ephemeral

import (
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.EphemeralToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

type payload struct {
	A string `json:"a"`
}

func TestPutTakeSingleUse(t *testing.T) {
	s := testStore(t)
	id, err := s.Put(KindOIDCState, payload{A: "x"}, time.Minute)
	if err != nil || id == "" {
		t.Fatalf("put: id=%q err=%v", id, err)
	}
	var got payload
	if ok, err := s.Take(KindOIDCState, id, &got); !ok || err != nil || got.A != "x" {
		t.Fatalf("take: ok=%v err=%v got=%+v", ok, err, got)
	}
	if ok, _ := s.Take(KindOIDCState, id, &got); ok {
		t.Fatal("token must be single-use")
	}
}

func TestTakeRejectsWrongKindUnknownAndEmpty(t *testing.T) {
	s := testStore(t)
	id, _ := s.Put(KindSSOCode, payload{}, time.Minute)
	if ok, _ := s.Take(KindWebAuthn, id, nil); ok {
		t.Fatal("an id must only redeem for its own kind")
	}
	if ok, _ := s.Take(KindSSOCode, id, nil); !ok {
		t.Fatal("a wrong-kind attempt must not consume the token")
	}
	if ok, _ := s.Take(KindSSOCode, "nope", nil); ok {
		t.Fatal("unknown id accepted")
	}
	if ok, _ := s.Take(KindSSOCode, "", nil); ok {
		t.Fatal("empty id accepted")
	}
}

func TestExpiryAndSweep(t *testing.T) {
	s := testStore(t)
	base := time.Now()
	s.now = func() time.Time { return base }
	id, _ := s.Put(KindSSETicket, payload{}, 10*time.Second)
	keep, _ := s.Put(KindSSETicket, payload{}, time.Hour)

	s.now = func() time.Time { return base.Add(time.Minute) }
	if ok, _ := s.Take(KindSSETicket, id, nil); ok {
		t.Fatal("expired token accepted")
	}
	_, _ = s.Put(KindSSETicket, payload{}, time.Second) // expires at base+61s
	s.now = func() time.Time { return base.Add(2 * time.Minute) }
	if n, err := s.Sweep(); err != nil || n != 1 {
		t.Fatalf("sweep: n=%d err=%v (want 1)", n, err)
	}
	if ok, _ := s.Take(KindSSETicket, keep, nil); !ok {
		t.Fatal("sweep removed an unexpired token")
	}
}

func TestStoresOnlyHash(t *testing.T) {
	s := testStore(t)
	id, _ := s.Put(KindWebAuthn, payload{}, time.Minute)
	var n int64
	s.db.Model(&models.EphemeralToken{}).Where("key_hash = ?", id).Count(&n)
	if n != 0 {
		t.Fatal("the plaintext id must not be stored")
	}
}
