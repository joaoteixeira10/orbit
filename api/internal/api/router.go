// Package api wires the HTTP routes: the /api/v1 REST surface (accounts and
// administration, then the planner: workspaces, pages, tasks, calendar), the
// API reference, the public calendar feeds and the embedded SPA.
package api

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/audit"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/caldav"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/collab"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/gitsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/helpdocs"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/linkpreview"
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
	CalSync  *calsync.Syncer      // nil: built here
	Inbox    *inbox.Service       // nil: built here
	Git      *gitsync.Engine      // nil: built here
	CalDAV   *caldav.Syncer       // nil: built here
	Links    *linkpreview.Fetcher // nil: the public-only guarded one
}

// Handlers holds the wired dependencies and implements the route handlers.
type Handlers struct {
	cfg      *config.Config
	db       *gorm.DB
	log      *zap.Logger
	auth     *auth.Manager
	audit    *audit.Recorder
	updates  *version.Checker
	guides   *helpdocs.Set // Orbit's own guides (the docs page, orbit_help)
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
	git       *gitsync.Engine
	dav       *caldav.Syncer
	links     *linkpreview.Fetcher // nil: the public-only guarded one
	// googleTokens: Google access tokens while they last (by link, or by a
	// sign-in waiting for its calendar).
	googleTokens tokenCache
	// room: who's on what now, and editing together's start locks (memory).
	room *collab.Room
}

