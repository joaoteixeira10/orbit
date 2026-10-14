// Package api wires the HTTP routes: the /api/v1 REST surface (accounts and
// administration, then the planner: workspaces, pages, tasks, calendar), the
// API reference, the public calendar feeds and the embedded SPA.
package api

import (
	"io/fs"
	"mime"
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/audit"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/version"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/webui"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Deps are the shared dependencies handed to the router.
type Deps struct {
	Cfg      *config.Config
	DB       *gorm.DB
	Log      *zap.Logger
	Auth     *auth.Manager
	Audit    *audit.Recorder
	Updates  *version.Checker
	Notify   *notify.Service
	Sealer   *keystore.Sealer
	OIDC     *auth.OIDC
	WebAuthn *auth.WebAuthn
	Events   *events.Hub
	CalSync  *calsync.Syncer // nil: built here
	Inbox    *inbox.Service  // nil: built here
}

// Handlers holds the wired dependencies and implements the route handlers.
type Handlers struct {
	cfg      *config.Config
	db       *gorm.DB
	log      *zap.Logger
	auth     *auth.Manager
	audit    *audit.Recorder
	updates  *version.Checker
	notify   *notify.Service
	sealer   *keystore.Sealer
	oidc     *auth.OIDC
	webauthn *auth.WebAuthn
	events   *events.Hub
	// ephemeral holds short-lived single-use pre-auth state (SSO handoff codes)
	// in the shared DB so any replica can redeem it.
	ephemeral *ephemeral.Store
	calsync   *calsync.Syncer
	inbox     *inbox.Service
}

// New builds the configured *gin.Engine.
func New(d Deps) *gin.Engine {
	gin.SetMode(d.Cfg.GinMode)
	h := &Handlers{cfg: d.Cfg, db: d.DB, log: d.Log, auth: d.Auth, audit: d.Audit, updates: d.Updates, notify: d.Notify, sealer: d.Sealer, oidc: d.OIDC, webauthn: d.WebAuthn, events: d.Events, ephemeral: ephemeral.New(d.DB), calsync: d.CalSync, inbox: d.Inbox}
	if h.calsync == nil {
		h.calsync = calsync.New(d.DB, d.Sealer, d.Log)
	}
	if h.inbox == nil {
		h.inbox = inbox.New(d.DB, d.Events, d.Notify, d.Sealer, d.Log, d.Cfg.PublicURL)
	}
	exportDomainMetrics(d.DB)
	// Calendar apps poll feeds; they get their own, looser per-IP limit.
	feedLimit := newIPRateLimiter(60, 1).middleware()

	r := gin.New()
	// Trust only the configured reverse-proxy CIDRs for X-Forwarded-For (empty =
	// trust none → ClientIP() is the socket peer, so a spoofed XFF can't forge
	// the client IP that the rate limiter and audit log key on).
	if err := r.SetTrustedProxies(d.Cfg.TrustedProxies); err != nil {
		d.Log.Error("invalid TRUSTED_PROXIES; trusting no proxy", zap.Error(err))
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(gin.Recovery())
	r.Use(requestID())
	r.Use(requestLogger(d.Log))
	r.Use(securityHeaders(nil))
	// Cap request bodies.
	r.Use(limitBody(d.Cfg.MaxBodyBytes, nil))
	r.Use(metricsMiddleware())

	// Defence-in-depth brute-force throttle for credential/second-factor
	// endpoints (per-IP, configurable via AUTH_RATE_*). The upstream proxy may
	// add coarser limits.
	authLimit := newIPRateLimiter(float64(d.Cfg.AuthRateBurst), float64(d.Cfg.AuthRatePerMinute)/60.0).middleware()

	// Optional Prometheus endpoint (kept behind the proxy; toggle with METRICS_ENABLED).
	if d.Cfg.MetricsEnabled {
		r.GET("/metrics", metricsHandler())
	}

	if len(d.Cfg.CORSOrigins) > 0 {
		cc := cors.DefaultConfig()
		cc.AllowOrigins = d.Cfg.CORSOrigins
		cc.AllowCredentials = true
		cc.AddAllowHeaders("Authorization", "X-Orbit-Client", "X-CSRF-Token")
		r.Use(cors.New(cc))
	}

	v1 := r.Group("/api/v1")
	{
		v1.GET("/healthz", h.health)
		v1.GET("/readyz", h.ready)
		v1.GET("/version", h.version)
		v1.GET("/branding", h.getBranding) // public: SPA theming at boot

		// --- Auth (public) ---
		v1.POST("/auth/login", authLimit, h.login)
		v1.POST("/auth/refresh", h.refresh)
		v1.POST("/auth/logout", h.logout)
		v1.GET("/auth/oidc/login", h.oidcLogin)
		v1.GET("/auth/oidc/callback", h.oidcCallback)
		v1.POST("/auth/oidc/exchange", authLimit, h.oidcExchange)
		// Passwordless / second-factor passkey assertion (pre-auth).
		v1.POST("/auth/webauthn/login/begin", authLimit, h.webauthnLoginBegin)
		v1.POST("/auth/webauthn/login/finish", authLimit, h.webauthnLoginFinish)
		// SSE stream: self-authenticates by redeeming a one-time ticket (the
		// browser EventSource API can't send the Authorization header).
		v1.GET("/auth/events", h.eventStream)

		// --- Authenticated ---
		// Server-scoped privileges are enforced inside the handlers (they need
		// the server); global privileges are gated by RequireGlobal middleware.
		authed := v1.Group("")
		authed.Use(h.auth.Required())
		{
			// Self-service.
			authed.GET("/auth/me", h.me)
			authed.POST("/auth/events/ticket", h.eventsTicket)

			// Feature flags (read: any authed user gates UI; write: owner only)
			// + instance-owner ("god mode") actions.
			authed.GET("/features", h.listFeatures)
			authed.PUT("/features", h.auth.RequireOwner(), h.requireStepUp(), h.updateFeatures)
			authed.POST("/owner/transfer", h.auth.RequireOwner(), h.requireStepUp(), h.transferOwner)
			authed.GET("/security-alerts", h.auth.RequireOwner(), h.getSecurityAlerts)
			authed.PUT("/security-alerts", h.auth.RequireOwner(), h.updateSecurityAlerts)
			// Encrypted backups (owner: the archive holds every user's secrets, sealed).
			authed.POST("/backup/export", h.auth.RequireOwner(), h.requireStepUp(), h.exportBackup)
			authed.GET("/backup/schedule", h.auth.RequireOwner(), h.getBackupSchedule)
			authed.PUT("/backup/schedule", h.auth.RequireOwner(), h.requireStepUp(), h.updateBackupSchedule)
			authed.POST("/backup/run", h.auth.RequireOwner(), h.runBackupNow)

			// Step-up re-authentication (password or passkey) for dangerous actions.
			authed.POST("/auth/step-up", h.stepUp)
			authed.POST("/auth/step-up/webauthn/begin", h.stepUpWebAuthnBegin)
			authed.POST("/auth/step-up/webauthn/finish", h.stepUpWebAuthnFinish)

			// Active sessions (self-service; gated by the "sessions" feature flag).
			authed.GET("/me/sessions", h.listSessions)
			authed.POST("/me/sessions/revoke-others", h.revokeOtherSessions)
			authed.DELETE("/me/sessions/:sid", h.revokeSession)
			authed.GET("/about", h.about)
			authed.PUT("/me/profile", h.updateProfile)
			authed.PUT("/me/password", h.changePassword)
			authed.POST("/me/rotate-token", h.rotateToken)
			// Personal API tokens (self-service; inherit the caller's RBAC).
			authed.POST("/me/api-tokens", h.createAPIToken)
			authed.GET("/me/api-tokens", h.listAPITokens)
			authed.DELETE("/me/api-tokens/:id", h.deleteAPIToken)
			authed.POST("/me/otp/setup", h.setupOTP)
			authed.POST("/me/otp/verify", authLimit, h.verifyOTP)
			authed.POST("/me/otp/disable", authLimit, h.disableOTP)
			authed.POST("/me/otp/backup-codes", authLimit, h.regenerateBackupCodes)
			// Passkeys (self-service).
			authed.POST("/me/webauthn/register/begin", h.webauthnRegisterBegin)
			authed.POST("/me/webauthn/register/finish", h.webauthnRegisterFinish)
			authed.GET("/me/webauthn", h.listWebAuthnCredentials)
			authed.PATCH("/me/webauthn/:id", h.renameWebAuthnCredential)
			authed.DELETE("/me/webauthn/:id", h.deleteWebAuthnCredential)
			authed.GET("/me/notifications", h.listNotifications)
			authed.POST("/me/notifications/read", h.readNotifications)
			authed.GET("/me/push", h.pushInfo)
			authed.POST("/me/push", h.subscribePush)
			authed.POST("/me/push/remove", h.unsubscribePush)
			authed.POST("/me/push/test", h.testPush)
			authed.GET("/me/prefs", h.getPrefs)
			authed.PUT("/me/prefs", h.updatePrefs)
			// Notification channels the admin has enabled (for the user's picker).
			authed.GET("/notifications/channels", h.notifChannels)

			// Global-privilege routes.
			authed.GET("/users", h.auth.RequireGlobal(rbac.PUsersManage), h.listUsers)
			authed.POST("/users", h.auth.RequireGlobal(rbac.PUsersManage), h.createUser)
			authed.GET("/users/:id", h.auth.RequireGlobal(rbac.PUsersManage), h.getUser)
			authed.PUT("/users/:id", h.auth.RequireGlobal(rbac.PUsersManage), h.updateUser)
			authed.PUT("/users/:id/password", h.auth.RequireGlobal(rbac.PUsersManage), h.resetUserPassword)
			authed.POST("/users/:id/reset-otp", h.auth.RequireGlobal(rbac.PUsersManage), h.resetUserOTP)
			authed.POST("/users/:id/reset-webauthn", h.auth.RequireGlobal(rbac.PUsersManage), h.resetUserWebAuthn)
			authed.POST("/users/:id/logout", h.auth.RequireGlobal(rbac.PUsersManage), h.forceLogout)
			authed.POST("/users/:id/unlock", h.auth.RequireGlobal(rbac.PUsersManage), h.unlockUser)
			authed.POST("/users/:id/impersonate", h.auth.RequireOwnerOrGlobal(rbac.PUsersImpersonate), h.requireStepUp(), h.impersonateUser)
			authed.GET("/api-tokens", h.auth.RequireGlobal(rbac.PUsersManage), h.listAllAPITokens)
			authed.DELETE("/api-tokens/:id", h.auth.RequireGlobal(rbac.PUsersManage), h.revokeAnyAPIToken)
			authed.DELETE("/users/:id", h.auth.RequireGlobal(rbac.PUsersManage), h.deleteUser)

			authed.GET("/roles", h.auth.RequireGlobal(rbac.PRolesManage), h.listRoles)
			authed.GET("/privileges", h.auth.RequireGlobal(rbac.PRolesManage), h.listPrivileges)
			authed.POST("/roles", h.auth.RequireGlobal(rbac.PRolesManage), h.createRole)
			authed.GET("/roles/:id", h.auth.RequireGlobal(rbac.PRolesManage), h.getRole)
			authed.PUT("/roles/:id", h.auth.RequireGlobal(rbac.PRolesManage), h.updateRole)
			authed.DELETE("/roles/:id", h.auth.RequireGlobal(rbac.PRolesManage), h.deleteRole)

			// Planner. Access to a workspace comes from its membership and is
			// checked in the handlers (wsAccess); only creating one needs an
			// instance privilege.
			authed.GET("/catalog", h.planCatalog)
			authed.GET("/home", h.home)
			authed.GET("/search", h.search)
			authed.GET("/calendar", h.myCalendar)
			authed.GET("/users/lookup", h.lookupUser)
			authed.GET("/me/favorites", h.listFavorites)
			authed.PUT("/me/workspace-order", h.orderWorkspaces)
			authed.GET("/me/feeds", h.listFeeds)
			authed.POST("/me/feeds", h.createFeed)
			authed.DELETE("/me/feeds/:id", h.deleteFeed)

			authed.GET("/workspaces", h.listWorkspaces)
			authed.POST("/workspaces", h.auth.RequireGlobal(rbac.PWorkspacesCreate), h.createWorkspace)
			authed.GET("/workspaces/:ws", h.getWorkspace)
			authed.PUT("/workspaces/:ws", h.updateWorkspace)
			authed.DELETE("/workspaces/:ws", h.deleteWorkspace)
			authed.GET("/workspaces/:ws/members", h.listMembers)
			authed.POST("/workspaces/:ws/members", h.addMember)
			authed.PUT("/workspaces/:ws/members/:uid", h.setMemberRole)
			authed.DELETE("/workspaces/:ws/members/:uid", h.removeMember)
			authed.PUT("/workspaces/:ws/mute", h.muteWorkspace)

			authed.GET("/workspaces/:ws/pages", h.listPages)
			authed.POST("/workspaces/:ws/pages", h.createPage)
			authed.GET("/workspaces/:ws/trash", h.listTrash)
			authed.GET("/pages/:id", h.getPage)
			authed.PUT("/pages/:id", h.updatePage)
			authed.DELETE("/pages/:id", h.trashPage)
			authed.POST("/pages/:id/move", h.movePage)
			authed.POST("/pages/:id/restore", h.restorePage)
			authed.POST("/pages/:id/duplicate", h.duplicatePage)
			authed.POST("/pages/:id/favorite", h.favoritePage)
			authed.DELETE("/pages/:id/favorite", h.favoritePage)

			authed.GET("/workspaces/:ws/notes", h.listNotes)
			authed.POST("/workspaces/:ws/notes", h.createNote)
			authed.PUT("/workspaces/:ws/notes/order", h.orderNotes)
			authed.PUT("/notes/:id", h.updateNote)
			authed.DELETE("/notes/:id", h.deleteNote)

			authed.GET("/workspaces/:ws/lists", h.listLists)
			authed.POST("/workspaces/:ws/lists", h.createList)
			authed.PUT("/workspaces/:ws/lists/order", h.orderLists)
			authed.PUT("/lists/:id", h.updateList)
			authed.DELETE("/lists/:id", h.deleteList)
			authed.GET("/workspaces/:ws/tasks", h.listTasks)
			authed.POST("/workspaces/:ws/tasks", h.createTask)
			authed.PUT("/tasks/:id", h.updateTask)
			authed.POST("/tasks/:id/move", h.moveTask)
			authed.DELETE("/tasks/:id", h.deleteTask)

			authed.GET("/workspaces/:ws/calendar", h.workspaceCalendar)
			authed.POST("/workspaces/:ws/events", h.createEvent)
			authed.GET("/events/:id", h.getEvent)
			authed.PUT("/events/:id", h.updateEvent)
			authed.DELETE("/events/:id", h.deleteEvent)
			authed.GET("/workspaces/:ws/subscriptions", h.listSubscriptions)
			authed.POST("/workspaces/:ws/subscriptions", h.createSubscription)
			authed.PUT("/subscriptions/:id", h.updateSubscription)
			authed.POST("/subscriptions/:id/refresh", h.refreshSubscription)
			authed.DELETE("/subscriptions/:id", h.deleteSubscription)

			authed.GET("/audit", h.auth.RequireGlobal(rbac.PAuditRead), h.listAudit)
			authed.GET("/audit/verify", h.auth.RequireGlobal(rbac.PAuditRead), h.verifyAudit)
			authed.GET("/audit/export", h.auth.RequireGlobal(rbac.PAuditRead), h.exportAudit)
			authed.PUT("/settings", h.auth.RequireOwnerOrGlobal(rbac.PSettingsBranding), h.updateSettings)

			// Notification channel configuration (owner, or an owner-granted role).
			authed.GET("/notifications/config", h.auth.RequireOwnerOrGlobal(rbac.PSettingsNotify), h.getNotifConfig)
			authed.PUT("/notifications/config", h.auth.RequireOwnerOrGlobal(rbac.PSettingsNotify), h.updateNotifConfig)
			authed.POST("/notifications/test", h.auth.RequireOwnerOrGlobal(rbac.PSettingsNotify), h.testNotif)

			// Security policy — owner only (weakens instance-wide auth).
			authed.GET("/security/config", h.auth.RequireOwner(), h.getSecurityConfig)
			authed.PUT("/security/config", h.auth.RequireOwner(), h.requireStepUp(), h.updateSecurityConfig)

			// SSO / OIDC configuration — owner only (controls who can sign in).
			authed.GET("/auth/oidc/config", h.auth.RequireOwner(), h.getOIDCConfig)
			authed.PUT("/auth/oidc/config", h.auth.RequireOwner(), h.requireStepUp(), h.updateOIDCConfig)
			authed.POST("/auth/oidc/test", h.auth.RequireOwner(), h.testOIDCDiscovery)
		}
	}

	// API reference (Scalar) and its OpenAPI document. The old Swagger UI
	// address sends you to the reference.
	r.GET("/api/docs", h.apiDocs)
	r.GET("/api/v1/openapi.json", h.openAPI)
	r.GET("/api/v1/swagger/*any", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/api/docs") })

	// Private calendar feeds for other calendar apps (the token is the path).
	r.GET("/ical/:file", feedLimit, h.serveFeed)

	registerSPA(r)
	return r
}

// health is a liveness probe (process is up).
//
//	@Summary	Liveness probe
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	map[string]string
//	@Router		/healthz [get]
func (h *Handlers) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ready is a readiness probe: reports 503 until the database is reachable.
//
//	@Summary	Readiness probe
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	map[string]string
//	@Failure	503	{object}	map[string]string
//	@Router		/readyz [get]
func (h *Handlers) ready(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err == nil {
		err = sqlDB.Ping()
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

// version returns build/identity info.
//
//	@Summary	API version
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	map[string]string
//	@Router		/version [get]
func (h *Handlers) version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":    "orbit",
		"api":     "v1",
		"version": version.Version,
		"commit":  version.Commit,
	})
}

// registerSPA serves the embedded Vue app with HTML5 history-mode fallback.
func registerSPA(r *gin.Engine) {
	// Go's built-in table has no .webmanifest; it only comes from the host's
	// /etc/mime.types, so without this it's sniffed as text/plain on some hosts.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	dist := webui.FS()
	fileServer := http.FileServer(http.FS(dist))

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if p != "/" {
			if _, err := fs.Stat(dist, strings.TrimPrefix(p, "/")); err == nil {
				// The service worker is checked for updates on every load.
				if p == "/sw.js" {
					c.Header("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
