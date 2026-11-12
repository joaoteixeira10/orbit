package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SCIM 2.0 (RFC 7643 / 7644): an identity provider — Okta, Microsoft Entra —
// creates, changes and deactivates accounts and groups here as it does in its
// directory. It reaches /scim/v2 with a token the owner makes (kept only as
// a hash). It touches only what it made: accounts it created (SCIMManaged),
// groups of source "scim" — never someone's own account, never the owner.
// A deleted account is disabled, not erased (its work stays). Its people
// sign in through SSO; their first sign-in finds the account by email.

const (
	scimSettingKey = "scim"
	scimUserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"
	scimGroupSch   = "urn:ietf:params:scim:schemas:core:2.0:Group"
	scimListSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	scimErrSchema  = "urn:ietf:params:scim:api:messages:2.0:Error"
	scimPatchSch   = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
)

type scimSettings struct {
	Enabled     bool   `json:"enabled"`
	TokenHash   string `json:"token_hash,omitempty"`
	DefaultRole string `json:"default_role"` // a new account's role ("" none)
}

func (h *Handlers) scimSettings() scimSettings {
	var row models.Setting
	var s scimSettings
	if h.db.Where(&models.Setting{Key: scimSettingKey}).Limit(1).Find(&row).RowsAffected == 1 {
		_ = json.Unmarshal([]byte(row.Value), &s)
	}
	return s
}

func (h *Handlers) saveSCIMSettings(s scimSettings) error {
	b, _ := json.Marshal(s)
	return h.db.Save(&models.Setting{Key: scimSettingKey, Value: string(b), UpdatedAt: time.Now()}).Error
}

func scimHash(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func scimFail(c *gin.Context, status int, detail string) {
	c.Header("Content-Type", "application/scim+json")
	c.AbortWithStatusJSON(status, gin.H{"schemas": []string{scimErrSchema}, "status": strconv.Itoa(status), "detail": detail})
}

func scimJSON(c *gin.Context, status int, v any) {
	c.Header("Content-Type", "application/scim+json")
	c.JSON(status, v)
}

// scimAuth admits the identity provider: SCIM on, its token.
func (h *Handlers) scimAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		s := h.scimSettings()
		raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !s.Enabled || s.TokenHash == "" || !ok ||
			subtle.ConstantTimeCompare([]byte(scimHash(strings.TrimSpace(raw))), []byte(s.TokenHash)) != 1 {
			scimFail(c, http.StatusUnauthorized, "SCIM is off, or this isn't its token")
			return
		}
		c.Next()
	}
}

// ---- The owner's settings ----

type scimConfigResponse struct {
	Enabled     bool   `json:"enabled"`
	HasToken    bool   `json:"has_token"`
	DefaultRole string `json:"default_role"`
	URL         string `json:"url"` // the base URL to give the identity provider
}

// getSCIMConfig: whether SCIM is on, and where.
//
//	@Summary	SCIM provisioning settings
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	scimConfigResponse
//	@Router		/scim-config [get]
func (h *Handlers) getSCIMConfig(c *gin.Context) {
	s := h.scimSettings()
	c.JSON(http.StatusOK, scimConfigResponse{Enabled: s.Enabled, HasToken: s.TokenHash != "", DefaultRole: s.DefaultRole, URL: h.baseURL(c) + "/scim/v2"})
}

type scimConfigRequest struct {
	Enabled     bool   `json:"enabled"`
	DefaultRole string `json:"default_role"`
	NewToken    bool   `json:"new_token"` // make a new token (the old one stops working)
}

// setSCIMConfig turns SCIM on or off, sets new accounts' role, or makes a
// new token (shown once).
//
//	@Summary	Change SCIM provisioning settings
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		scimConfigRequest	true	"Settings"
//	@Success	200		{object}	map[string]any
//	@Router		/scim-config [put]
func (h *Handlers) setSCIMConfig(c *gin.Context) {
	var req scimConfigRequest
	if !bindJSON(c, &req) {
		return
	}
	s := h.scimSettings()
	s.Enabled, s.DefaultRole = req.Enabled, strings.TrimSpace(req.DefaultRole)
	if s.DefaultRole != "" {
		var r models.Role
		if h.db.Where("name = ?", s.DefaultRole).First(&r).Error != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no role of that name"})
			return
		}
		if r.Superuser {
			c.JSON(http.StatusBadRequest, gin.H{"error": "new accounts can't be admins by default"})
			return
		}
	}
	token := ""
	if req.NewToken {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		token = "scim_" + hex.EncodeToString(b)
		s.TokenHash = scimHash(token)
	}
	if err := h.saveSCIMSettings(s); err != nil {
		h.internalError(c, err, "could not save SCIM settings")
		return
	}
	h.recordCtx(c, "auth.scim_config", "settings", "scim", gin.H{"enabled": s.Enabled, "new_token": req.NewToken})
	h.securityAlert(c, "config_change", "SCIM provisioning changed")
	out := gin.H{"enabled": s.Enabled, "has_token": s.TokenHash != "", "default_role": s.DefaultRole, "url": h.baseURL(c) + "/scim/v2"}
	if token != "" {
		out["token"] = token // shown once
	}
	c.JSON(http.StatusOK, out)
}

