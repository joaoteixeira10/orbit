package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
)

func TestExtractRoleClaim(t *testing.T) {
	if got := extractRoleClaim("admin operator"); len(got) != 2 {
		t.Errorf("space-separated string: got %v", got)
	}
	if got := extractRoleClaim([]any{"a", "b", 3}); len(got) != 2 {
		t.Errorf("[]any (non-strings skipped): got %v", got)
	}
	if got := extractRoleClaim([]string{"x"}); len(got) != 1 {
		t.Errorf("[]string: got %v", got)
	}
	if got := extractRoleClaim(nil); got != nil {
		t.Errorf("nil claim should yield nil, got %v", got)
	}
}

func TestOIDCStateSingleUse(t *testing.T) {
	o := NewOIDC(testDB(t), nil)
	state, err := o.states.Put(ephemeral.KindOIDCState, oidcState{Nonce: "n", Verifier: "v"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// First use consumes the state and proceeds past the CSRF check (then fails
	// on discovery, since no issuer is configured).
	if _, err := o.Finish(context.Background(), state, "code"); errors.Is(err, ErrOIDCState) {
		t.Fatal("a freshly issued state was rejected")
	}
	if _, err := o.Finish(context.Background(), state, "code"); !errors.Is(err, ErrOIDCState) {
		t.Fatalf("state must be single-use, got %v", err)
	}
	if _, err := o.Finish(context.Background(), "unknown", "code"); !errors.Is(err, ErrOIDCState) {
		t.Fatalf("unknown state accepted: %v", err)
	}
}

// TestDomainAllowed: with domains set, a verified email of one of them;
// Google's hosted-domain claim must agree too.
func TestDomainAllowed(t *testing.T) {
	g := OIDCSettings{Kind: "google", Domains: []string{"school.edu"}}
	o := OIDCSettings{Kind: "oidc", Domains: []string{"school.edu"}}
	ok := map[string]any{"email_verified": true, "hd": "school.edu"}
	if !domainAllowed(g, "ana@School.edu", ok) || !domainAllowed(o, "ana@school.edu", map[string]any{"email_verified": true}) {
		t.Fatal("allowed domain refused")
	}
	if domainAllowed(g, "ana@school.edu", map[string]any{"email_verified": true}) {
		t.Error("google without hd")
	}
	if domainAllowed(o, "ana@school.edu", map[string]any{}) || domainAllowed(o, "ana@evil.test", map[string]any{"email_verified": true}) {
		t.Error("unverified or other domain")
	}
	if !domainAllowed(OIDCSettings{}, "anyone@anywhere", nil) {
		t.Error("no domains: anyone")
	}
}
