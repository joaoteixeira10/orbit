package api

import (
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// listUsers returns all users with their roles.
//
//	@Summary	List users
//	@Tags		users
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	models.User
//	@Router		/users [get]
func (h *Handlers) listUsers(c *gin.Context) {
	users := []models.User{}
	if err := h.db.Preload("Prefs").Preload("Roles").Order("id").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list users"})
		return
	}
	var withPasskey []string
	h.db.Model(&models.WebAuthnCredential{}).Distinct("user_id").Pluck("user_id", &withPasskey)
	has := map[string]bool{}
	for _, id := range withPasskey {
		has[id] = true
	}
	enforced := h.otpEnforced()
	for i := range users {
		u := &users[i]
		u.HasPasskey = has[u.ID]
		u.NeedsOTPSetup = !u.OIDC && !u.OTPEnabled && !u.HasPasskey && (u.OTPRequired || enforced)
	}
	c.JSON(http.StatusOK, users)
}

// createUser creates a new account and assigns roles.
//
//	@Summary	Create user
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		createUserRequest	true	"New user"
//	@Success	201		{object}	models.User
//	@Failure	409		{object}	map[string]string
//	@Router		/users [post]
func (h *Handlers) createUser(c *gin.Context) {
	var req createUserRequest
	if !bindJSON(c, &req) {
		return
	}
	var existing int64
	h.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&existing)
	if existing > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}
	if h.emailTaken(req.Email, "") {
		c.JSON(http.StatusConflict, gin.H{"error": errEmailTaken})
		return
	}
	// Only the instance owner may create an admin (superuser) account.
	if h.rolesIncludeSuperuser(req.RoleIDs) && !auth.IsOwner(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the instance owner can create an admin"})
		return
	}
	// Resolve the password: generate one, or validate the supplied one.
	password := req.Password
	generated := ""
	if req.GeneratePassword {
		password = auth.TempPassword()
		generated = password
	} else if len(password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}
	user := models.User{
		Username:           req.Username,
		DisplayName:        strings.TrimSpace(req.DisplayName),
		Email:              strings.TrimSpace(req.Email),
		PasswordHash:       hash,
		MustChangePassword: req.MustChangePassword || req.GeneratePassword, // a generated password is always temporary
		OTPRequired:        req.OTPRequired,
		Prefs:              models.UserPrefs{Timezone: "local", Theme: models.ThemeSystem, WeekStart: 1, TimeFormat: "24h"},
	}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create user"})
		return
	}
	// No role_ids at all: a member (creates their own workspaces). An empty
	// list means no instance role (they only see workspaces they're invited to).
	if req.RoleIDs == nil {
		var member models.Role
		if h.db.Where("name = ? AND builtin = ?", string(models.RoleMember), true).First(&member).Error == nil {
			req.RoleIDs = []string{member.ID}
		}
	}
	if roles := h.loadRoles(req.RoleIDs); len(roles) > 0 {
		_ = h.db.Model(&user).Association("Roles").Replace(&roles)
	}
	h.recordCtx(c, "user.create", "user", user.ID,
		gin.H{"username": user.Username, "role_ids": req.RoleIDs, "otp_required": req.OTPRequired})
	if h.rolesIncludeSuperuser(req.RoleIDs) {
		h.securityAlert(c, "admin_grant", "Admin (superuser) account created: "+user.Username)
	}
	h.notifyUsersChanged()
	h.db.Preload("Prefs").Preload("Roles").First(&user, "id = ?", user.ID)
	// The generated password is returned exactly once.
	c.JSON(http.StatusCreated, gin.H{"user": user, "generated_password": generated})
}

// getUser returns a single user with roles.
//
//	@Summary	Get user
//	@Tags		users
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"User ID"
//	@Success	200	{object}	models.User
//	@Failure	404	{object}	map[string]string
//	@Router		/users/{id} [get]
func (h *Handlers) getUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var user models.User
	if err := h.db.Preload("Prefs").Preload("Roles").First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// updateUser updates email, disabled, and/or role assignments.