// ---- Discovery ----

func (h *Handlers) scimServiceProvider(c *gin.Context) {
	scimJSON(c, http.StatusOK, gin.H{
		"schemas":        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":          gin.H{"supported": true},
		"bulk":           gin.H{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":         gin.H{"supported": true, "maxResults": 200},
		"changePassword": gin.H{"supported": false},
		"sort":           gin.H{"supported": false},
		"etag":           gin.H{"supported": false},
		"authenticationSchemes": []gin.H{{"type": "oauthbearertoken", "name": "Bearer token",
			"description": "The token made in Orbit (Owner → SSO → Provisioning)"}},
	})
}

func (h *Handlers) scimResourceTypes(c *gin.Context) {
	scimJSON(c, http.StatusOK, gin.H{"schemas": []string{scimListSchema}, "totalResults": 2, "Resources": []gin.H{
		{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"}, "id": "User", "name": "User", "endpoint": "/Users", "schema": scimUserSchema},
		{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"}, "id": "Group", "name": "Group", "endpoint": "/Groups", "schema": scimGroupSch},
	}})
}

// ---- Users ----

type scimName struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type scimEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type scimUser struct {
	Schemas     []string    `json:"schemas"`
	ID          string      `json:"id,omitempty"`
	ExternalID  string      `json:"externalId,omitempty"`
	UserName    string      `json:"userName"`
	Name        *scimName   `json:"name,omitempty"`
	DisplayName string      `json:"displayName,omitempty"`
	Emails      []scimEmail `json:"emails,omitempty"`
	Active      *bool       `json:"active,omitempty"`
	Meta        gin.H       `json:"meta,omitempty"`
}

func (h *Handlers) toSCIMUser(c *gin.Context, u models.User) scimUser {
	active := !u.Disabled
	out := scimUser{Schemas: []string{scimUserSchema}, ID: u.ID, ExternalID: u.SCIMID, UserName: u.Username,
		Name: &scimName{Formatted: u.DisplayName}, DisplayName: u.DisplayName, Active: &active,
		Meta: gin.H{"resourceType": "User", "created": u.CreatedAt.UTC().Format(time.RFC3339), "lastModified": u.UpdatedAt.UTC().Format(time.RFC3339),
			"location": h.baseURL(c) + "/scim/v2/Users/" + u.ID}}
	if u.Email != "" {
		out.Emails = []scimEmail{{Value: u.Email, Type: "work", Primary: true}}
	}
	return out
}

// apply copies what was sent onto an account.
func (su scimUser) apply(u *models.User) {
	if su.UserName != "" {
		u.Username = strings.TrimSpace(su.UserName)
	}
	if su.ExternalID != "" {
		u.SCIMID = su.ExternalID
	}
	switch {
	case su.DisplayName != "":
		u.DisplayName = su.DisplayName
	case su.Name != nil && su.Name.Formatted != "":
		u.DisplayName = su.Name.Formatted
	case su.Name != nil && (su.Name.GivenName != "" || su.Name.FamilyName != ""):
		u.DisplayName = strings.TrimSpace(su.Name.GivenName + " " + su.Name.FamilyName)
	}
	for i, e := range su.Emails {
		if e.Primary || i == 0 {
			u.Email = strings.TrimSpace(e.Value)
		}
	}
	if su.Active != nil {
		u.Disabled = !*su.Active
	}
}

// scimFilter reads `attr eq "value"` (the filters identity providers send).
var scimFilter = regexp.MustCompile(`(?i)^\s*([a-z.]+)\s+eq\s+"([^"]*)"\s*$`)

func scimPage(c *gin.Context) (int, int) {
	start, _ := strconv.Atoi(c.DefaultQuery("startIndex", "1"))
	count, _ := strconv.Atoi(c.DefaultQuery("count", "100"))
	return max(start, 1), min(max(count, 0), 200)
}

func (h *Handlers) scimListUsers(c *gin.Context) {
	q := h.db.Model(&models.User{}).Where("scim_managed = ?", true)
	if f := c.Query("filter"); f != "" {
		m := scimFilter.FindStringSubmatch(f)
		if m == nil {
			scimFail(c, http.StatusBadRequest, "only `attribute eq \"value\"` filters")
			return
		}
		switch strings.ToLower(m[1]) {
		case "username":
			q = q.Where("LOWER(username) = ?", strings.ToLower(m[2]))
		case "externalid":
			q = q.Where("scim_id = ?", m[2])
		case "emails.value", "emails":
			q = q.Where("LOWER(email) = ?", strings.ToLower(m[2]))
		default:
			scimFail(c, http.StatusBadRequest, "filter by userName, externalId or emails.value")
			return
		}
	}
	var total int64
	q.Count(&total)
	start, count := scimPage(c)
	var users []models.User
	q.Order("created_at").Offset(start - 1).Limit(count).Find(&users)
	res := make([]scimUser, len(users))
	for i, u := range users {
		res[i] = h.toSCIMUser(c, u)
	}
	scimJSON(c, http.StatusOK, gin.H{"schemas": []string{scimListSchema}, "totalResults": total, "startIndex": start, "itemsPerPage": len(res), "Resources": res})
}

// scimUserOf loads :id — one SCIM made.
func (h *Handlers) scimUserOf(c *gin.Context) (models.User, bool) {
	var u models.User
	if h.db.First(&u, "id = ? AND scim_managed = ?", c.Param("id"), true).Error != nil {
		scimFail(c, http.StatusNotFound, "no such user")
		return u, false
	}
	return u, true
}

func (h *Handlers) scimGetUser(c *gin.Context) {
	if u, ok := h.scimUserOf(c); ok {
		scimJSON(c, http.StatusOK, h.toSCIMUser(c, u))
	}
}

func (h *Handlers) scimCreateUser(c *gin.Context) {
	var su scimUser
	if err := c.ShouldBindJSON(&su); err != nil || strings.TrimSpace(su.UserName) == "" {
		scimFail(c, http.StatusBadRequest, "userName is required")
		return
	}
	var n int64
	h.db.Model(&models.User{}).Where("LOWER(username) = ?", strings.ToLower(strings.TrimSpace(su.UserName))).Count(&n)
	if n > 0 {
		c.Header("Content-Type", "application/scim+json")
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"schemas": []string{scimErrSchema}, "status": "409", "scimType": "uniqueness", "detail": "an account with that userName exists"})
		return
	}
	u := models.User{SCIMManaged: true}
	su.apply(&u)
	if err := h.db.Create(&u).Error; err != nil {
		scimFail(c, http.StatusInternalServerError, "could not create the account")
		return
	}
	s := h.scimSettings()
	roles := h.mapOIDCRoles(nil, s.DefaultRole, false)
	_ = h.db.Model(&u).Association("Roles").Replace(roles)
	h.audit.Record("", "scim.user_create", "user", u.ID, c.ClientIP(), gin.H{"username": u.Username})
	scimJSON(c, http.StatusCreated, h.toSCIMUser(c, u))
}

func (h *Handlers) scimReplaceUser(c *gin.Context) {
	u, ok := h.scimUserOf(c)
	if !ok {
		return
	}
	var su scimUser
	if err := c.ShouldBindJSON(&su); err != nil {
		scimFail(c, http.StatusBadRequest, "a User")
		return
	}
	h.saveSCIMUser(c, u, func(x *models.User) { su.apply(x) })
}

type scimPatch struct {
	Operations []struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	} `json:"Operations"`
}

