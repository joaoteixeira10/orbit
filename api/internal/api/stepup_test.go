package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestStepUp covers the step-up gate: a no-op while the flag is off, then
// (flag on) a gated route 403s until a fresh password re-auth is presented.
func TestStepUp(t *testing.T) {
	r := newTestRouter(t)
	owner := loginTok(t, r, "admin", "adminpass")

	// Flag off → a gated owner route proceeds without any step-up.
	if w := authdo(r, "PUT", "/api/v1/security/config", `{"otp_enforced":false}`, owner); w.Code != 200 {
		t.Fatalf("security/config with step-up off: got %d, want 200", w.Code)
	}

	// Enable step-up. PUT /features is itself step-up-gated, but the flag is
	// still off at this instant, so the gate is a no-op and the write succeeds.
	if w := authdo(r, "PUT", "/api/v1/features", `{"step_up_auth":true}`, owner); w.Code != 200 {
		t.Fatalf("enable step_up_auth: got %d, body=%s", w.Code, w.Body.String())
	}

	// Now the gated route requires step-up.
	w := authdo(r, "PUT", "/api/v1/security/config", `{"otp_enforced":false}`, owner)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "step_up_required") {
		t.Fatalf("gated route without step-up: got %d body=%s, want 403 step_up_required", w.Code, w.Body.String())
	}

	// Wrong password is rejected.
	if w := authdo(r, "POST", "/api/v1/auth/step-up", `{"password":"wrong"}`, owner); w.Code != 401 {
		t.Errorf("step-up wrong password: got %d, want 401", w.Code)
	}

	// Correct password mints a step-up token.
	su := authdo(r, "POST", "/api/v1/auth/step-up", `{"password":"adminpass"}`, owner)
	if su.Code != 200 {
		t.Fatalf("step-up: got %d, body=%s", su.Code, su.Body.String())
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(su.Body.Bytes(), &tok)
	if tok.AccessToken == "" {
		t.Fatal("step-up returned no access token")
	}

	// The step-up token clears the gate.
	if w := authdo(r, "PUT", "/api/v1/security/config", `{"otp_enforced":false}`, tok.AccessToken); w.Code != 200 {
		t.Errorf("gated route with step-up token: got %d, body=%s, want 200", w.Code, w.Body.String())
	}

	// A plain (non-step-up) token is still rejected afterwards.
	if w := authdo(r, "PUT", "/api/v1/security/config", `{"otp_enforced":false}`, owner); w.Code != 403 {
		t.Errorf("gated route with plain token after step-up: got %d, want 403", w.Code)
	}
}