//
//	@Summary	Update user
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		int					true	"User ID"
//	@Param		body	body		updateUserRequest	true	"Fields to update"
//	@Success	200		{object}	models.User
//	@Router		/users/{id} [put]
func (h *Handlers) updateUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req updateUserRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !h.mayManage(c, user) {
		return
	}

	// Only the instance owner may grant or revoke the admin (superuser) role, so
	// admins can't mint or unmint other admins.
	if req.RoleIDs != nil && !auth.IsOwner(c) &&
		h.rolesIncludeSuperuser(*req.RoleIDs) != h.userIsSuperuser(user.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the instance owner can grant or revoke the admin role"})
		return
	}

	// Guard: don't strip superuser/disable the last active superuser.
	if h.userIsSuperuser(user.ID) {
		willBeSuper := true
		if req.RoleIDs != nil {
			willBeSuper = h.rolesIncludeSuperuser(*req.RoleIDs)
		}
		willBeDisabled := user.Disabled
		if req.Disabled != nil {
			willBeDisabled = *req.Disabled
		}
		if (!willBeSuper || willBeDisabled) && h.isLastSuperuser(user.ID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot demote or disable the last active admin"})
			return
		}
	}

	// You can't lock yourself out by disabling your own account.
	if req.Disabled != nil && *req.Disabled && user.ID == auth.UserID(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you can't disable your own account"})
		return
	}

	wasSuper := h.userIsSuperuser(user.ID) // for the admin-grant security alert

	updates := map[string]any{}
	// SSO accounts: email is owned by the identity provider; ignore changes.
	if req.Email != nil && user.OIDCSubject == "" {
		email := strings.TrimSpace(*req.Email)
		if h.emailTaken(email, user.ID) {
			c.JSON(http.StatusConflict, gin.H{"error": errEmailTaken})
			return
		}
		updates["email"] = email
	}
	if req.DisplayName != nil && user.OIDCSubject == "" {
		updates["display_name"] = strings.TrimSpace(*req.DisplayName)
	}
	disabling := req.Disabled != nil && *req.Disabled && !user.Disabled
	if req.Disabled != nil {
		updates["disabled"] = *req.Disabled
	}
	if disabling {
		// Disabling signs the user out everywhere: live access tokens die now
		// (and stay dead if the account is enabled again).
		updates["token_version"] = user.TokenVersion + 1
	}
	if req.OTPRequired != nil {
		updates["otp_required"] = *req.OTPRequired
	}
	if len(updates) > 0 {
		if err := h.db.Model(&user).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update user"})
			return
		}
	}
	if req.RoleIDs != nil {
		roles := h.loadRoles(*req.RoleIDs)
		if err := h.db.Model(&user).Association("Roles").Replace(&roles); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update roles"})
			return
		}
	}
	// On disable, revoke the user's sessions so they can't refresh either.
	if req.Disabled != nil && *req.Disabled {
		_ = h.auth.RevokeAllSessions(user.ID)
	}
	h.recordCtx(c, "user.update", "user", user.ID, updates)
	if req.Disabled != nil && *req.Disabled != user.Disabled {
		action := "user.enable"
		if *req.Disabled {
			action = "user.disable"
		}
		h.recordCtx(c, action, "user", user.ID, nil)
	}
	if req.RoleIDs != nil && !wasSuper && h.rolesIncludeSuperuser(*req.RoleIDs) {
		h.securityAlert(c, "admin_grant", "Admin (superuser) role granted to "+user.Username)
	}
	// A role or disabled change alters this user's effective privileges; nudge
	// their open sessions to re-sync so gated UI updates without a reload.
	// Requiring two-factor does the same: their next request gets
	// otp_setup_required and the SPA opens the setup screen.
	if req.RoleIDs != nil || req.Disabled != nil || req.OTPRequired != nil {
		h.notifyPrivilegeChange(user.ID)
	}
	h.notifyUsersChanged() // refresh any open admin user list
	h.db.Preload("Prefs").Preload("Roles").First(&user, "id = ?", id)
	c.JSON(http.StatusOK, user)
}

