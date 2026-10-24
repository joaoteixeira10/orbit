package gitsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// "Sign in with Gitea": instead of pasting a personal access token, a person
// approves Orbit on the server (OAuth2 with PKCE) and the server gives Orbit a
// token of theirs. The owner registers Orbit as an OAuth2 application on each
// server once (its client id and secret, kept sealed). The access token is
// short-lived; the refresh token renews it when needed.

// OAuthApp is Orbit's registration on one server.
type OAuthApp struct {
	BaseURL      string `json:"base_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	// Kind: gitea (Gitea, Forgejo), github or gitlab — their sign-in pages
	// differ. Empty: from the address (github.com, gitlab.com), else gitea.
	Kind string `json:"kind,omitempty"`
}

// appKind is the kind of server an app is registered on.
func appKind(a OAuthApp) string {
	switch {
	case a.Kind == KindGitHub || a.Kind == KindGitLab || a.Kind == KindGitea:
		return a.Kind
	case isGitHubCom(a.BaseURL):
		return KindGitHub
	case isGitLabCom(a.BaseURL):
		return KindGitLab
	}
	return KindGitea
}

const oauthKey = "git_oauth_apps"

// stored is how the registrations are kept (sealed as a whole).
type stored struct {
	Base string `json:"b"`
	ID   string `json:"i"`
	Key  string `json:"k"`
	Kind string `json:"t,omitempty"`
}

// OAuthApps are the servers where people can sign in, with their secrets.
func (e *Engine) OAuthApps() []OAuthApp {
	var row models.Setting
	out := []OAuthApp{}
	if e.sealer == nil || e.db.Where(&models.Setting{Key: oauthKey}).First(&row).Error != nil {
		return out
	}
	blob, err := base64.StdEncoding.DecodeString(row.Value)
	if err != nil {
		return out
	}
	raw, err := e.sealer.Unseal(blob)
	if err != nil {
		return out
	}
	var rows []stored
	_ = json.Unmarshal(raw, &rows)
	for _, r := range rows {
		out = append(out, OAuthApp{BaseURL: r.Base, ClientID: r.ID, ClientSecret: r.Key, Kind: appKind(OAuthApp{BaseURL: r.Base, Kind: r.Kind})})
	}
	return out
}

// SetOAuthApps stores the registrations (sealed). A blank secret keeps the
// one already stored for that server.
func (e *Engine) SetOAuthApps(apps []OAuthApp) ([]OAuthApp, error) {
	if e.sealer == nil {
		return nil, errors.New("secrets can't be stored on this instance")
	}
	old := map[string]string{}
	for _, a := range e.OAuthApps() {
		old[a.BaseURL] = a.ClientSecret
	}
	clean := []OAuthApp{}
	seen := map[string]bool{}
	for _, a := range apps {
		base, err := e.NormalizeBase(a.BaseURL)
		if err != nil {
			return nil, err
		}
		a.BaseURL, a.ClientID, a.ClientSecret = base, strings.TrimSpace(a.ClientID), strings.TrimSpace(a.ClientSecret)
		a.Kind = appKind(a)
		if a.ClientSecret == "" {
			a.ClientSecret = old[base]
		}
		if a.ClientID == "" || a.ClientSecret == "" {
			return nil, fmt.Errorf("give the client id and secret for %s", base)
		}
		if !seen[base] {
			seen[base] = true
			clean = append(clean, a)
		}
	}
	rows := make([]stored, 0, len(clean))
	for _, a := range clean {
		rows = append(rows, stored{Base: a.BaseURL, ID: a.ClientID, Key: a.ClientSecret, Kind: a.Kind})
	}
	b, _ := json.Marshal(rows)
	blob, err := e.sealer.Seal(b)
	if err != nil {
		return nil, err
	}
	return clean, e.db.Save(&models.Setting{Key: oauthKey, Value: base64.StdEncoding.EncodeToString(blob), UpdatedAt: time.Now()}).Error
}

// OAuthAppFor is the registration on base, if any.
func (e *Engine) OAuthAppFor(base string) (OAuthApp, bool) {
	for _, a := range e.OAuthApps() {
		if a.BaseURL == base {
			return a, true
		}
	}
	return OAuthApp{}, false
}

// Challenge is PKCE's S256 code challenge of a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL is where the person approves Orbit on the server.
func AuthorizeURL(app OAuthApp, redirect, state, verifier string) string {
	q := url.Values{
		"client_id":             {app.ClientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"state":                 {state},
		"code_challenge":        {Challenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	switch appKind(app) {
	case KindGitHub:
		// GitHub grants what's asked: the repositories (issues, labels,
		// comments) and who the person is.
		q.Set("scope", "repo read:user")
	case KindGitLab:
		// GitLab: the API (issues, labels, notes, hooks) and who they are.
		q.Set("scope", "api read_user")
		return app.BaseURL + "/oauth/authorize?" + q.Encode()
	}
	return app.BaseURL + "/login/oauth/authorize?" + q.Encode()
}

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// tokenCall posts to the server's token endpoint.
func (e *Engine) tokenCall(ctx context.Context, app OAuthApp, body map[string]string) (oauthToken, error) {
	var tok oauthToken
	body["client_id"], body["client_secret"] = app.ClientID, app.ClientSecret
	var payload []byte
	ctype := "application/json"
	endpoint := app.BaseURL + "/login/oauth/access_token"
	if appKind(app) == KindGitLab {
		endpoint = app.BaseURL + "/oauth/token"
	}
	if appKind(app) != KindGitea {
		// GitHub's and GitLab's token endpoints take a form.
		form := url.Values{}
		for k, v := range body {
			form.Set(k, v)
		}
		payload, ctype = []byte(form.Encode()), "application/x-www-form-urlencoded"
	} else {
		payload, _ = json.Marshal(body)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return tok, err
	}
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Accept", "application/json")
	resp, err := e.clientFor(app.BaseURL).Do(req)
	if err != nil {
		return tok, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = json.Unmarshal(data, &tok)
	if resp.StatusCode >= 300 || tok.AccessToken == "" {
		msg := tok.Description
		if msg == "" {
			msg = tok.Error
		}
		if msg == "" {
			msg = resp.Status
		}
		return tok, errors.New("the server refused: " + msg)
	}
	return tok, nil
}

// ConnectOAuth finishes a sign-in: the code becomes tokens, the account is
// saved (sealed) as the user's on that server, as with a pasted token.
func (e *Engine) ConnectOAuth(ctx context.Context, userID string, app OAuthApp, redirect, code, verifier string) (models.GitAccount, error) {
	tok, err := e.tokenCall(ctx, app, map[string]string{
		"grant_type": "authorization_code", "code": code, "redirect_uri": redirect, "code_verifier": verifier,
	})
	if err != nil {
		return models.GitAccount{}, err
	}
	c := &Client{base: app.BaseURL, kind: appKind(app), token: tok.AccessToken, http: e.clientFor(app.BaseURL)}
	me, err := c.Me(ctx)
	if err != nil {
		return models.GitAccount{}, fmt.Errorf("couldn't read your account: %w", err)
	}
	acc := models.GitAccount{UserID: userID, BaseURL: app.BaseURL}
	e.db.Where("user_id = ? AND base_url = ?", userID, app.BaseURL).First(&acc)
	acc.Kind = c.kind
	if err := e.storeTokens(&acc, tok); err != nil {
		return acc, err
	}
	now := time.Now()
	acc.Login, acc.LastError, acc.CheckedAt, acc.Via = me.Login, "", &now, "oauth"
	return acc, e.db.Save(&acc).Error
}

func (e *Engine) storeTokens(acc *models.GitAccount, tok oauthToken) error {
	sealed, err := e.sealer.Seal([]byte(tok.AccessToken))
	if err != nil {
		return err
	}
	acc.TokenSealed = sealed
	acc.RefreshSealed = nil
	if tok.RefreshToken != "" {
		if acc.RefreshSealed, err = e.sealer.Seal([]byte(tok.RefreshToken)); err != nil {
			return err
		}
	}
	acc.ExpiresAt = nil
	if tok.ExpiresIn > 0 {
		at := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		acc.ExpiresAt = &at
	}
	return nil
}

// refresh renews a signed-in account's access token when it's about to
// expire; the caller then reads the new one.
func (e *Engine) refresh(acc *models.GitAccount) error {
	if acc.ExpiresAt == nil || time.Until(*acc.ExpiresAt) > 2*time.Minute || len(acc.RefreshSealed) == 0 {
		return nil
	}
	app, ok := e.OAuthAppFor(acc.BaseURL)
	if !ok {
		return errors.New("signing in with " + acc.BaseURL + " was turned off: connect the account again")
	}
	rt, err := e.sealer.Unseal(acc.RefreshSealed)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tok, err := e.tokenCall(ctx, app, map[string]string{"grant_type": "refresh_token", "refresh_token": string(rt)})
	if err != nil {
		e.db.Model(acc).Update("last_error", "sign in again: "+err.Error())
		return errors.New("your sign-in with " + acc.BaseURL + " expired: sign in again (Settings → Git)")
	}
	if err := e.storeTokens(acc, tok); err != nil {
		return err
	}
	return e.db.Model(acc).Updates(map[string]any{"token_sealed": acc.TokenSealed, "refresh_sealed": acc.RefreshSealed, "expires_at": acc.ExpiresAt, "last_error": ""}).Error
}