// scimBool reads a boolean as sent (Entra sends "False").
func scimBool(raw json.RawMessage) (bool, bool) {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		v, err := strconv.ParseBool(s)
		return v, err == nil
	}
	return false, false
}

func scimString(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func (h *Handlers) scimPatchUser(c *gin.Context) {
	u, ok := h.scimUserOf(c)
	if !ok {
		return
	}
	var p scimPatch
	if err := c.ShouldBindJSON(&p); err != nil {
		scimFail(c, http.StatusBadRequest, "a PatchOp")
		return
	}
	h.saveSCIMUser(c, u, func(x *models.User) {
		for _, op := range p.Operations {
			if strings.EqualFold(op.Op, "remove") {
				continue
			}
			switch path := strings.ToLower(op.Path); {
			case path == "":
				var su scimUser
				_ = json.Unmarshal(op.Value, &su)
				var raw map[string]json.RawMessage // {"active": "False"}
				if json.Unmarshal(op.Value, &raw) == nil {
					if v, ok := scimBool(raw["active"]); ok {
						su.Active = &v
					}
				}
				su.apply(x)
			case path == "active":
				if v, ok := scimBool(op.Value); ok {
					x.Disabled = !v
				}
			case path == "username":
				x.Username = strings.TrimSpace(scimString(op.Value))
			case path == "displayname", path == "name.formatted":
				x.DisplayName = scimString(op.Value)
			case path == "externalid":
				x.SCIMID = scimString(op.Value)
			case strings.HasPrefix(path, "emails"):
				if v := scimString(op.Value); v != "" {
					x.Email = v
				} else {
					var es []scimEmail
					if json.Unmarshal(op.Value, &es) == nil && len(es) > 0 {
						x.Email = es[0].Value
					}
				}
			}
		}
	})
}

func (h *Handlers) saveSCIMUser(c *gin.Context, u models.User, change func(*models.User)) {
	was := u.Disabled
	change(&u)
	if u.Username == "" {
		scimFail(c, http.StatusBadRequest, "userName can't be empty")
		return
	}
	if err := h.db.Save(&u).Error; err != nil {
		scimFail(c, http.StatusConflict, "could not save the account (is the userName taken?)")
		return
	}
	if u.Disabled && !was {
		h.scimDeactivated(u)
	}
	h.audit.Record("", "scim.user_update", "user", u.ID, c.ClientIP(), gin.H{"username": u.Username, "active": !u.Disabled})
	scimJSON(c, http.StatusOK, h.toSCIMUser(c, u))
}

// scimDeactivated: a disabled account's sessions end, and its group access
// goes (a sync leaves out disabled people).
func (h *Handlers) scimDeactivated(u models.User) {
	h.db.Model(&models.User{}).Where("id = ?", u.ID).Update("token_version", gorm.Expr("token_version + 1"))
	h.db.Model(&models.RefreshToken{}).Where("user_id = ?", u.ID).Update("revoked", true)
	if _, err := h.regroup(planner.UserGroupWorkspaces(h.db, u.ID), func(*gorm.DB) error { return nil }); err != nil {
		h.log.Sugar().Warnf("scim: %s's workspaces not resynced: %v", u.Username, err)
	}
}

func (h *Handlers) scimDeleteUser(c *gin.Context) {
	u, ok := h.scimUserOf(c)
	if !ok {
		return
	}
	if !u.Disabled {
		h.db.Model(&u).Update("disabled", true)
		h.scimDeactivated(u)
	}
	h.audit.Record("", "scim.user_delete", "user", u.ID, c.ClientIP(), gin.H{"username": u.Username})
	c.Status(http.StatusNoContent)
}

// ---- Groups ----

type scimMember struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
}

