package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Invite links: a workspace's owners make a link (a role, an expiry, a
// number of uses); whoever opens it joins — signed in, with one click, or
// creating an account first. The link's token is shown once and kept only
// as a SHA-256. Owner flag workspace_invites (on by default).

func (h *Handlers) invitesEnabled(c *gin.Context) bool {
	if !feature.Enabled(h.db, feature.WorkspaceInvites) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite links are turned off on this instance"})
		return false
	}
	return true
}

func hashInvite(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type inviteRequest struct {
	Role    string `json:"role" binding:"required,oneof=editor viewer"`
	Days    int    `json:"days" binding:"omitempty,min=1,max=30"`      // default 7
	MaxUses int    `json:"max_uses" binding:"omitempty,min=1,max=100"` // default 1
}

type inviteCreated struct {
	Invite models.WorkspaceInvite `json:"invite"`
	Token  string                 `json:"token"` // shown once
	Path   string                 `json:"path"`  // /join/<token>
}

// createInvite makes an invite link (the workspace's owners).
//
//	@Summary	Create an invite link
//	@Tags		workspaces
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		inviteRequest	true	"Role, days, uses"
//	@Success	201		{object}	inviteCreated
//	@Router		/workspaces/{ws}/invites [post]
func (h *Handlers) createInvite(c *gin.Context) {
	if !h.invitesEnabled(c) {
		return
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var req inviteRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Days == 0 {
		req.Days = 7
	}
	if req.MaxUses == 0 {
		req.MaxUses = 1
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		h.internalError(c, err, "could not make the link")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	inv := models.WorkspaceInvite{WorkspaceID: ws.ID, TokenHash: hashInvite(token), Prefix: token[:6], Role: req.Role,
		CreatedBy: auth.UserID(c), ExpiresAt: time.Now().AddDate(0, 0, req.Days), MaxUses: req.MaxUses}
	if err := h.db.Create(&inv).Error; err != nil {
		h.internalError(c, err, "could not make the link")
		return
	}
	h.recordCtx(c, "workspace.invite_create", "workspace", ws.ID, gin.H{"role": inv.Role, "days": req.Days, "max_uses": inv.MaxUses, "prefix": inv.Prefix})
	c.JSON(http.StatusCreated, inviteCreated{Invite: inv, Token: token, Path: "/join/" + token})
}

// listInvites lists a workspace's invite links that can still be used
// (owners).
//
//	@Summary	Invite links of a workspace
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	models.WorkspaceInvite
//	@Router		/workspaces/{ws}/invites [get]
func (h *Handlers) listInvites(c *gin.Context) {
	if !h.invitesEnabled(c) {
		return
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var out []models.WorkspaceInvite
	h.db.Where("workspace_id = ? AND revoked_at IS NULL AND expires_at > ? AND uses < max_uses", ws.ID, time.Now()).
		Order("created_at DESC").Find(&out)
	if out == nil {
		out = []models.WorkspaceInvite{}
	}
	c.JSON(http.StatusOK, out)
}

// revokeInvite ends a link (owners).
//
//	@Summary	Revoke an invite link
//	@Tags		workspaces
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Param		id	path	string	true	"Invite id"
//	@Success	204
//	@Router		/workspaces/{ws}/invites/{id} [delete]
func (h *Handlers) revokeInvite(c *gin.Context) {
	if !h.invitesEnabled(c) {
		return
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	now := time.Now()
	res := h.db.Model(&models.WorkspaceInvite{}).Where("id = ? AND workspace_id = ? AND revoked_at IS NULL", c.Param("id"), ws.ID).Update("revoked_at", &now)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite not found"})
		return
	}
	h.recordCtx(c, "workspace.invite_revoke", "workspace", ws.ID, gin.H{"invite": c.Param("id")})
	c.Status(http.StatusNoContent)
}

// liveInvite loads the :token invite if it can still be used.
func (h *Handlers) liveInvite(c *gin.Context) (models.WorkspaceInvite, models.Workspace, bool) {
	var inv models.WorkspaceInvite
	var ws models.Workspace
	gone := func() (models.WorkspaceInvite, models.Workspace, bool) {
		c.JSON(http.StatusNotFound, gin.H{"error": "this invite link has expired or was revoked; ask for a new one"})
		return inv, ws, false
	}
	if h.db.First(&inv, "token_hash = ?", hashInvite(c.Param("token"))).Error != nil {
		return gone()
	}
	if inv.RevokedAt != nil || !time.Now().Before(inv.ExpiresAt) || inv.Uses >= inv.MaxUses {
		return gone()
	}
	if h.db.First(&ws, "id = ?", inv.WorkspaceID).Error != nil || ws.ArchivedAt != nil {
		return gone()
	}
	// A link stops working when whoever made it is no longer an owner.
	var n int64
	h.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ? AND user_id = ? AND role = ?", ws.ID, inv.CreatedBy, models.WSOwner).Count(&n)
	if n == 0 {
		return gone()
	}
	return inv, ws, true
}

type invitePreview struct {
	Workspace   string `json:"workspace"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	InvitedBy   string `json:"invited_by"`
	Role        string `json:"role"`
	CanSignUp   bool   `json:"can_sign_up"` // an account can be made here (password sign-in isn't hidden)
	ExpiresAt   string `json:"expires_at"`
	MemberCount int64  `json:"member_count"`
}

// previewInvite tells what a link is for (public: the join page asks it
// before anyone signs in).
//
//	@Summary	What an invite link is for
//	@Tags		workspaces
//	@Produce	json
//	@Param		token	path		string	true	"Invite token"
//	@Success	200		{object}	invitePreview
//	@Router		/invites/{token} [get]
func (h *Handlers) previewInvite(c *gin.Context) {
	if !h.invitesEnabled(c) {
		return
	}
	inv, ws, ok := h.liveInvite(c)
	if !ok {
		return
	}
	var n int64
	h.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ?", ws.ID).Count(&n)
	c.JSON(http.StatusOK, invitePreview{Workspace: ws.Name, Icon: ws.Icon, Color: ws.Color, InvitedBy: h.actorName(inv.CreatedBy),
		Role: inv.Role, CanSignUp: !h.hideLocalLogin(), ExpiresAt: inv.ExpiresAt.Format(time.RFC3339), MemberCount: n})
}

// join makes u a member through inv (keeping a higher role they already
// have) and uses the link once. It reports whether u was already a member;
// someone already in, whom the link doesn't raise, doesn't use it up.
func (h *Handlers) join(c *gin.Context, inv models.WorkspaceInvite, ws models.Workspace, u models.User) (bool, error) {
	already := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var m models.WorkspaceMember
		found := tx.Where("workspace_id = ? AND user_id = ?", ws.ID, u.ID).First(&m).Error == nil
		already = found
		raise := found && m.Role == models.WSViewer && inv.Role == models.WSEditor
		if found && !raise {
			return nil
		}
		// Spend a use (still usable: the update is the check, against races).
		res := tx.Model(&models.WorkspaceInvite{}).Where("id = ? AND uses < max_uses AND revoked_at IS NULL", inv.ID).
			Update("uses", gorm.Expr("uses + 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return badRequest("this invite link has just been used up; ask for a new one")
		}
		if raise {
			return tx.Model(&m).Update("role", models.WSEditor).Error
		}
		var last struct{ Max *float64 }
		tx.Model(&models.WorkspaceMember{}).Select("MAX(position) AS max").Where("user_id = ?", u.ID).Scan(&last)
		return tx.Create(&models.WorkspaceMember{WorkspaceID: ws.ID, UserID: u.ID, Role: inv.Role, Position: planner.Rank(last.Max, nil)}).Error
	})
	if err != nil {
		return false, err
	}
	h.audit.Record(u.ID, "workspace.invite_accept", "workspace", ws.ID, c.ClientIP(), gin.H{"role": inv.Role, "prefix": inv.Prefix, "invited_by": inv.CreatedBy})
	h.publishWS(c, ws.ID, "members")
	if !already {
		h.notifyJoined(c, ws, u, inv.Role)
	}
	return already, nil
}

// acceptInvite joins the signed-in caller to the workspace.
//
//	@Summary	Accept an invite link
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Param		token	path		string	true	"Invite token"
//	@Success	200		{object}	map[string]any
//	@Router		/invites/{token}/accept [post]
func (h *Handlers) acceptInvite(c *gin.Context) {
	if !h.invitesEnabled(c) {
		return
	}
	inv, ws, ok := h.liveInvite(c)
	if !ok {
		return
	}
	var u models.User
	if h.db.First(&u, "id = ?", auth.UserID(c)).Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found"})
		return
	}
	already, err := h.join(c, inv, ws, u)
	if writeTxError(c, h, err, "could not join the workspace") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"workspace_id": ws.ID, "already_member": already})
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

type inviteSignupRequest struct {
	Username    string `json:"username" binding:"required,max=32"`
	DisplayName string `json:"display_name" binding:"max=80"`
	Email       string `json:"email" binding:"omitempty,email,max=255"`
	Password    string `json:"password" binding:"required,min=8,max=200"`
}

// signupInvite makes an account through a link and joins the workspace
// with it, signed in (public; rate-limited). Not when the sign-in page hides
// the password form for SSO: then people sign in with SSO and accept.
//
//	@Summary	Create an account through an invite link
//	@Tags		workspaces
//	@Accept		json
//	@Produce	json
//	@Param		token	path		string				true	"Invite token"
//	@Param		body	body		inviteSignupRequest	true	"The new account"
//	@Success	201		{object}	inviteSignupResponse
//	@Router		/invites/{token}/signup [post]
func (h *Handlers) signupInvite(c *gin.Context) {
	if !h.invitesEnabled(c) {
		return
	}
	inv, ws, ok := h.liveInvite(c)
	if !ok {
		return
	}
	if h.hideLocalLogin() {
		c.JSON(http.StatusForbidden, gin.H{"error": "accounts here are made by signing in with SSO; sign in, then open the link again"})
		return
	}
	var req inviteSignupRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if !usernameRe.MatchString(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a username is 3 to 32 lowercase letters, digits, dots, dashes or underscores"})
		return
	}
	var n int64
	h.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&n)
	if n > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "that username is taken"})
		return
	}
	if h.emailTaken(req.Email, "") {
		c.JSON(http.StatusConflict, gin.H{"error": errEmailTaken})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.internalError(c, err, "could not create the account")
		return
	}
	u := models.User{
		Username: req.Username, DisplayName: strings.TrimSpace(req.DisplayName), Email: strings.TrimSpace(req.Email), PasswordHash: hash,
		Prefs: models.UserPrefs{Timezone: "local", Theme: models.ThemeSystem, WeekStart: 1, TimeFormat: "24h"},
	}
	if err := h.db.Create(&u).Error; err != nil {
		h.internalError(c, err, "could not create the account")
		return
	}
	// The built-in member role, as an admin-made account gets.
	var member models.Role
	if h.db.Where("name = ? AND builtin = ?", string(models.RoleMember), true).First(&member).Error == nil {
		_ = h.db.Model(&u).Association("Roles").Replace([]models.Role{member})
	}
	h.audit.Record(u.ID, "user.create", "user", u.ID, c.ClientIP(), gin.H{"username": u.Username, "via": "invite", "invited_by": inv.CreatedBy, "workspace": ws.ID})
	h.notifyUsersChanged()
	if _, err := h.join(c, inv, ws, u); writeTxError(c, h, err, "could not join the workspace") {
		return
	}
	h.db.Preload("Prefs").Preload("Roles").First(&u, "id = ?", u.ID)
	resp, err := h.issueTokens(c, &u, time.Now(), true)
	if err != nil {
		h.internalError(c, err, "could not sign you in")
		return
	}
	c.JSON(http.StatusCreated, inviteSignupResponse{tokenResponse: resp, WorkspaceID: ws.ID})
}

type inviteSignupResponse struct {
	*tokenResponse
	WorkspaceID string `json:"workspace_id"`
}