// New builds the configured *gin.Engine.
func New(d Deps) *gin.Engine {
	gin.SetMode(d.Cfg.GinMode)
	h := &Handlers{cfg: d.Cfg, db: d.DB, log: d.Log, auth: d.Auth, audit: d.Audit, updates: d.Updates, notify: d.Notify, sealer: d.Sealer, oidc: d.OIDC, webauthn: d.WebAuthn, events: d.Events, ephemeral: ephemeral.New(d.DB), calsync: d.CalSync, inbox: d.Inbox, git: d.Git, dav: d.CalDAV, links: d.Links, room: collab.New(), guides: helpdocs.Load()}
	if h.calsync == nil {
		h.calsync = calsync.New(d.DB, d.Sealer, d.Log)
	}
	if h.inbox == nil {
		h.inbox = inbox.New(d.DB, d.Events, d.Notify, d.Sealer, d.Log, d.Cfg.PublicURL)
	}
	if h.dav == nil {
		h.dav = caldav.New(d.DB, d.Sealer, d.Log).NotifyHub(d.Events)
	}
	if h.dav.Bearer == nil {
		h.dav.Bearer = h.googleBearer // Google links sign in with a token
	}
	if h.git == nil {
		h.git = gitsync.New(d.DB, d.Sealer, d.Cfg.GitServers, d.Log)
		h.git.NotifyHub(d.Events)
	}
	migratePagesToMarkdown(d.DB, d.Log)
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
	h.refreshDrawio()
	r.Use(securityHeaders(nil, drawioOrigin))
	// Cap request bodies.
	// Uploads have their own, larger limit (UPLOAD_MAX_MB, in the handler).
	r.Use(limitBody(d.Cfg.MaxBodyBytes, func(p string) bool {
		// These keep their own, larger caps (files; drawings with their images).
		return p == "/api/v1/uploads" || p == "/api/v1/workspaces/import" ||
			strings.HasPrefix(p, "/api/v1/drawings/") || strings.HasSuffix(p, "/drawings") ||
			strings.HasPrefix(p, "/api/v1/sheets/") || strings.HasSuffix(p, "/sheets") ||
			strings.HasPrefix(p, "/api/v1/diagrams/") || strings.HasSuffix(p, "/diagrams")
	}))
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
		// Invite links: what one is for, and an account made through it.
		v1.GET("/invites/:token", authLimit, h.previewInvite)
		// "Sign in with Gitea" comes back here (the state names who started it).
		v1.GET("/git/oauth/callback", authLimit, h.gitOAuthCallback)
		v1.GET("/google/oauth/callback", authLimit, h.googleCallback)
		// A repository's webhook (signed with the list's secret).
		v1.POST("/git/hook/:link", feedLimit, h.gitHook)
		// A push notification's "Snooze" button (a signed link).
		v1.POST("/notifications/:id/snooze", feedLimit, h.snoozeByLink)
		v1.POST("/invites/:token/signup", authLimit, h.signupInvite)
		// Guest links and public forms: no account, the link's secret.
		v1.GET("/shared/:token", feedLimit, h.guestShared)
		v1.GET("/shared/:token/pages/:id", feedLimit, h.guestPageOf)
		v1.PUT("/shared/:token/pages/:id", feedLimit, h.guestEdit)
		v1.GET("/shared/:token/embed/:kind/:id", feedLimit, h.guestEmbed)
		v1.GET("/shared/:token/uploads/:id/*name", feedLimit, h.guestUpload)
		v1.GET("/shared/:token/markdown", feedLimit, h.guestMarkdown)
		v1.GET("/f/:token", feedLimit, h.publicFormOf)
		v1.POST("/f/:token", authLimit, h.answerPublicForm)
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
			authed.POST("/auth/step-up", authLimit, h.stepUp)
			authed.POST("/auth/step-up/webauthn/begin", authLimit, h.stepUpWebAuthnBegin)
			authed.POST("/auth/step-up/webauthn/finish", authLimit, h.stepUpWebAuthnFinish)

			// Active sessions (self-service; gated by the "sessions" feature flag).
			authed.GET("/me/sessions", h.listSessions)
			authed.POST("/me/sessions/revoke-others", h.revokeOtherSessions)
			authed.DELETE("/me/sessions/:sid", h.revokeSession)
			authed.GET("/about", h.about)
			authed.PUT("/me/profile", h.updateProfile)
			authed.PUT("/me/password", authLimit, notImpersonating(), h.changePassword)
			authed.POST("/me/rotate-token", notImpersonating(), h.rotateToken)
			// Personal API tokens (self-service; inherit the caller's RBAC).
			authed.POST("/me/api-tokens", notImpersonating(), h.createAPIToken)
			authed.GET("/me/api-tokens", h.listAPITokens)
			authed.DELETE("/me/api-tokens/:id", h.deleteAPIToken)
			authed.POST("/me/otp/setup", notImpersonating(), h.setupOTP)
			authed.POST("/me/otp/verify", authLimit, notImpersonating(), h.verifyOTP)
			authed.POST("/me/otp/disable", authLimit, notImpersonating(), h.disableOTP)
			authed.POST("/me/otp/backup-codes", authLimit, notImpersonating(), h.regenerateBackupCodes)
			// Passkeys (self-service).
			authed.POST("/me/webauthn/register/begin", notImpersonating(), h.webauthnRegisterBegin)
			authed.POST("/me/webauthn/register/finish", notImpersonating(), h.webauthnRegisterFinish)
			authed.GET("/me/webauthn", h.listWebAuthnCredentials)
			authed.PATCH("/me/webauthn/:id", notImpersonating(), h.renameWebAuthnCredential)
			authed.DELETE("/me/webauthn/:id", notImpersonating(), h.deleteWebAuthnCredential)
			authed.GET("/me/notifications", h.listNotifications)
			authed.POST("/me/notifications/read", h.readNotifications)
			authed.POST("/me/notifications/:id/snooze", h.snoozeNotification)
			authed.GET("/me/push", h.pushInfo)
			authed.POST("/me/push", notImpersonating(), h.subscribePush)
			authed.POST("/me/push/remove", h.unsubscribePush)
			authed.POST("/me/push/test", h.testPush)
			authed.GET("/me/prefs", h.getPrefs)
			authed.PUT("/me/prefs", h.updatePrefs)
			authed.GET("/me/channels", h.listMyChannels)
			authed.POST("/me/channels", authLimit, notImpersonating(), h.addMyChannel)
			authed.POST("/me/channels/:id/test", authLimit, h.testMyChannel)
			authed.DELETE("/me/channels/:id", notImpersonating(), h.deleteMyChannel)
			// Notification channels the admin has enabled (for the user's picker).
			authed.GET("/notifications/channels", h.notifChannels)
			authed.PUT("/notifications/personal-kinds", h.auth.RequireOwner(), h.requireStepUp(), h.setPersonalKinds)

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
			authed.GET("/docs", h.listGuides)
			authed.GET("/docs/:slug", h.getGuide)
			authed.POST("/users/:id/impersonate/end", h.auth.RequireOwnerOrGlobal(rbac.PUsersImpersonate), h.endImpersonation)
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
			authed.GET("/users/lookup", authLimit, h.lookupUser) // tells whether a name exists: throttled
			authed.GET("/me/favorites", h.listFavorites)
			authed.PUT("/me/workspace-order", h.orderWorkspaces)
			authed.GET("/me/feeds", h.listFeeds)
			authed.POST("/me/feeds", notImpersonating(), h.createFeed)
			authed.DELETE("/me/feeds/:id", h.deleteFeed)

			authed.GET("/workspaces", h.listWorkspaces)
			authed.POST("/workspaces", h.auth.RequireGlobal(rbac.PWorkspacesCreate), h.createWorkspace)
			authed.POST("/workspaces/import", h.auth.RequireGlobal(rbac.PWorkspacesCreate), h.importWorkspace)
			authed.GET("/workspaces/:ws/transfer", h.exportWorkspace)
			authed.GET("/workspaces/:ws", h.getWorkspace)
			authed.PUT("/workspaces/:ws", h.updateWorkspace)
			authed.DELETE("/workspaces/:ws", h.deleteWorkspace)
			authed.GET("/workspaces/:ws/members", h.listMembers)
			authed.POST("/workspaces/:ws/members", h.addMember)
			authed.PUT("/workspaces/:ws/members/:uid", h.setMemberRole)
			authed.DELETE("/workspaces/:ws/members/:uid", h.removeMember)
			authed.GET("/workspaces/:ws/activity", h.workspaceActivity)
			authed.GET("/workspaces/:ws/caldav", h.getCalDAV)
			authed.GET("/workspaces/:ws/drawings", h.listDrawings)
			authed.GET("/workspaces/:ws/habits", h.listHabits)
			authed.GET("/workspaces/:ws/goals", h.listGoals)
			authed.GET("/workspaces/:ws/journal", h.listJournal)
			authed.GET("/workspaces/:ws/bookmarks", h.listBookmarks)
			authed.POST("/workspaces/:ws/bookmarks", h.createBookmark)
			authed.PUT("/bookmarks/:id", h.updateBookmark)
			authed.DELETE("/bookmarks/:id", h.deleteBookmark)
			authed.POST("/bookmarks/:id/refresh", h.refreshBookmark)
			authed.GET("/bookmarks/:id/image", h.bookmarkImage)
			authed.GET("/workspaces/:ws/journal/:date", h.getJournalDay)
			authed.PUT("/workspaces/:ws/journal/:date", h.putJournalDay)
			authed.POST("/workspaces/:ws/goals", h.createGoal)
			authed.PUT("/goals/:id", h.updateGoal)
			authed.DELETE("/goals/:id", h.deleteGoal)
			authed.GET("/workspaces/:ws/sheets", h.listSheets)
			authed.POST("/workspaces/:ws/sheets", h.createSheet)
			authed.POST("/workspaces/:ws/google-sheet", h.googleSheet)
			authed.GET("/sheets/:id", h.getSheet)
			authed.PUT("/sheets/:id", h.updateSheet)
			authed.DELETE("/sheets/:id", h.deleteSheet)
			authed.GET("/google/status", h.googleStatus)
			authed.GET("/google/app", h.auth.RequireOwner(), h.getGoogleApp)
			authed.PUT("/google/app", h.auth.RequireOwner(), h.requireStepUp(), h.setGoogleApp)
			authed.POST("/workspaces/:ws/google/start", notImpersonating(), h.startGoogle)
			authed.GET("/workspaces/:ws/google/calendars", h.googleCalendars)
			authed.POST("/workspaces/:ws/google/link", h.linkGoogle)
			authed.POST("/drawings/:id/move", h.moveDrawing)
			authed.GET("/me/blocks", h.listBlocks)
			authed.PUT("/me/blocks", h.planBlocks)
			authed.DELETE("/me/blocks/:id", h.deleteBlock)
			authed.GET("/workspaces/:ws/folders", h.listFolders)
			authed.GET("/workspaces/:ws/tags", h.listTags)
			authed.GET("/workspaces/:ws/graph", h.workspaceGraph)
			authed.GET("/oauth/clients/:id", h.oauthClientInfo)
			authed.PUT("/pages/:id/access", h.setPageAccess)
			authed.GET("/me/page-changes", h.listPageChanges)
			authed.POST("/page-changes/:id/undo", h.undoPageChange)
			authed.GET("/pages/:id/links", h.listShareLinks)
			authed.POST("/pages/:id/links", h.createShareLink)
			authed.PUT("/links/:id", h.updateShareLink)
			authed.DELETE("/links/:id", h.deleteShareLink)
			authed.GET("/pages/:id/forms", h.listForms)
			authed.POST("/pages/:id/forms", h.createForm)
			authed.GET("/forms/:id", h.getFormToFill)
			authed.PUT("/forms/:id", h.updateForm)
			authed.DELETE("/forms/:id", h.deleteForm)
			authed.POST("/forms/:id/answers", h.answerForm)
			authed.POST("/oauth/authorize", notImpersonating(), h.oauthAuthorize)
			authed.GET("/pages/:id/versions", h.listPageVersions)
			authed.GET("/page-versions/:id", h.getPageVersion)
			authed.POST("/page-versions/:id/restore", h.restorePageVersion)
			authed.POST("/workspaces/:ws/folders", h.createFolder)
			authed.PUT("/folders/:id", h.updateFolder)
			authed.DELETE("/folders/:id", h.deleteFolder)
			authed.POST("/workspaces/:ws/files/move", h.moveFiles)
			authed.GET("/workspaces/:ws/files", h.listFiles)
			authed.GET("/workspaces/:ws/files/usage", h.filesUsage)
			authed.GET("/files/:id", h.getFile)
			authed.PUT("/files/:id", h.updateFile)
			authed.PUT("/files/:id/text", h.setFileText)
			authed.GET("/files/:id/versions", h.fileVersions)
			authed.POST("/files/:id/current", h.fileCurrent)
			authed.POST("/files/:id/restore", h.fileUntrash)
			authed.POST("/presence/:kind/:id", h.presence)
			authed.GET("/collab/:kind/:id", h.collabSince)
			authed.POST("/collab/:kind/:id", h.collabAdd)
			authed.POST("/collab/:kind/:id/seed", h.collabSeed)
			authed.POST("/collab/:kind/:id/awareness", h.collabAwareness)
			authed.POST("/collab/:kind/:id/compact", h.collabCompactLog)
			authed.GET("/me/folders", h.listMyFolders)
			authed.POST("/me/folders", h.createMyFolder)
			authed.POST("/me/files/move", h.moveMyFiles)
			authed.GET("/me/files", h.listMyFiles)
			authed.GET("/me/files/usage", h.myFilesUsage)
			authed.GET("/workspaces/:ws/diagrams", h.listDiagrams)
			authed.POST("/workspaces/:ws/diagrams", h.createDiagram)
			authed.GET("/diagrams/:id", h.getDiagram)
			authed.PUT("/diagrams/:id", h.updateDiagram)
			authed.DELETE("/diagrams/:id", h.deleteDiagram)
			authed.GET("/workspaces/:ws/smart-lists", h.listSmartLists)
			authed.POST("/workspaces/:ws/smart-lists", h.createSmartList)
			authed.PUT("/smart-lists/:id", h.updateSmartList)
			authed.DELETE("/smart-lists/:id", h.deleteSmartList)
			authed.GET("/me/focus", h.getFocus)
			authed.POST("/me/focus", h.startFocus)
			authed.POST("/me/focus/stop", h.stopFocus)
			authed.POST("/workspaces/:ws/habits", h.createHabit)
			authed.PUT("/habits/:id", h.updateHabit)
			authed.DELETE("/habits/:id", h.deleteHabit)
			authed.PUT("/habits/:id/checks/:date", h.checkHabit)
			authed.POST("/workspaces/:ws/drawings", h.createDrawing)
			authed.GET("/drawings/:id", h.getDrawing)
			authed.GET("/me/drawings", h.listMyDrawings)
			authed.POST("/me/drawings", h.createMyDrawing)
			authed.PUT("/drawings/:id", h.updateDrawing)
			authed.DELETE("/drawings/:id", h.deleteDrawing)
			authed.POST("/notes/:id/drawing", h.noteDrawingOf)
			authed.PUT("/workspaces/:ws/caldav", h.setCalDAV)
			authed.DELETE("/workspaces/:ws/caldav", h.deleteCalDAV)
			authed.POST("/workspaces/:ws/caldav/calendars", h.caldavCalendars)
			authed.POST("/workspaces/:ws/caldav/sync", h.syncCalDAV)
			authed.GET("/holidays/countries", h.holidayCountries)
			authed.GET("/workspaces/:ws/invites", h.listInvites)
			authed.POST("/workspaces/:ws/invites", h.createInvite)
			authed.DELETE("/workspaces/:ws/invites/:id", h.revokeInvite)
			authed.POST("/invites/:token/accept", authLimit, notImpersonating(), h.acceptInvite)
			// Comments on tasks.
			authed.GET("/tasks/:id/comments", h.listComments)
			authed.GET("/tasks/:id/activity", h.taskActivity)
			authed.POST("/tasks/:id/comments", h.createComment)
			authed.PUT("/comments/:id", h.updateComment)
			authed.DELETE("/comments/:id", h.deleteComment)
			// School: courses (shared) and grades (each person's own).
			authed.GET("/workspaces/:ws/courses", h.listCourses)
			authed.POST("/workspaces/:ws/courses", h.createCourse)
			authed.PUT("/courses/:id", h.updateCourse)
			authed.DELETE("/courses/:id", h.deleteCourse)
			authed.PUT("/courses/:id/goal", h.setGoal)
			authed.POST("/courses/:id/page", h.coursePage)
			authed.POST("/courses/:id/grades", h.createGrade)
			authed.GET("/workspaces/:ws/grades", h.listGrades)
			authed.PUT("/grades/:id", h.updateGrade)
			authed.DELETE("/grades/:id", h.deleteGrade)
			authed.PUT("/workspaces/:ws/mute", h.muteWorkspace)

			authed.GET("/workspaces/:ws/pages", h.listPages)
			authed.POST("/workspaces/:ws/pages", h.createPage)
			authed.GET("/workspaces/:ws/trash", h.listTrash)
			authed.GET("/workspaces/:ws/export", h.exportPages)
			authed.GET("/pages/:id", h.getPage)
			authed.PUT("/pages/:id", h.updatePage)
			authed.DELETE("/pages/:id", h.trashPage)
			authed.POST("/pages/:id/move", h.movePage)
			authed.POST("/pages/:id/restore", h.restorePage)
			authed.POST("/pages/:id/duplicate", h.duplicatePage)
			authed.GET("/pages/:id/backlinks", h.pageBacklinks)
			authed.POST("/pages/:id/favorite", h.favoritePage)
			authed.DELETE("/pages/:id/favorite", h.favoritePage)

			authed.GET("/workspaces/:ws/notes", h.listNotes)
			authed.POST("/workspaces/:ws/notes", h.createNote)
			authed.PUT("/workspaces/:ws/notes/order", h.orderNotes)
			authed.PUT("/notes/:id", h.updateNote)
			authed.POST("/notes/:id/move", h.moveNote)
			authed.GET("/me/notes", h.listMyNotes)
			authed.POST("/me/notes", h.createMyNote)
			authed.PUT("/me/notes/order", h.orderMyNotes)
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

			authed.POST("/uploads", h.uploadFile)
			authed.GET("/uploads", h.listUploads)
			authed.GET("/uploads/:id/*name", h.serveUpload)
			authed.DELETE("/uploads/:id", h.deleteUpload)

			authed.GET("/me/git", h.listGitAccounts)
			authed.POST("/me/git", notImpersonating(), h.connectGit)
			authed.DELETE("/me/git/:id", notImpersonating(), h.disconnectGit)
			authed.GET("/me/git/:id/repos", h.gitRepos)
			authed.GET("/me/git/:id/boards", h.gitBoards)
			authed.POST("/workspaces/:ws/lists/git", h.createGitList)
			authed.DELETE("/lists/:id/git", h.unlinkGitList)
			authed.POST("/lists/:id/git/sync", h.syncGitList)
			authed.POST("/lists/:id/git/hook", h.gitHookOn)
			authed.DELETE("/lists/:id/git/hook", h.gitHookOff)
			authed.GET("/lists/:id/git/milestones", h.gitMilestones)
			authed.PUT("/lists/:id/git/bot", notImpersonating(), h.setGitBot)
			authed.POST("/me/git/oauth/start", notImpersonating(), h.startGitOAuth)
			authed.GET("/git/oauth-apps", h.auth.RequireOwner(), h.getGitOAuthApps)
			authed.PUT("/git/oauth-apps", h.auth.RequireOwner(), h.requireStepUp(), h.setGitOAuthApps)
			authed.POST("/tasks/:id/git", h.linkGitTask)
			authed.DELETE("/tasks/:id/git", h.unlinkGitTask)

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

	// MCP (Model Context Protocol): assistants use Orbit as the person who
	// signed them in (mcp.go), with Orbit's OAuth to sign in (oauth.go).
	open := openCORS()
	mcp := r.Group("/mcp", open)
	mcp.OPTIONS("", func(*gin.Context) {})
	mcp.POST("", h.mcpChallenge(), h.auth.Required(), h.mcpHandler(r))
	mcp.GET("", mcpNoStream)
	mcp.DELETE("", mcpNoStream)
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		r.GET(p, open, h.oauthResource)
		r.OPTIONS(p, open)
	}
	r.GET("/.well-known/oauth-authorization-server", open, h.oauthServer)
	r.OPTIONS("/.well-known/oauth-authorization-server", open)
	r.POST("/api/v1/oauth/register", open, authLimit, h.oauthRegister)
	r.OPTIONS("/api/v1/oauth/register", open)
	r.POST("/api/v1/oauth/token", open, authLimit, h.oauthToken)
	r.OPTIONS("/api/v1/oauth/token", open)

	registerSPA(r, h)
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

// registerSPA serves the embedded Vue app with HTML5 history-mode fallback;
// its page carries the link-preview tags (preview.go).
func registerSPA(r *gin.Engine, h *Handlers) {
	// Go's built-in table has no .webmanifest; it only comes from the host's
	// /etc/mime.types, so without this it's sniffed as text/plain on some hosts.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	_ = mime.AddExtensionType(".woff2", "font/woff2") // drawings' fonts
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
		if index, err := fs.ReadFile(dist, "index.html"); err == nil && bytes.Contains(index, []byte("</head>")) {
			page := bytes.Replace(index, []byte("</head>"), []byte("  "+h.previewTags(c)+"\n  </head>"), 1)
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", page)
			return
		}
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