type scimGroup struct {
	Schemas     []string     `json:"schemas"`
	ID          string       `json:"id,omitempty"`
	ExternalID  string       `json:"externalId,omitempty"`
	DisplayName string       `json:"displayName"`
	Members     []scimMember `json:"members"`
	Meta        gin.H        `json:"meta,omitempty"`
}

func (h *Handlers) toSCIMGroup(c *gin.Context, g models.Group) scimGroup {
	out := scimGroup{Schemas: []string{scimGroupSch}, ID: g.ID, ExternalID: g.ExternalID, DisplayName: g.Name, Members: []scimMember{},
		Meta: gin.H{"resourceType": "Group", "created": g.CreatedAt.UTC().Format(time.RFC3339), "lastModified": g.UpdatedAt.UTC().Format(time.RFC3339),
			"location": h.baseURL(c) + "/scim/v2/Groups/" + g.ID}}
	h.db.Table("group_members").Select("users.id AS value, users.username AS display").
		Joins("JOIN users ON users.id = group_members.user_id").Where("group_members.group_id = ?", g.ID).Scan(&out.Members)
	return out
}

func (h *Handlers) scimListGroups(c *gin.Context) {
	q := h.db.Model(&models.Group{}).Where("source = ?", "scim")
	if f := c.Query("filter"); f != "" {
		m := scimFilter.FindStringSubmatch(f)
		switch {
		case m == nil:
			scimFail(c, http.StatusBadRequest, "only `attribute eq \"value\"` filters")
			return
		case strings.EqualFold(m[1], "displayName"):
			q = q.Where("name = ?", m[2])
		case strings.EqualFold(m[1], "externalId"):
			q = q.Where("external_id = ?", m[2])
		default:
			scimFail(c, http.StatusBadRequest, "filter by displayName or externalId")
			return
		}
	}
	var total int64
	q.Count(&total)
	start, count := scimPage(c)
	var gs []models.Group
	q.Order("created_at").Offset(start - 1).Limit(count).Find(&gs)
	res := make([]scimGroup, len(gs))
	for i, g := range gs {
		res[i] = h.toSCIMGroup(c, g)
	}
	scimJSON(c, http.StatusOK, gin.H{"schemas": []string{scimListSchema}, "totalResults": total, "startIndex": start, "itemsPerPage": len(res), "Resources": res})
}

