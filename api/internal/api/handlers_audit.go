package api

import (
	"encoding/csv"
	"encoding/json"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// auditEntryView is an audit row enriched with human-readable names for the
// actor and the target entity, resolved at list time. Names degrade to "#<id>"
// (or "system") when the referenced entity no longer exists.
type auditEntryView struct {
	models.AuditLog
	UserName         string `json:"user_name"`
	TargetName       string `json:"target_name"`
	ImpersonatorName string `json:"impersonator_name,omitempty"`
}

// verifyAudit walks the tamper-evidence hash chain and reports the first break.
//
//	@Summary	Verify audit-log integrity
//	@Tags		audit
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	audit.VerifyResult
//	@Router		/audit/verify [get]
func (h *Handlers) verifyAudit(c *gin.Context) {
	c.JSON(http.StatusOK, h.audit.Verify())
}

// listAudit returns a page of audit entries, newest first (admin only).
//
//	@Summary	List audit log (paginated)
//	@Tags		audit
//	@Produce	json
//	@Security	BearerAuth
//	@Param		page		query	int	false	"Page number (1-based)"
//	@Param		per_page	query	int	false	"Page size (default 50, max 500)"
//	@Param		q			query	string	false	"Search action, detail, IP or user name"
//	@Param		actions		query	string	false	"Only these actions (comma-separated)"
//	@Param		impersonation	query	string	false	"1: only impersonations (start, end, and what was done meanwhile)"
//	@Success	200		{object}	map[string]interface{}
//	@Router		/audit [get]
func (h *Handlers) listAudit(c *gin.Context) {
	page, perPage := pageParams(c, 50, 500)
	q := h.auditQuery(c)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list audit log"})
		return
	}
	entries := []models.AuditLog{}
	if err := q.Order("id desc").Limit(perPage).Offset((page - 1) * perPage).Find(&entries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list audit log"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": h.enrichAudit(entries), "total": total, "page": page, "per_page": perPage})
}

// enrichAudit resolves actor + target names for a page of audit entries with a
// handful of batched lookups (one query per referenced entity type).
func (h *Handlers) enrichAudit(entries []models.AuditLog) []auditEntryView {
	// Collect the ids we need to resolve, per entity type.
	userIDs := map[string]struct{}{}
	byType := map[string]map[string]struct{}{} // target_type -> set of ids
	addTarget := func(typ, id string) {
		if byType[typ] == nil {
			byType[typ] = map[string]struct{}{}
		}
		byType[typ][id] = struct{}{}
	}
	for i := range entries {
		e := &entries[i]
		if e.UserID != "" {
			userIDs[e.UserID] = struct{}{}
		}
		if e.ImpersonatorID != nil {
			userIDs[*e.ImpersonatorID] = struct{}{}
		}
		if e.TargetID != "" {
			addTarget(e.TargetType, e.TargetID)
		}
	}

	users := h.resolveNames(&models.User{}, "username", merge(userIDs, byType["user"]))
	roles := h.resolveNames(&models.Role{}, "name", byType["role"])

	views := make([]auditEntryView, len(entries))
	for i := range entries {
		e := entries[i]
		v := auditEntryView{AuditLog: e}
		// Actor.
		switch {
		case e.UserID == "":
			v.UserName = "system"
		case users[e.UserID] != "":
			v.UserName = users[e.UserID]
		default:
			v.UserName = "#" + e.UserID
		}
		// Impersonator (the real admin behind an impersonated action).
		if e.ImpersonatorID != nil {
			if n := users[*e.ImpersonatorID]; n != "" {
				v.ImpersonatorName = n
			} else {
				v.ImpersonatorName = "#" + *e.ImpersonatorID
			}
		}
		// Target (left empty when not resolvable; the client falls back to type#id).
		if e.TargetID != "" {
			switch e.TargetType {
			case "user":
				v.TargetName = users[e.TargetID]
			case "role":
				v.TargetName = roles[e.TargetID]
			}
		}
		views[i] = v
	}
	return views
}

