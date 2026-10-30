package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// OAuth for MCP clients (OAuth 2.1 with PKCE, the MCP authorization spec):
// an app registers itself (RFC 7591), sends the person to Orbit's page
// /oauth/authorize, where they allow it (to read and change, or only to
// read); the app trades the code for an access token — an API token of
// theirs, named after the app, listed and revocable in Settings → API
// tokens. Orbit describes itself at /.well-known/oauth-authorization-server
// and the MCP endpoint at /.well-known/oauth-protected-resource (RFC 8414,
// RFC 9728).

const oauthCodeKind = "oauth_code"

// openCORS lets any page call these (they take no cookies: bearer tokens,
// and public clients with PKCE).
func openCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Protocol-Version, Mcp-Session-Id")
		c.Header("Access-Control-Expose-Headers", "WWW-Authenticate, Mcp-Session-Id")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// oauthResource describes the MCP endpoint and who signs in to it
// (/.well-known/oauth-protected-resource, RFC 9728; outside /api/v1).
func (h *Handlers) oauthResource(c *gin.Context) {
	base := h.baseURL(c)
	c.JSON(http.StatusOK, gin.H{
		"resource":                 base + "/mcp",
		"authorization_servers":    []string{base},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"orbit", "orbit:read"},
		"resource_name":            "Orbit",
	})
}

// oauthServer describes Orbit's OAuth (/.well-known/oauth-authorization-server,
// RFC 8414; outside /api/v1).
func (h *Handlers) oauthServer(c *gin.Context) {
	base := h.baseURL(c)
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/api/v1/oauth/token",
		"registration_endpoint":                 base + "/api/v1/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"orbit", "orbit:read"},
	})
}

// okRedirect: an https address; a loopback http one (an app on the
// computer); or an app's own scheme (cursor://…) — never javascript:,
// data: or file:.
func okRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Fragment != "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return u.Host != ""
	case "http":
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	case "javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss":
		return false
	default:
		return strings.Contains(u.Scheme, ".") || len(u.Scheme) > 2 // a private-use scheme (RFC 8252)
	}
}

type oauthRegisterRequest struct {
	ClientName   string   `json:"client_name" binding:"max=200"`
	RedirectURIs []string `json:"redirect_uris" binding:"required,min=1,max=10,dive,max=2000"`
}

// oauthRegister registers an app (RFC 7591; public clients only).
//
//	@Summary	Register an OAuth app
//	@Tags		mcp
//	@Accept		json
//	@Produce	json
//	@Param		body	body		oauthRegisterRequest	true	"Its name and where to come back"
//	@Success	201		{object}	map[string]any
//	@Router		/oauth/register [post]
func (h *Handlers) oauthRegister(c *gin.Context) {
	var req oauthRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client_metadata", "error_description": "redirect_uris are needed"})
		return
	}
	for _, r := range req.RedirectURIs {
		if !okRedirect(r) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_redirect_uri", "error_description": "not a redirect Orbit allows: " + r})
			return
		}
	}
	name := strings.TrimSpace(req.ClientName)
	if name == "" {
		name = "An app"
	}
	uris, _ := json.Marshal(req.RedirectURIs)
	cl := models.OAuthClient{Name: name, RedirectURIs: string(uris)}
	if err := h.db.Create(&cl).Error; err != nil {
		h.internalError(c, err, "could not register the app")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"client_id": cl.ID, "client_name": cl.Name, "redirect_uris": req.RedirectURIs, "client_id_issued_at": cl.CreatedAt.Unix(),
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"},
	})
}

func (h *Handlers) oauthClient(id string) (models.OAuthClient, []string, bool) {
	var cl models.OAuthClient
	if id == "" || h.db.First(&cl, "id = ?", id).Error != nil {
		return cl, nil, false
	}
	var uris []string
	_ = json.Unmarshal([]byte(cl.RedirectURIs), &uris)
	return cl, uris, true
}

// oauthClientInfo tells the consent page who's asking.
//
//	@Summary	An OAuth app (for its consent page)
//	@Tags		mcp
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string	true	"Client id"
//	@Param		redirect_uri	query		string	true	"Where it wants to come back"
//	@Success	200				{object}	map[string]any
//	@Router		/oauth/clients/{id} [get]
func (h *Handlers) oauthClientInfo(c *gin.Context) {
	cl, uris, ok := h.oauthClient(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "this app isn't registered with Orbit"})
		return
	}
	redirect := c.Query("redirect_uri")
	if !contains(uris, redirect) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the app asked to come back to an address it didn't register"})
		return
	}
	u, _ := url.Parse(redirect)
	c.JSON(http.StatusOK, gin.H{"client_name": cl.Name, "redirect_host": u.Host, "redirect_scheme": u.Scheme})
}