func (h *Handlers) scimGroupOf(c *gin.Context) (models.Group, bool) {
	var g models.Group
	if h.db.First(&g, "id = ? AND source = ?", c.Param("id"), "scim").Error != nil {
		scimFail(c, http.StatusNotFound, "no such group")
		return g, false
	}
	return g, true
}

func (h *Handlers) scimGetGroup(c *gin.Context) {
	if g, ok := h.scimGroupOf(c); ok {
		scimJSON(c, http.StatusOK, h.toSCIMGroup(c, g))
	}
}

// scimSetMembers makes a group's people exactly ids (existing accounts),
// its workspaces following.
func (h *Handlers) scimMembers(g models.Group, add, remove []string, replace bool) error {
	_, err := h.regroup(planner.GroupWorkspaces(h.db, g.ID), func(tx *gorm.DB) error {
		if replace {
			if err := tx.Where("group_id = ?", g.ID).Delete(&models.GroupMember{}).Error; err != nil {
				return err
			}
		}
		if len(remove) > 0 {
			if err := tx.Where("group_id = ? AND user_id IN ?", g.ID, remove).Delete(&models.GroupMember{}).Error; err != nil {
				return err
			}
		}
		for _, uid := range add {
			var n int64
			tx.Model(&models.User{}).Where("id = ?", uid).Count(&n)
			if n == 0 {
				continue
			}
			m := models.GroupMember{GroupID: g.ID, UserID: uid}
			if err := tx.Where(m).FirstOrCreate(&m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func memberIDs(ms []scimMember) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Value)
	}
	return out
}

func (h *Handlers) scimMembersFailed(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, planner.ErrNoOwner) {
		scimFail(c, http.StatusConflict, err.Error())
	} else {
		scimFail(c, http.StatusInternalServerError, "could not change the group's members")
	}
	return true
}

func (h *Handlers) scimCreateGroup(c *gin.Context) {
	var sg scimGroup
	if err := c.ShouldBindJSON(&sg); err != nil || strings.TrimSpace(sg.DisplayName) == "" {
		scimFail(c, http.StatusBadRequest, "displayName is required")
		return
	}
	g := models.Group{Name: strings.TrimSpace(sg.DisplayName), Source: "scim", ExternalID: sg.ExternalID}
	var n int64
	h.db.Model(&models.Group{}).Where("name = ?", g.Name).Count(&n)
	if n > 0 {
		c.Header("Content-Type", "application/scim+json")
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"schemas": []string{scimErrSchema}, "status": "409", "scimType": "uniqueness", "detail": "a group of that name exists"})
		return
	}
	if err := h.db.Create(&g).Error; err != nil {
		scimFail(c, http.StatusInternalServerError, "could not create the group")
		return
	}
	if h.scimMembersFailed(c, h.scimMembers(g, memberIDs(sg.Members), nil, false)) {
		return
	}
	h.audit.Record("", "scim.group_create", "group", g.ID, c.ClientIP(), gin.H{"name": g.Name})
	scimJSON(c, http.StatusCreated, h.toSCIMGroup(c, g))
}

