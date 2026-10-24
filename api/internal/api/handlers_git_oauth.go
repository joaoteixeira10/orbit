package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/gitsync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// "Sign in with Gitea": the owner registers Orbit as an OAuth2 application on
// a server once; then each person connects their account there with one
// click instead of pasting a token. The flow's state is single-use, kept on
// the server, and tied to the browser that started it (a cookie), so a link
// someone else started can't attach your account to theirs.

const gitOAuthCookie = "orbit_git_oauth"

func (h *Handlers) gitOAuthRedirect(c *gin.Context) string {
	return strings.TrimRight(h.baseURL(c), "/") + "/api/v1/git/oauth/callback"
}

type gitOAuthAppResponse struct {
	BaseURL   string `json:"base_url"`
	ClientID  string `json:"client_id"`
	HasSecret bool   `json:"has_secret"`
	Kind      string `json:"kind"` // gitea | github | gitlab
}

type gitOAuthAppsResponse struct {
	Apps        []gitOAuthAppResponse `json:"apps"`
	RedirectURI string                `json:"redirect_uri"` // to paste in the server's application
}

func (h *Handlers) gitOAuthApps(c *gin.Context) gitOAuthAppsResponse {
	out := gitOAuthAppsResponse{Apps: []gitOAuthAppResponse{}, RedirectURI: h.gitOAuthRedirect(c)}
	for _, a := range h.git.OAuthApps() {
		out.Apps = append(out.Apps, gitOAuthAppResponse{BaseURL: a.BaseURL, ClientID: a.ClientID, HasSecret: a.ClientSecret != "", Kind: a.Kind})
	}
	return out
}

// getGitOAuthApps lists the servers people can sign in with (the owner).
//
//	@Summary	Git sign-in applications
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	gitOAuthAppsResponse
//	@Router		/git/oauth-apps [get]
func (h *Handlers) getGitOAuthApps(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	c.JSON(http.StatusOK, h.gitOAuthApps(c))
}

type gitOAuthAppsRequest struct {
	Apps []gitsync.OAuthApp `json:"apps" binding:"max=20"`
}

// setGitOAuthApps stores the servers' OAuth2 registrations (the owner); a
// blank secret keeps the stored one.
//
//	@Summary	Set git sign-in applications
//	@Tags		git
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		gitOAuthAppsRequest	true	"Registrations"
//	@Success	200		{object}	gitOAuthAppsResponse
//	@Router		/git/oauth-apps [put]
func (h *Handlers) setGitOAuthApps(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	var req gitOAuthAppsRequest
	if !bindJSON(c, &req) {
		return
	}
	apps, err := h.git.SetOAuthApps(req.Apps)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bases := make([]string, 0, len(apps))
	for _, a := range apps {
		bases = append(bases, a.BaseURL)
	}
	h.recordCtx(c, "git.oauth_apps", "settings", "git", gin.H{"servers": bases})
	c.JSON(http.StatusOK, h.gitOAuthApps(c))
}

type gitOAuthState struct {
	UserID   string `json:"user_id"`
	BaseURL  string `json:"base_url"`
	Verifier string `json:"verifier"`
	Nonce    string `json:"nonce"`
}

func randomString() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

type gitOAuthStartRequest struct {
	BaseURL string `json:"base_url" binding:"required,max=255"`
}

// startGitOAuth begins "Sign in with …": it answers the server's address to
// send the browser to (and ties the attempt to this browser).
//
//	@Summary	Start signing in with a git server
//	@Tags		git
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		gitOAuthStartRequest	true	"Server"
//	@Success	200		{object}	map[string]string
//	@Router		/me/git/oauth/start [post]
func (h *Handlers) startGitOAuth(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	var req gitOAuthStartRequest
	if !bindJSON(c, &req) {
		return
	}
	base, err := h.git.NormalizeBase(req.BaseURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	app, ok := h.git.OAuthAppFor(base)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "signing in with " + base + " isn't set up on this instance"})
		return
	}
	st := gitOAuthState{UserID: auth.UserID(c), BaseURL: base, Verifier: randomString(), Nonce: randomString()}
	state, err := h.ephemeral.Put(ephemeral.KindGitOAuth, st, 10*time.Minute)
	if err != nil {
		h.internalError(c, err, "could not start")
		return
	}
	// Lax: the server's redirect back is a top-level navigation.
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(gitOAuthCookie, st.Nonce, 600, "/api/v1/git/oauth", h.cfg.CookieDomain, h.cfg.CookieSecure, true)
	c.JSON(http.StatusOK, gin.H{"url": gitsync.AuthorizeURL(app, h.gitOAuthRedirect(c), state, st.Verifier)})
}

// gitOAuthCallback is where the server sends the browser back: the code
// becomes the person's account there. It ends on Settings → Git.
//
//	@Summary	Git sign-in callback
//	@Tags		git
//	@Router		/git/oauth/callback [get]
func (h *Handlers) gitOAuthCallback(c *gin.Context) {
	done := func(q string) {
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(gitOAuthCookie, "", -1, "/api/v1/git/oauth", h.cfg.CookieDomain, h.cfg.CookieSecure, true)
		c.Redirect(http.StatusFound, "/settings?"+q+"#git")
	}
	if !h.gitOn() {
		done("git_error=disabled")
		return
	}
	var st gitOAuthState
	ok, err := h.ephemeral.Take(ephemeral.KindGitOAuth, c.Query("state"), &st)
	cookie, _ := c.Cookie(gitOAuthCookie)
	if err != nil || !ok || cookie == "" || cookie != st.Nonce {
		done("git_error=expired")
		return
	}
	if e := c.Query("error"); e != "" {
		done("git_error=denied")
		return
	}
	app, ok := h.git.OAuthAppFor(st.BaseURL)
	if !ok {
		done("git_error=disabled")
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	acc, err := h.git.ConnectOAuth(ctx, st.UserID, app, h.gitOAuthRedirect(c), c.Query("code"), st.Verifier)
	if err != nil {
		h.log.Warn("git sign-in failed", zap.Error(err))
		done("git_error=" + url.QueryEscape(err.Error()))
		return
	}
	h.audit.Record(st.UserID, "git.connect", "user", st.UserID, c.ClientIP(), gin.H{"server": acc.BaseURL, "login": acc.Login, "via": "oauth"})
	done("git=connected")
}
