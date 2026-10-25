package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/google"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Google Calendar, two ways: a workspace owner signs in with Google, picks
// one of their calendars, and the workspace's calendar is kept in step with
// it — over Google's CalDAV interface, by the CalDAV syncer, with an access
// token (refreshed here) in place of a password. The instance's owner
// registers the OAuth client first.

const googleOAuthCookie = "orbit_google_oauth"

func (h *Handlers) googleRedirect(c *gin.Context) string {
	return strings.TrimRight(h.baseURL(c), "/") + "/api/v1/google/oauth/callback"
}

type googleAppResponse struct {
	ClientID    string `json:"client_id"`
	HasSecret   bool   `json:"has_secret"`
	RedirectURI string `json:"redirect_uri"` // to add in Google Cloud's OAuth client
}

// getGoogleApp returns the instance's Google OAuth client (owner; the secret
// is never shown).
//
//	@Summary	Google sign-in for Calendar
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	googleAppResponse
//	@Router		/google/app [get]
func (h *Handlers) getGoogleApp(c *gin.Context) {
	app, _ := google.LoadApp(h.db, h.sealer)
	c.JSON(http.StatusOK, googleAppResponse{ClientID: app.ClientID, HasSecret: app.ClientSecret != "", RedirectURI: h.googleRedirect(c)})
}

type googleAppRequest struct {
	ClientID     string `json:"client_id" binding:"max=300"`
	ClientSecret string `json:"client_secret" binding:"max=300"` // "" keeps the one there is
}

// setGoogleApp sets (or, with no client id, removes) the instance's Google
// OAuth client (owner).
//
//	@Summary	Set Google sign-in for Calendar
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		googleAppRequest	true	"The OAuth client"
//	@Success	200		{object}	googleAppResponse
//	@Router		/google/app [put]
func (h *Handlers) setGoogleApp(c *gin.Context) {
	var req googleAppRequest
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.ClientID) != "" && strings.TrimSpace(req.ClientSecret) == "" {
		if old, _ := google.LoadApp(h.db, h.sealer); old.ClientSecret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the client secret is needed too"})
			return
		}
	}
	if err := google.SaveApp(h.db, h.sealer, req.ClientID, req.ClientSecret); err != nil {
		h.internalError(c, err, "could not save it")
		return
	}
	h.recordCtx(c, "google.app", "settings", "google", gin.H{"set": strings.TrimSpace(req.ClientID) != ""})
	h.getGoogleApp(c)
}

// googleStatus says whether workspace owners can sign in with Google here.
//
//	@Summary	Is Google Calendar sync available
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]bool
//	@Router		/google/status [get]
func (h *Handlers) googleStatus(c *gin.Context) {
	_, ok := google.LoadApp(h.db, h.sealer)
	c.JSON(http.StatusOK, gin.H{"available": ok && feature.Enabled(h.db, feature.CalDAVSync)})
}

type googleOAuthState struct {
	UserID   string `json:"u"`
	WS       string `json:"w"`
	Verifier string `json:"v"`
	Nonce    string `json:"n"`
}

// googlePending is a sign-in done, waiting for a calendar to be picked.
type googlePending struct {
	UserID  string `json:"u"`
	WS      string `json:"w"`
	Refresh []byte `json:"r"` // sealed
	Email   string `json:"e"`
}

// startGoogle begins signing in with Google for a workspace's calendar
// (owners): the address to send the browser to.
//
//	@Summary	Sign in with Google for Calendar
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	map[string]string
//	@Router		/workspaces/{ws}/google/start [post]
func (h *Handlers) startGoogle(c *gin.Context) {
	ws, ok := h.caldavOwner(c)
	if !ok {
		return
	}
	app, ok := google.LoadApp(h.db, h.sealer)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "signing in with Google isn't set up on this instance"})
		return
	}
	st := googleOAuthState{UserID: auth.UserID(c), WS: ws.ID, Verifier: randomString(), Nonce: randomString()}
	state, err := h.ephemeral.Put(ephemeral.KindGoogleOAuth, st, 10*time.Minute)
	if err != nil {
		h.internalError(c, err, "could not start")
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(googleOAuthCookie, st.Nonce, 600, "/api/v1/google/oauth", h.cfg.CookieDomain, h.cfg.CookieSecure, true)
	c.JSON(http.StatusOK, gin.H{"url": google.AuthorizeURL(app, h.googleRedirect(c), state, st.Verifier)})
}