func (h *Handlers) scimReplaceGroup(c *gin.Context) {
	g, ok := h.scimGroupOf(c)
	if !ok {
		return
	}
	var sg scimGroup
	if err := c.ShouldBindJSON(&sg); err != nil {
		scimFail(c, http.StatusBadRequest, "a Group")
		return
	}
	if name := strings.TrimSpace(sg.DisplayName); name != "" && name != g.Name {
		if h.db.Model(&g).Update("name", name).Error != nil {
			scimFail(c, http.StatusConflict, "a group of that name exists")
			return
		}
		g.Name = name
	}
	if h.scimMembersFailed(c, h.scimMembers(g, memberIDs(sg.Members), nil, true)) {
		return
	}
	h.audit.Record("", "scim.group_update", "group", g.ID, c.ClientIP(), gin.H{"name": g.Name})
	scimJSON(c, http.StatusOK, h.toSCIMGroup(c, g))
}

// scimMemberPath: `members[value eq "id"]`.
var scimMemberPath = regexp.MustCompile(`(?i)^members\[value eq "([^"]+)"\]$`)

func (h *Handlers) scimPatchGroup(c *gin.Context) {
	g, ok := h.scimGroupOf(c)
	if !ok {
		return
	}
	var p scimPatch
	if err := c.ShouldBindJSON(&p); err != nil {
		scimFail(c, http.StatusBadRequest, "a PatchOp")
		return
	}
	var add, remove []string
	replace := false
	for _, op := range p.Operations {
		var ms []scimMember
		_ = json.Unmarshal(op.Value, &ms)
		switch o, path := strings.ToLower(op.Op), strings.ToLower(op.Path); {
		case path == "members" && o == "add":
			add = append(add, memberIDs(ms)...)
		case path == "members" && o == "replace":
			add, replace = memberIDs(ms), true
		case path == "members" && o == "remove":
			remove = append(remove, memberIDs(ms)...)
		case o == "remove" && scimMemberPath.MatchString(op.Path):
			remove = append(remove, scimMemberPath.FindStringSubmatch(op.Path)[1])
		case path == "displayname" || (path == "" && o != "remove"):
			name := scimString(op.Value)
			if path == "" {
				var sg scimGroup
				_ = json.Unmarshal(op.Value, &sg)
				name = sg.DisplayName
				if sg.Members != nil {
					add = append(add, memberIDs(sg.Members)...)
				}
			}
			if name = strings.TrimSpace(name); name != "" && name != g.Name {
				if h.db.Model(&g).Update("name", name).Error != nil {
					scimFail(c, http.StatusConflict, "a group of that name exists")
					return
				}
				g.Name = name
			}
		}
	}
	if h.scimMembersFailed(c, h.scimMembers(g, add, remove, replace)) {
		return
	}
	h.audit.Record("", "scim.group_update", "group", g.ID, c.ClientIP(), gin.H{"name": g.Name, "added": len(add), "removed": len(remove)})
	scimJSON(c, http.StatusOK, h.toSCIMGroup(c, g))
}

func (h *Handlers) scimDeleteGroup(c *gin.Context) {
	g, ok := h.scimGroupOf(c)
	if !ok {
		return
	}
	_, err := h.regroup(planner.GroupWorkspaces(h.db, g.ID), func(tx *gorm.DB) error {
		for _, m := range []any{&models.WorkspaceGroup{}, &models.GroupMember{}, &models.GroupRole{}} {
			if err := tx.Where("group_id = ?", g.ID).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&g).Error
	})
	if h.scimMembersFailed(c, err) {
		return
	}
	h.audit.Record("", "scim.group_delete", "group", g.ID, c.ClientIP(), gin.H{"name": g.Name})
	c.Status(http.StatusNoContent)
}

// scimRoutes mounts /scim/v2 on r.
func (h *Handlers) scimRoutes(r *gin.Engine) {
	g := r.Group("/scim/v2", h.scimAuth())
	g.GET("/ServiceProviderConfig", h.scimServiceProvider)
	g.GET("/ResourceTypes", h.scimResourceTypes)
	g.GET("/Users", h.scimListUsers)
	g.POST("/Users", h.scimCreateUser)
	g.GET("/Users/:id", h.scimGetUser)
	g.PUT("/Users/:id", h.scimReplaceUser)
	g.PATCH("/Users/:id", h.scimPatchUser)
	g.DELETE("/Users/:id", h.scimDeleteUser)
	g.GET("/Groups", h.scimListGroups)
	g.POST("/Groups", h.scimCreateGroup)
	g.GET("/Groups/:id", h.scimGetGroup)
	g.PUT("/Groups/:id", h.scimReplaceGroup)
	g.PATCH("/Groups/:id", h.scimPatchGroup)
	g.DELETE("/Groups/:id", h.scimDeleteGroup)
}