// resetUserPassword sets a new password for any user (admin only).
//
//	@Summary	Reset user password
//	@Tags		users
//	@Accept		json
//	@Security	BearerAuth
//	@Param		id		path	int						true	"User ID"
//	@Param		body	body	resetPasswordRequest	true	"New password, or generate: true"
//	@Success	200	{object}	map[string]string	"generated_password (only with generate: true)"
//	@Success	204
//	@Router		/users/{id}/password [put]
func (h *Handlers) resetUserPassword(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req resetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !h.mayManage(c, user) {
		return
	}
	if user.OIDCSubject != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this account signs in via SSO; password is managed by the identity provider"})
		return
	}
	generated := ""
	if req.Generate {
		req.NewPassword = auth.TempPassword()
		generated = req.NewPassword
		t := true
		req.MustChangePassword = &t // a generated password is always temporary
	} else if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}
	// An admin-set password is a temporary credential: by default the user must
	// replace it on next sign-in. Every existing session ends immediately — the
	// token-version bump rejects live access tokens, and the refresh tokens are
	// revoked so nothing can mint new ones.
	mustChange := req.MustChangePassword == nil || *req.MustChangePassword
	if err := h.db.Model(&user).Updates(map[string]any{
		"password_hash":        hash,
		"must_change_password": mustChange,
		"token_version":        user.TokenVersion + 1,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update password"})
		return
	}
	_ = h.auth.RevokeAllSessions(user.ID)
	h.db.Where("user_id = ?", user.ID).Delete(&models.APIToken{}) // signed out everywhere, scripts too
	h.recordCtx(c, "user.password_reset", "user", user.ID, gin.H{"must_change_password": mustChange, "generated": generated != ""})
	if generated != "" {
		// The generated password is returned exactly once.
		c.JSON(http.StatusOK, gin.H{"generated_password": generated})
		return
	}
	c.Status(http.StatusNoContent)
}

// unlockUser clears a lockout after too many failed sign-ins (admin only).
//
//	@Summary	Unlock user
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id	path	string	true	"User ID"
//	@Success	204
//	@Failure	404	{object}	map[string]string
//	@Router		/users/{id}/unlock [post]
func (h *Handlers) unlockUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !h.mayManage(c, user) {
		return
	}
	if err := h.auth.Unlock(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not unlock user"})
		return
	}
	h.recordCtx(c, "user.unlock", "user", user.ID, nil)
	h.notifyUsersChanged()
	c.Status(http.StatusNoContent)
}

// deleteUser removes a user (admin only).
//
//	@Summary	Delete user
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id	path	int	true	"User ID"
//	@Success	204
//	@Failure	400	{object}	map[string]string
//	@Router		/users/{id} [delete]
func (h *Handlers) deleteUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if id == auth.UserID(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !h.mayManage(c, user) {
		return
	}
	if h.userIsSuperuser(user.ID) && h.isLastSuperuser(user.ID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last active admin"})
		return
	}
	// Their workspaces first: ones they were alone in go, shared ones keep an owner.
	affected := []string{}
	h.db.Model(&models.WorkspaceMember{}).Where("workspace_id IN (?) AND user_id <> ?",
		h.db.Model(&models.WorkspaceMember{}).Select("workspace_id").Where("user_id = ?", user.ID), user.ID).
		Distinct().Pluck("user_id", &affected)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := releaseWorkspaces(tx, user.ID); err != nil {
			return err
		}
		return tx.Select("Prefs", "Roles").Delete(&user).Error
	}); err != nil {
		h.internalError(c, err, "could not delete user")
		return
	}
	for _, id := range affected {
		h.events.Publish(id, liveEvent("workspaces"))
	}
	h.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{})
	h.db.Where("user_id = ?", user.ID).Delete(&models.APIToken{})
	h.db.Where("created_by = ?", user.ID).Delete(&models.WorkspaceInvite{})
	h.db.Where("owner_id = ?", user.ID).Delete(&models.Grade{})
	h.db.Where("user_id = ?", user.ID).Delete(&models.CourseGoal{})
	h.db.Where("user_id = ?", user.ID).Delete(&models.Notification{})
	_ = uploads.Delete(h.db, "workspace_id = '' AND owner_id = ?", user.ID) // their Quick Notes' files
	h.db.Where("workspace_id = '' AND owner_id = ?", user.ID).Delete(&models.Note{})
	h.db.Where("user_id = ?", user.ID).Delete(&models.PushSubscription{})
	h.db.Where("user_id = ?", user.ID).Delete(&models.GitAccount{})
	h.recordCtx(c, "user.delete", "user", user.ID,
		gin.H{"username": user.Username})
	h.notifyUsersChanged()
	c.Status(http.StatusNoContent)
}

