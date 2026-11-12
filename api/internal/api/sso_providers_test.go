package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// fakeIdP is an OpenID Connect provider for tests: discovery, keys, an
// authorize step that answers with the code, and a token endpoint that signs
// an ID token with the claims set for the next sign-in.
type fakeIdP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	nonces map[string]string // code → nonce
	claims map[string]any    // the next sign-in's
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, nonces: map[string]string{}}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	iss := f.srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": iss, "authorization_endpoint": iss + "/authorize", "token_endpoint": iss + "/token",
			"jwks_uri": iss + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		b64 := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		code := "code-" + r.URL.Query().Get("state")[:8]
		f.mu.Lock()
		f.nonces[code] = r.URL.Query().Get("nonce")
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "state": r.URL.Query().Get("state")})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		nonce := f.nonces[r.Form.Get("code")]
		claims := jwt.MapClaims{"iss": iss, "aud": r.Form.Get("client_id"), "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce}
		for k, v := range f.claims {
			claims[k] = v
		}
		f.mu.Unlock()
		if claims["aud"] == "" {
			user, _, _ := r.BasicAuth()
			claims["aud"] = user
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "k1"
		signed, err := tok.SignedString(key)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": signed, "expires_in": 3600})
	})
	t.Cleanup(f.srv.Close)
	return f
}

// signIn goes through the whole SSO flow with a provider: Orbit's login
// redirect, the IdP, the callback, the code exchange. It returns the access
// token, or the sso_error the login page would get.
func (f *fakeIdP) signIn(t *testing.T, r *gin.Engine, provider string, claims map[string]any) (string, string) {
	t.Helper()
	f.mu.Lock()
	f.claims = claims
	f.mu.Unlock()
	w := do(r, "GET", "/api/v1/auth/oidc/login?provider="+provider, "")
	if w.Code != http.StatusFound {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	resp, err := http.Get(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Code, State string }
	_ = json.NewDecoder(resp.Body).Decode(&got)
	_ = resp.Body.Close()
	w = do(r, "GET", "/api/v1/auth/oidc/callback?state="+url.QueryEscape(got.State)+"&code="+url.QueryEscape(got.Code), "")
	loc, _ := url.Parse(w.Header().Get("Location"))
	if e := loc.Query().Get("sso_error"); e != "" {
		return "", e
	}
	w = do(r, "POST", "/api/v1/auth/oidc/exchange", `{"code":"`+loc.Query().Get("sso")+`"}`)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	if tok.AccessToken == "" {
		t.Fatalf("exchange: %d %s", w.Code, w.Body.String())
	}
	return tok.AccessToken, ""
}

func setProviders(t *testing.T, db *gorm.DB, list []auth.OIDCSettings) {
	t.Helper()
	b, _ := json.Marshal(list)
	if err := db.Save(&models.Setting{Key: "oidc_providers", Value: string(b)}).Error; err != nil {
		t.Fatal(err)
	}
}

// TestSSOProviders: two providers side by side, each with its button; a
// domain limit; the provider's groups kept as Orbit groups — joined, left,
// and with them their workspaces.
func TestSSOProviders(t *testing.T) {
	old := auth.OIDCClient
	auth.OIDCClient = func() *http.Client { return &http.Client{Timeout: 10 * time.Second} }
	defer func() { auth.OIDCClient = old }()

	r, db, owner, alice, _ := plannerSetupDB(t)
	corp, school := newFakeIdP(t), newFakeIdP(t)
	setProviders(t, db, []auth.OIDCSettings{
		{ID: "corp", Name: "Corp", Kind: "oidc", Enabled: true, Issuer: corp.srv.URL, ClientID: "orbit", RedirectURL: "http://orbit.test/api/v1/auth/oidc/callback", GroupClaim: "groups"},
		{ID: "school", Name: "School", Kind: "oidc", Enabled: true, Issuer: school.srv.URL, ClientID: "orbit", RedirectURL: "http://orbit.test/api/v1/auth/oidc/callback", Domains: []string{"school.edu"}},
		{ID: "off", Name: "Off", Kind: "oidc", Enabled: false, Issuer: "https://nowhere.test", ClientID: "x"},
	})

	// The login page shows a button per usable provider.
	var b struct {
		Providers []struct{ ID, Name string } `json:"sso_providers"`
	}
	decode(t, do(r, "GET", "/api/v1/branding", ""), &b)
	if len(b.Providers) != 2 || b.Providers[0].ID != "corp" || b.Providers[1].Name != "School" {
		t.Fatalf("buttons: %+v", b.Providers)
	}

	// Corp: signs in, put in its groups (made here, marked as the provider's).
	tok, fail := corp.signIn(t, r, "corp", map[string]any{"sub": "u-1", "email": "dana@corp.test", "email_verified": true, "name": "Dana", "preferred_username": "dana", "groups": []string{"Engineering", "Design"}})
	if fail != "" {
		t.Fatalf("corp sign-in: %s", fail)
	}
	var groups []struct {
		ID, Name, Source string
		Members          int
	}
	decode(t, authdo(r, "GET", "/api/v1/groups", "", owner), &groups)
	var eng string
	for _, g := range groups {
		if g.Name == "Engineering" && g.Source == "oidc" && g.Members == 1 {
			eng = g.ID
		}
	}
	if eng == "" || len(groups) != 2 {
		t.Fatalf("groups: %+v", groups)
	}
	expect(t, authdo(r, "POST", "/api/v1/groups/"+eng+"/members", `{"user":"alice"}`, owner), http.StatusConflict, "kept by the IdP")

	// Engineering in a workspace: Dana is in it — until the IdP says she isn't.
	ws := createWS(t, r, alice, `{"name":"Builds","kind":"work","starter":false,"modules":["home","pages"]}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/groups", `{"group_id":"`+eng+`","role":"editor"}`, alice), http.StatusOK, "engineering in")
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID, "", tok), http.StatusOK, "dana in")
	tok, _ = corp.signIn(t, r, "corp", map[string]any{"sub": "u-1", "email": "dana@corp.test", "email_verified": true, "name": "Dana", "groups": []string{"Design"}})
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID, "", tok), http.StatusNotFound, "dana out with the group")

	// School: only verified school.edu accounts; a different account from
	// Corp's same subject (subjects are per provider).
	if _, fail := school.signIn(t, r, "school", map[string]any{"sub": "u-1", "email": "dana@gmail.test", "email_verified": true}); fail != "domain" {
		t.Fatalf("other domain: %q", fail)
	}
	if _, fail := school.signIn(t, r, "school", map[string]any{"sub": "u-1", "email": "dana@school.edu", "email_verified": false}); fail != "domain" {
		t.Fatalf("unverified: %q", fail)
	}
	st, fail := school.signIn(t, r, "school", map[string]any{"sub": "u-1", "email": "dana@school.edu", "email_verified": true, "preferred_username": "dana"})
	if fail != "" {
		t.Fatalf("school sign-in: %s", fail)
	}
	if meID(t, r, st) == meID(t, r, tok) {
		t.Fatal("the same account through two providers' same subject")
	}

	// A provider that's off can't be used.
	if w := do(r, "GET", "/api/v1/auth/oidc/login?provider=off", ""); !strings.Contains(w.Header().Get("Location"), "sso_error=disabled") {
		t.Fatalf("off provider: %s", w.Header().Get("Location"))
	}
}