// googleCallback is where Google sends the browser back: the sign-in is
// kept a few minutes for a calendar to be picked, on the workspace's
// calendar page.
//
//	@Summary	Google sign-in callback
//	@Tags		calendar
//	@Router		/google/oauth/callback [get]
func (h *Handlers) googleCallback(c *gin.Context) {
	var st googleOAuthState
	done := func(ws, q string) {
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(googleOAuthCookie, "", -1, "/api/v1/google/oauth", h.cfg.CookieDomain, h.cfg.CookieSecure, true)
		if ws == "" {
			c.Redirect(http.StatusFound, "/?"+q)
			return
		}
		c.Redirect(http.StatusFound, "/w/"+url.PathEscape(ws)+"/calendar?"+q)
	}
	ok, err := h.ephemeral.Take(ephemeral.KindGoogleOAuth, c.Query("state"), &st)
	cookie, _ := c.Cookie(googleOAuthCookie)
	if err != nil || !ok || cookie == "" || cookie != st.Nonce {
		done(st.WS, "google_error=expired")
		return
	}
	if c.Query("error") != "" {
		done(st.WS, "google_error=denied")
		return
	}
	app, ok := google.LoadApp(h.db, h.sealer)
	if !ok {
		done(st.WS, "google_error=disabled")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	tok, err := google.Exchange(ctx, app, h.googleRedirect(c), c.Query("code"), st.Verifier)
	var email string
	if err == nil {
		email, err = google.Email(ctx, tok.Access)
	}
	var sealed []byte
	if err == nil {
		sealed, err = h.sealer.Seal([]byte(tok.Refresh))
	}
	if err != nil {
		h.log.Warn("google sign-in failed", zap.Error(err))
		done(st.WS, "google_error="+url.QueryEscape(err.Error()))
		return
	}
	key, err := h.ephemeral.Put(ephemeral.KindGooglePending, googlePending{UserID: st.UserID, WS: st.WS, Refresh: sealed, Email: email}, 15*time.Minute)
	if err != nil {
		done(st.WS, "google_error=failed")
		return
	}
	h.googleTokens.put("pending:"+key, tok)
	done(st.WS, "google="+url.QueryEscape(key))
}

// pendingGoogle reads a sign-in waiting for its calendar (the same owner,
// the same workspace); take removes it.
func (h *Handlers) pendingGoogle(c *gin.Context, ws, key string, take bool) (googlePending, bool) {
	var p googlePending
	var ok bool
	var err error
	if take {
		ok, err = h.ephemeral.Take(ephemeral.KindGooglePending, key, &p)
	} else {
		ok, err = h.ephemeral.Peek(ephemeral.KindGooglePending, key, &p)
	}
	if err != nil || !ok || p.UserID != auth.UserID(c) || p.WS != ws {
		c.JSON(http.StatusGone, gin.H{"error": "the Google sign-in has expired: sign in again"})
		return p, false
	}
	return p, true
}

// googleCalendars lists the signed-in account's calendars, to pick one (owners).
//
//	@Summary	A Google account's calendars
//	@Tags		calendar
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string	true	"Workspace id"
//	@Param		pending	query		string	true	"The sign-in (from the callback)"
//	@Success	200		{object}	map[string]any
//	@Router		/workspaces/{ws}/google/calendars [get]
func (h *Handlers) googleCalendars(c *gin.Context) {
	ws, ok := h.caldavOwner(c)
	if !ok {
		return
	}
	p, ok := h.pendingGoogle(c, ws.ID, c.Query("pending"), false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	access, err := h.googleAccess(ctx, "pending:"+c.Query("pending"), p.Refresh)
	var cals []google.Calendar
	if err == nil {
		cals, err = google.Calendars(ctx, access)
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"email": p.Email, "calendars": cals})
}

type googleLinkRequest struct {
	Pending      string `json:"pending" binding:"required"`
	CalendarID   string `json:"calendar_id" binding:"required,max=500"`
	CalendarName string `json:"calendar_name" binding:"max=200"`
}

// linkGoogle links the workspace's calendar to the Google calendar picked
// and runs the first sync (owners).
//
//	@Summary	Sync with a Google calendar
//	@Tags		calendar
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		body	body		googleLinkRequest	true	"The sign-in and the calendar"
//	@Success	200		{object}	caldavLinkResponse
//	@Router		/workspaces/{ws}/google/link [post]
func (h *Handlers) linkGoogle(c *gin.Context) {
	ws, ok := h.caldavOwner(c)
	if !ok {
		return
	}
	var req googleLinkRequest
	if !bindJSON(c, &req) {
		return
	}
	p, ok := h.pendingGoogle(c, ws.ID, req.Pending, true)
	if !ok {
		return
	}
	link := models.CalDAVLink{
		WorkspaceID: ws.ID, Provider: "google", CalendarURL: google.CalDAVURL(req.CalendarID), CalendarName: strings.TrimSpace(req.CalendarName),
		Host: "calendar.google.com", Username: p.Email, PasswordSealed: p.Refresh, CreatedBy: auth.UserID(c),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var old models.CalDAVLink
		if tx.Where("workspace_id = ?", ws.ID).First(&old).Error == nil {
			if err := tx.Where("link_id = ?", old.ID).Delete(&models.CalDAVItem{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}
		return tx.Create(&link).Error
	})
	if err != nil {
		h.internalError(c, err, "could not save the calendar sync")
		return
	}
	if tok, ok := h.googleTokens.get("pending:" + req.Pending); ok {
		h.googleTokens.put(link.ID, tok)
	}
	h.googleTokens.drop("pending:" + req.Pending)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	_ = h.dav.Sync(ctx, link.ID) // the error is on the link
	h.db.First(&link, "id = ?", link.ID)
	h.recordCtx(c, "workspace.caldav_link", "workspace", ws.ID, gin.H{"host": link.Host, "calendar": link.CalendarName, "provider": "google"})
	h.publishWS(c, ws.ID, "events")
	c.JSON(http.StatusOK, caldavLinkResponse{Link: &link})
}

// ---- Access tokens: kept while they last, refreshed when they don't ----

type tokenCache struct {
	mu sync.Mutex
	m  map[string]google.Token
}

func (t *tokenCache) get(k string) (google.Token, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tok, ok := t.m[k]
	return tok, ok && time.Until(tok.Expiry) > time.Minute
}

func (t *tokenCache) put(k string, tok google.Token) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]google.Token{}
	}
	t.m[k] = tok
}

func (t *tokenCache) drop(k string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, k)
}

// googleAccess is an access token for a sealed refresh token (cached by key).
func (h *Handlers) googleAccess(ctx context.Context, key string, sealedRefresh []byte) (string, error) {
	if tok, ok := h.googleTokens.get(key); ok {
		return tok.Access, nil
	}
	app, ok := google.LoadApp(h.db, h.sealer)
	if !ok {
		return "", errors.New("signing in with Google isn't set up on this instance any more")
	}
	refresh, err := h.sealer.Unseal(sealedRefresh)
	if err != nil {
		return "", errors.New("the saved Google sign-in can't be read (was the master key changed?)")
	}
	tok, err := google.Refresh(ctx, app, string(refresh))
	if err != nil {
		return "", err
	}
	h.googleTokens.put(key, tok)
	return tok.Access, nil
}

// googleBearer gives the CalDAV syncer a Google link's access token.
func (h *Handlers) googleBearer(ctx context.Context, link models.CalDAVLink) (string, error) {
	if link.Provider != "google" {
		return "", errors.New("unknown sign-in provider " + link.Provider)
	}
	return h.googleAccess(ctx, link.ID, link.PasswordSealed)
}