// resetUserOTP clears a user's two-factor enrollment (admin). If OTP is
// enforced, the user must re-enroll at next sign-in.
//
//	@Summary	Reset a user's two-factor
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id	path	int	true	"User ID"
//	@Success	204
//	@Router		/users/{id}/reset-otp [post]
func (h *Handlers) resetUserOTP(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !h.mayManage(c, user) {
		return
	}
	if err := h.db.Model(&user).Updates(map[string]any{
		"otp_enabled": false, "otp_secret_enc": nil, "otp_backup_codes": "",
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset 2FA"})
		return
	}
	h.recordCtx(c, "user.reset_otp", "user", user.ID, nil)
	c.Status(http.StatusNoContent)
}

// forceLogout immediately signs a user out everywhere: it bumps the user's
// TokenVersion (so every live access token is rejected on its next request)
// and revokes all refresh tokens (so they can't be refreshed). The effect is
// instant, not bounded by the access-token TTL. Independent of the self-service
// "sessions" feature flag.
//
//	@Summary	Force a user to sign out everywhere
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id	path	int	true	"User id"
//	@Success	204
//	@Router		/users/{id}/logout [post]
func (h *Handlers) forceLogout(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !h.mayManage(c, user) {
		return
	}
	// Invalidate live access tokens immediately (the middleware rejects tokens
	// whose tv claim no longer matches the user's TokenVersion).
	if err := h.db.Model(&user).Update("token_version", user.TokenVersion+1).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not sign the user out"})
		return
	}
	if err := h.auth.RevokeAllSessions(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not sign the user out"})
		return
	}
	// API tokens don't carry the token version: "everywhere" includes them.
	h.db.Where("user_id = ?", user.ID).Delete(&models.APIToken{})
	h.recordCtx(c, "user.force_logout", "user", user.ID, nil)
	c.Status(http.StatusNoContent)
}

// loadRoles loads the Role entities for the given ids.
func (h *Handlers) loadRoles(ids []string) []models.Role {
	if len(ids) == 0 {
		return nil
	}
	var roles []models.Role
	h.db.Where("id IN ?", ids).Find(&roles)
	return roles
}

// rolesIncludeSuperuser reports whether any of the role ids is a superuser role.
func (h *Handlers) rolesIncludeSuperuser(ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	var n int64
	h.db.Model(&models.Role{}).Where("id IN ? AND superuser = ?", ids, true).Count(&n)
	return n > 0
}

// mayManage refuses account actions (edit, roles, disable, reset password /
// two-factor / passkeys, unlock, sign out, delete, revoke tokens) on the owner
// — nobody acts on the owner's account; the owner does it in Settings — and on
// an admin (superuser) unless you are the owner: admins can't act on each
// other, and users.manage is no way to take over a stronger account. Your own
// account is handled by each handler's self rules.
func (h *Handlers) mayManage(c *gin.Context, target models.User) bool {
	if target.ID == auth.UserID(c) {
		return true
	}
	if target.Owner {
		c.JSON(http.StatusForbidden, gin.H{"error": "nobody can change the owner's account (the owner does it in Settings)"})
		return false
	}
	if !auth.IsOwner(c) && h.userIsSuperuser(target.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can change an admin's account"})
		return false
	}
	return true
}

// userIsSuperuser reports whether the user currently holds a superuser role.
func (h *Handlers) userIsSuperuser(userID string) bool {
	var n int64
	h.db.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.superuser = ?", userID, true).
		Count(&n)
	return n > 0
}

// isLastSuperuser reports whether userID is the only enabled superuser.
func (h *Handlers) isLastSuperuser(excludeUserID string) bool {
	var n int64
	h.db.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Joins("JOIN users ON users.id = user_roles.user_id").
		Where("roles.superuser = ? AND users.disabled = ? AND users.id <> ?", true, false, excludeUserID).
		Distinct("users.id").Count(&n)
	return n == 0
}

// errEmailTaken is the conflict message for a duplicate email address.
const errEmailTaken = "email address is already used by another account"

// emailTaken reports whether another account (excluding excludeID) already
// uses email, compared case-insensitively. Emails are where notifications go,
// so they must be unique; empty emails never conflict.
func (h *Handlers) emailTaken(email, excludeID string) bool {
	email = strings.TrimSpace(email)
	if email == "" {
		return false
	}
	q := h.db.Model(&models.User{}).Where("LOWER(email) = ?", strings.ToLower(email))
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	var n int64
	q.Count(&n)
	return n > 0
}
