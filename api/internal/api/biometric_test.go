package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
)

// TestBiometric: Face ID (a device's own lock) is off until the owner allows
// it — and needs passkeys on; then registering asks for the device's own,
// verified authenticator, and signing in with it works even with passkey
// sign-in off, aimed at the one credential the device names.
func TestBiometric(t *testing.T) {
	_, db := newTestRouterDB(t, false)
	plain := routerOn(t, db, &config.Config{GinMode: "test", AuthRateBurst: 100, AuthRatePerMinute: 6000}, nil)
	owner := loginTok(t, plain, "admin", "adminpass")
	expect(t, authdo(plain, "PUT", "/api/v1/security/config", `{"biometric_enabled":true}`, owner), http.StatusBadRequest, "no passkeys on the server")

	r := routerOn(t, db, &config.Config{GinMode: "test", AuthRateBurst: 100, AuthRatePerMinute: 6000,
		WebAuthnRPID: "localhost", WebAuthnRPOrigins: []string{"http://localhost"}, WebAuthnRPDisplayName: "Orbit"}, nil)
	owner = loginTok(t, r, "admin", "adminpass")
	var pol map[string]any
	decode(t, authdo(r, "GET", "/api/v1/security/config", "", owner), &pol)
	if pol["biometric_enabled"] != false || pol["biometric_lock_max"] != float64(0) || pol["biometric_lock_required"] != false {
		t.Fatalf("defaults: %v", pol)
	}
	// Off: no Face ID anywhere.
	expect(t, authdo(r, "POST", "/api/v1/me/webauthn/register/begin?biometric=1", "", owner), http.StatusForbidden, "register while off")
	expect(t, authdo(r, "POST", "/api/v1/auth/webauthn/login/begin?biometric=1", "", ""), http.StatusForbidden, "sign in while off")
	expect(t, authdo(r, "POST", "/api/v1/auth/step-up/webauthn/begin?biometric=1", "", owner), http.StatusForbidden, "unlock while off")

	expect(t, authdo(r, "PUT", "/api/v1/security/config", `{"biometric_lock_max":2000}`, owner), http.StatusBadRequest, "lock over a day")
	w := authdo(r, "PUT", "/api/v1/security/config", `{"biometric_enabled":true,"biometric_lock_max":15,"biometric_lock_required":true,"passwordless_enabled":false}`, owner)
	expect(t, w, http.StatusOK, "allow Face ID")
	decode(t, w, &pol)
	if pol["biometric_enabled"] != true || pol["biometric_lock_max"] != float64(15) || pol["biometric_lock_required"] != true {
		t.Fatalf("saved: %v", pol)
	}
	// The login page learns it (it's public).
	var brand map[string]any
	decode(t, authdo(r, "GET", "/api/v1/branding", "", ""), &brand)
	if brand["biometric_enabled"] != true || brand["biometric_lock_max"] != float64(15) || brand["biometric_lock_required"] != true {
		t.Fatalf("branding: %v", brand)
	}

	// Registering: this device's own authenticator, verifying the person.
	var reg struct {
		Options struct {
			AuthenticatorSelection map[string]any `json:"authenticatorSelection"`
			Hints                  []string       `json:"hints"`
		} `json:"options"`
	}
	w = authdo(r, "POST", "/api/v1/me/webauthn/register/begin?biometric=1", "", owner)
	expect(t, w, http.StatusOK, "register begin")
	decode(t, w, &reg)
	sel := reg.Options.AuthenticatorSelection
	if sel["authenticatorAttachment"] != "platform" || sel["userVerification"] != "required" || sel["residentKey"] != "required" {
		t.Fatalf("selection: %v", sel)
	}
	if len(reg.Options.Hints) != 1 || reg.Options.Hints[0] != "client-device" {
		t.Fatalf("hints: %v", reg.Options.Hints)
	}

	// Signing in: passkey sign-in is off, Face ID still works — aimed at the
	// device's credential, and verifying the person.
	expect(t, authdo(r, "POST", "/api/v1/auth/webauthn/login/begin", "", ""), http.StatusForbidden, "plain passkey sign-in is off")
	w = authdo(r, "POST", "/api/v1/auth/webauthn/login/begin?biometric=1&cred=AQIDBA", "", "")
	expect(t, w, http.StatusOK, "Face ID sign-in begin")
	var login struct {
		Options struct {
			UserVerification string `json:"userVerification"`
			AllowCredentials []struct {
				ID string `json:"id"`
			} `json:"allowCredentials"`
		} `json:"options"`
	}
	decode(t, w, &login)
	pk := login.Options
	if pk.UserVerification != "required" || len(pk.AllowCredentials) != 1 || pk.AllowCredentials[0].ID != "AQIDBA" {
		t.Fatalf("login options: %+v", pk)
	}
	// A finish without a real assertion fails.
	w = authdo(r, "POST", "/api/v1/auth/webauthn/login/finish?sid=nope", `{}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bogus finish: %d", w.Code)
	}
	var e map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	if !strings.Contains(e["error"], "failed") {
		t.Fatalf("error: %v", e)
	}
}