// resolveNames batch-loads id -> <nameCol> for a model, given a set of ids.
func (h *Handlers) resolveNames(model any, nameCol string, ids map[string]struct{}) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	type idName struct {
		ID   string
		Name string
	}
	var rows []idName
	h.db.Model(model).Select("id", nameCol+" as name").Where("id IN ?", list).Scan(&rows)
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out
}

// merge unions two id sets into a new one.
func merge(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for id := range a {
		out[id] = struct{}{}
	}
	for id := range b {
		out[id] = struct{}{}
	}
	return out
}

// auditQuery applies the audit filters: q searches who / what / where (the
// action, the detail — targets, scope, errors, reasons —, the IP, or the
// user's name); actions keeps only those actions.
func (h *Handlers) auditQuery(c *gin.Context) *gorm.DB {
	q := h.db.Model(&models.AuditLog{})
	if term := strings.TrimSpace(c.Query("q")); term != "" {
		like := "%" + strings.ToLower(term) + "%"
		q = q.Where("LOWER(action) LIKE ? OR LOWER(detail) LIKE ? OR ip LIKE ? OR user_id IN (?)",
			like, like, like, h.db.Model(&models.User{}).Select("id").Where("LOWER(username) LIKE ?", like))
	}
	if acts := c.Query("actions"); acts != "" {
		q = q.Where("action IN ?", strings.Split(acts, ","))
	}
	// Only impersonations: their starts and ends, and what was done meanwhile.
	if c.Query("impersonation") == "1" {
		q = q.Where("impersonator_id IS NOT NULL OR action IN ?", []string{"user.impersonate", "user.impersonate_end"})
	}
	return q
}

// auditExportMax bounds one CSV export.
const auditExportMax = 100000

// exportAudit downloads the audit log (or what matches q) as CSV, newest
// first, with the chain hash of each entry.
//
//	@Summary	Export the audit log (CSV)
//	@Tags		audit
//	@Produce	text/csv
//	@Security	BearerAuth
//	@Param		q		query	string	false	"Search action, detail, IP or user name"
//	@Param		actions	query	string	false	"Only these actions (comma-separated)"
//	@Param		impersonation	query	string	false	"1: only impersonations"
//	@Success	200
//	@Router		/audit/export [get]
func (h *Handlers) exportAudit(c *gin.Context) {
	var rows []models.AuditLog
	if err := h.auditQuery(c).Order("id desc").Limit(auditExportMax).Find(&rows).Error; err != nil {
		h.internalError(c, err, "could not export the audit log")
		return
	}
	views := h.enrichAudit(rows)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="orbit-audit-`+time.Now().UTC().Format("20060102-150405")+`.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"time", "action", "user", "impersonator", "ip", "target_type", "target", "scope", "targets", "ok", "error", "reason", "detail", "hash"})
	for _, v := range views {
		var d struct {
			Targets []string `json:"targets"`
			Scope   string   `json:"scope"`
			OK      *bool    `json:"ok"`
			Error   string   `json:"error"`
			Reason  string   `json:"reason"`
			Actor   string   `json:"actor"`
		}
		_ = json.Unmarshal([]byte(v.Detail), &d)
		user := v.UserName
		if v.UserID == "" && d.Actor != "" {
			user = d.Actor
		}
		ok := ""
		if d.OK != nil {
			ok = strconv.FormatBool(*d.OK)
		}
		target := v.TargetName
		if target == "" && v.TargetID != "" {
			target = v.TargetID
		}
		_ = w.Write([]string{v.CreatedAt.UTC().Format(time.RFC3339), v.Action, csvSafe(user), csvSafe(v.ImpersonatorName), v.IP,
			v.TargetType, csvSafe(target), csvSafe(d.Scope), csvSafe(strings.Join(d.Targets, "; ")), ok, csvSafe(d.Error), csvSafe(d.Reason), csvSafe(v.Detail), v.Hash})
	}
	w.Flush()
	h.recordCtx(c, "audit.export", "", "", gin.H{"rows": len(views), "q": c.Query("q")})
}

// csvSafe keeps a spreadsheet from running a cell as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