type oauthAuthorizeRequest struct {
	ClientID            string `json:"client_id" binding:"required"`
	RedirectURI         string `json:"redirect_uri" binding:"required"`
	State               string `json:"state" binding:"max=2000"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	ReadOnly            bool   `json:"read_only"`
	Deny                bool   `json:"deny"`
}

type oauthCode struct {
	ClientID  string `json:"c"`
	Redirect  string `json:"r"`
	Challenge string `json:"x"`
	UserID    string `json:"u"`
	ReadOnly  bool   `json:"ro"`
}

// oauthAuthorize is the person's answer on the consent page: allowed (a code
// for the app) or not; either way, where to send the browser.
//
//	@Summary	Allow an OAuth app (or not)
//	@Tags		mcp
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		oauthAuthorizeRequest	true	"The request and the answer"
//	@Success	200		{object}	map[string]string
//	@Router		/oauth/authorize [post]
func (h *Handlers) oauthAuthorize(c *gin.Context) {
	if auth.ViaAPIToken(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "allow apps from Orbit itself, signed in"})
		return
	}
	var req oauthAuthorizeRequest
	if !bindJSON(c, &req) {
		return
	}
	cl, uris, ok := h.oauthClient(req.ClientID)
	if !ok || !contains(uris, req.RedirectURI) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this app or its address isn't registered with Orbit"})
		return
	}
	back, _ := url.Parse(req.RedirectURI)
	q := back.Query()
	if req.State != "" {
		q.Set("state", req.State)
	}
	q.Set("iss", h.baseURL(c))
	switch {
	case req.Deny:
		q.Set("error", "access_denied")
	case req.CodeChallengeMethod != "S256" || len(req.CodeChallenge) < 43:
		q.Set("error", "invalid_request")
		q.Set("error_description", "PKCE with S256 is required")
	default:
		code, err := h.ephemeral.Put(oauthCodeKind, oauthCode{ClientID: cl.ID, Redirect: req.RedirectURI, Challenge: req.CodeChallenge, UserID: auth.UserID(c), ReadOnly: req.ReadOnly}, 10*time.Minute)
		if err != nil {
			h.internalError(c, err, "could not allow the app")
			return
		}
		q.Set("code", code)
	}
	back.RawQuery = q.Encode()
	c.JSON(http.StatusOK, gin.H{"redirect": back.String()})
}

// oauthToken trades a code (and its PKCE verifier) for an access token: a
// new API token of the person who allowed the app.
//
//	@Summary	An OAuth app's access token
//	@Tags		mcp
//	@Accept		x-www-form-urlencoded
//	@Produce	json
//	@Success	200	{object}	map[string]any
//	@Router		/oauth/token [post]
func (h *Handlers) oauthToken(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	fail := func(code, desc string) {
		c.JSON(http.StatusBadRequest, gin.H{"error": code, "error_description": desc})
	}
	var req struct {
		GrantType    string `form:"grant_type" json:"grant_type"`
		Code         string `form:"code" json:"code"`
		RedirectURI  string `form:"redirect_uri" json:"redirect_uri"`
		ClientID     string `form:"client_id" json:"client_id"`
		CodeVerifier string `form:"code_verifier" json:"code_verifier"`
	}
	if err := c.ShouldBind(&req); err != nil {
		fail("invalid_request", "unreadable request")
		return
	}
	if req.GrantType != "authorization_code" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_grant_type"})
		return
	}
	var got oauthCode
	if ok, _ := h.ephemeral.Take(oauthCodeKind, req.Code, &got); !ok {
		fail("invalid_grant", "the code is unknown, used or expired")
		return
	}
	sum := sha256.Sum256([]byte(req.CodeVerifier))
	if got.ClientID != req.ClientID || got.Redirect != req.RedirectURI ||
		subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(got.Challenge)) != 1 {
		fail("invalid_grant", "the code wasn't given to this app, for this address, with this verifier")
		return
	}
	var u models.User
	if h.db.First(&u, "id = ?", got.UserID).Error != nil || u.Disabled {
		fail("invalid_grant", "the account can't sign in")
		return
	}
	cl, _, _ := h.oauthClient(got.ClientID)
	plaintext, hash, prefix := auth.NewAPITokenSecret()
	row := models.APIToken{UserID: u.ID, Name: clip("OAuth: "+cl.Name, 128), Prefix: prefix, TokenHash: hash, ReadOnly: got.ReadOnly}
	if err := h.db.Create(&row).Error; err != nil {
		h.internalError(c, err, "could not make the token")
		return
	}
	h.audit.Record(u.ID, "auth.api_token_create", "api_token", row.ID, c.ClientIP(), map[string]any{"oauth_client": cl.Name, "read_only": got.ReadOnly})
	scope := "orbit"
	if got.ReadOnly {
		scope = "orbit:read"
	}
	c.JSON(http.StatusOK, gin.H{"access_token": plaintext, "token_type": "Bearer", "scope": scope})
}
