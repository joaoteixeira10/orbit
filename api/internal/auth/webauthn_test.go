package auth

import (
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
)

func TestChallengeStoreSingleUse(t *testing.T) {
	w := &WebAuthn{ch: ephemeral.New(testDB(t))}
	id, err := w.putChallenge(challengeEntry{UserID: "7"})
	if err != nil || id == "" {
		t.Fatalf("put: id=%q err=%v", id, err)
	}
	e, ok := w.takeChallenge(id)
	if !ok || e.UserID != "7" {
		t.Fatalf("take after put failed: ok=%v entry=%+v", ok, e)
	}
	if _, ok := w.takeChallenge(id); ok {
		t.Fatal("challenge must be single-use (second take should fail)")
	}
	if _, ok := w.takeChallenge("does-not-exist"); ok {
		t.Fatal("unknown id should not be accepted")
	}
}

func TestDummyCompareDoesNotPanic(t *testing.T) {
	// dummyHash must be a valid bcrypt hash; DummyCompare runs a constant-cost
	// comparison and must never panic on the no-such-user path.
	DummyCompare("whatever")
}
