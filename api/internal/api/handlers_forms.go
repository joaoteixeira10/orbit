package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/feature"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// Forms: a database's way in. A form asks for some of its properties (and
// the row's name); each answer is a new row. The workspace's people answer
// it in Orbit; a public one, by its link, anyone (the owner can turn public
// forms off). Its editors make, change and close forms.

// FormField is one question: a property of the database ("title": the
// row's name), how it's put, and whether it must be answered.
type FormField struct {
	Prop     string `json:"prop"`
	Label    string `json:"label,omitempty"`
	Help     string `json:"help,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type formResponse struct {
	models.Form
	Fields []FormField `json:"fields"`
	URL    string      `json:"url,omitempty"` // a public one's link
}

func (h *Handlers) formOut(c *gin.Context, f models.Form) formResponse {
	out := formResponse{Form: f, Fields: []FormField{}}
	_ = json.Unmarshal([]byte(f.Fields), &out.Fields)
	if f.Public && f.TokenSealed != "" {
		out.URL = h.baseURL(c) + "/form/" + h.unsealLink(f.TokenSealed)
	}
	return out
}

type formRequest struct {
	Title  *string      `json:"title" binding:"omitempty,max=300"`
	Intro  *string      `json:"intro" binding:"omitempty,max=5000"`
	Thanks *string      `json:"thanks" binding:"omitempty,max=500"`
	Fields *[]FormField `json:"fields" binding:"omitempty,max=60"`
	Public *bool        `json:"public"`
	Open   *bool        `json:"open"`
}

// applyForm copies a request's fields, checked against the database: each
// question a property it has (once), formulas left out.
func (h *Handlers) applyForm(f *models.Form, db *planner.Database, req formRequest) (int, string) {
	if req.Title != nil {
		f.Title = strings.TrimSpace(*req.Title)
	}
	if req.Intro != nil {
		f.Intro = *req.Intro
	}
	if req.Thanks != nil {
		f.Thanks = strings.TrimSpace(*req.Thanks)
	}
	if req.Open != nil {
		f.Open = *req.Open
	}
	if req.Public != nil {
		if *req.Public && !feature.Enabled(h.db, feature.PublicForms) {
			return http.StatusForbidden, "public forms are off on this Orbit"
		}
		f.Public = *req.Public
	}
	if req.Fields != nil {
		seen := map[string]bool{}
		var out []FormField
		for _, x := range *req.Fields {
			if seen[x.Prop] {
				continue
			}
			if x.Prop != "title" {
				p := propOf(db, x.Prop)
				if p == nil || p.Type == planner.PropFormula {
					return http.StatusBadRequest, "a question isn't a property of the database: " + x.Prop
				}
			}
			seen[x.Prop] = true
			x.Label, x.Help = clip(strings.TrimSpace(x.Label), 200), clip(strings.TrimSpace(x.Help), 500)
			out = append(out, x)
		}
		b, _ := json.Marshal(out)
		f.Fields = string(b)
	}
	return 0, ""
}

func propOf(db *planner.Database, id string) *planner.DBProp {
	if db == nil {
		return nil
	}
	for i := range db.Props {
		if db.Props[i].ID == id {
			return &db.Props[i]
		}
	}
	return nil
}

// listForms lists a database's forms (its members).
//
//	@Summary	A database's forms
//	@Tags		forms
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Database page id"
//	@Success	200	{array}	formResponse
//	@Router		/pages/{id}/forms [get]
func (h *Handlers) listForms(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSViewer)
	if !ok {
		return
	}
	var fs []models.Form
	h.db.Where("page_id = ?", p.ID).Order("created_at").Find(&fs)
	out := make([]formResponse, 0, len(fs))
	for _, f := range fs {
		out = append(out, h.formOut(c, f))
	}
	c.JSON(http.StatusOK, out)
}

// createForm makes a form for a database (editors).
//
//	@Summary	New form
//	@Tags		forms
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"Database page id"
//	@Param		body	body		formRequest	true	"Its questions"
//	@Success	201		{object}	formResponse
//	@Router		/pages/{id}/forms [post]
func (h *Handlers) createForm(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	db := planner.ParseDatabase(p.DB)
	if db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a form fills a database: turn the page into one first"})
		return
	}
	var req formRequest
	if !bindJSON(c, &req) {
		return
	}
	f := models.Form{WorkspaceID: p.WorkspaceID, PageID: p.ID, Title: p.Title, Open: true, Fields: `[{"prop":"title","required":true}]`, CreatedBy: auth.UserID(c)}
	if code, msg := h.applyForm(&f, db, req); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	_, hash, sealed, err := h.newLinkSecret()
	if err != nil {
		h.internalError(c, err, "could not make the form")
		return
	}
	f.TokenHash, f.TokenSealed = hash, sealed
	if err := h.db.Create(&f).Error; err != nil {
		h.internalError(c, err, "could not make the form")
		return
	}
	h.publishWS(c, p.WorkspaceID, "forms")
	c.JSON(http.StatusCreated, h.formOut(c, f))
}

func (h *Handlers) formAccess(c *gin.Context, need string) (models.Form, models.Page, bool) {
	var f models.Form
	var p models.Page
	if h.db.First(&f, "id = ?", c.Param("id")).Error != nil || h.db.First(&p, "id = ? AND trashed_at IS NULL", f.PageID).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "form not found"})
		return f, p, false
	}
	_, _, ok := h.wsAccess(c, f.WorkspaceID, need)
	if ok && !h.pageVisible(c, p) {
		c.JSON(http.StatusNotFound, gin.H{"error": "form not found"})
		return f, p, false
	}
	return f, p, ok
}

// updateForm changes a form (editors).
//
//	@Summary	Change a form
//	@Tags		forms
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"Form id"
//	@Param		body	body		formRequest	true	"What changes"
//	@Success	200		{object}	formResponse
//	@Router		/forms/{id} [put]
func (h *Handlers) updateForm(c *gin.Context) {
	f, p, ok := h.formAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req formRequest
	if !bindJSON(c, &req) {
		return
	}
	if code, msg := h.applyForm(&f, planner.ParseDatabase(p.DB), req); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	if err := h.db.Model(&models.Form{}).Where("id = ?", f.ID).Updates(map[string]any{"title": f.Title, "intro": f.Intro, "thanks": f.Thanks, "fields": f.Fields, "public": f.Public, "open": f.Open}).Error; err != nil {
		h.internalError(c, err, "could not save the form")
		return
	}
	h.publishWS(c, f.WorkspaceID, "forms")
	c.JSON(http.StatusOK, h.formOut(c, f))
}

// deleteForm deletes a form (editors); the rows it made stay.
//
//	@Summary	Delete a form
//	@Tags		forms
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Form id"
//	@Success	204
//	@Router		/forms/{id} [delete]
func (h *Handlers) deleteForm(c *gin.Context) {
	f, _, ok := h.formAccess(c, models.WSEditor)
	if !ok {
		return
	}
	h.db.Delete(&f)
	h.publishWS(c, f.WorkspaceID, "forms")
	c.Status(http.StatusNoContent)
}

// ---- Filling one in ----

type fillOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

type fillField struct {
	Prop     string       `json:"prop"`
	Label    string       `json:"label"`
	Help     string       `json:"help,omitempty"`
	Required bool         `json:"required,omitempty"`
	Type     string       `json:"type"` // title | text | number | select | multi_select | date | checkbox | person | url
	Options  []fillOption `json:"options,omitempty"`
}

type fillForm struct {
	Title  string      `json:"title"`
	Intro  string      `json:"intro,omitempty"`
	Thanks string      `json:"thanks,omitempty"`
	Open   bool        `json:"open"`
	Fields []fillField `json:"fields"`
}

// fillable is what someone answering sees; public: without the questions
// that would show the workspace's people.
func (h *Handlers) fillable(f models.Form, p models.Page, public bool) fillForm {
	db := planner.ParseDatabase(p.DB)
	var qs []FormField
	_ = json.Unmarshal([]byte(f.Fields), &qs)
	out := fillForm{Title: f.Title, Intro: f.Intro, Thanks: f.Thanks, Open: f.Open, Fields: []fillField{}}
	if out.Title == "" {
		out.Title = p.Title
	}
	for _, q := range qs {
		x := fillField{Prop: q.Prop, Label: q.Label, Help: q.Help, Required: q.Required}
		if q.Prop == "title" {
			x.Type = "title"
			if x.Label == "" {
				x.Label = "Name"
			}
		} else {
			prop := propOf(db, q.Prop)
			if prop == nil || prop.Type == planner.PropFormula || (public && prop.Type == planner.PropPerson) {
				continue // gone from the database since
			}
			x.Type = prop.Type
			if x.Label == "" {
				x.Label = prop.Name
			}
			for _, o := range prop.Options {
				x.Options = append(x.Options, fillOption{ID: o.ID, Name: o.Name, Color: o.Color})
			}
			if prop.Type == planner.PropPerson && !public {
				for _, m := range h.wsMembers(f.WorkspaceID) {
					x.Options = append(x.Options, fillOption{ID: m.id, Name: m.name})
				}
			}
		}
		out.Fields = append(out.Fields, x)
	}
	return out
}

type wsPerson struct{ id, name string }

func (h *Handlers) wsMembers(wsID string) []wsPerson {
	type row struct{ ID, Username, DisplayName string }
	var rows []row
	h.db.Table("workspace_members").Select("users.id, users.username, users.display_name").
		Joins("JOIN users ON users.id = workspace_members.user_id").Where("workspace_members.workspace_id = ?", wsID).Order("users.username").Scan(&rows)
	out := make([]wsPerson, 0, len(rows))
	for _, r := range rows {
		n := r.DisplayName
		if n == "" {
			n = r.Username
		}
		out = append(out, wsPerson{r.ID, n})
	}
	return out
}

type answerRequest struct {
	Values map[string]any `json:"values"`
}

// answer checks the values against the questions and makes the row
// (createdBy: who answered; "" someone with the link).
func (h *Handlers) answer(c *gin.Context, f models.Form, p models.Page, public bool, createdBy string) {
	if !f.Open {
		c.JSON(http.StatusConflict, gin.H{"error": "this form isn't taking answers"})
		return
	}
	var req answerRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Values == nil {
		req.Values = map[string]any{}
	}
	// A field people don't see, that only robots fill in.
	if hp, _ := req.Values["_hp"].(string); hp != "" {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	title := ""
	props := map[string]any{}
	for _, q := range h.fillable(f, p, public).Fields {
		v, given := req.Values[q.Prop]
		empty := !given || v == nil || v == "" || v == false
		if arr, ok := v.([]any); ok && len(arr) == 0 {
			empty = true
		}
		if empty {
			if q.Required && (q.Type != "checkbox" || !given) {
				c.JSON(http.StatusBadRequest, gin.H{"error": q.Label + " is needed", "field": q.Prop})
				return
			}
			continue
		}
		clean, msg := checkAnswer(q, v)
		if msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": q.Label + ": " + msg, "field": q.Prop})
			return
		}
		if q.Type == "title" {
			title = clean.(string)
		} else {
			props[q.Prop] = clean
		}
	}
	b, _ := json.Marshal(props)
	merged, err := planner.MergeProps("", string(b))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	parent := p.ID
	row := models.Page{WorkspaceID: p.WorkspaceID, ParentID: &parent, Title: title, Props: merged, CreatedBy: createdBy, UpdatedBy: createdBy, Version: 1}
	row.Rank = h.siblingRank(h.db, p.WorkspaceID, &parent, "", "")
	if err := h.db.Create(&row).Error; err != nil {
		h.internalError(c, err, "could not save the answer")
		return
	}
	h.db.Model(&models.Form{}).Where("id = ?", f.ID).UpdateColumn("answers", f.Answers+1)
	h.publishWS(c, p.WorkspaceID, "pages")
	c.JSON(http.StatusCreated, gin.H{"ok": true, "thanks": f.Thanks})
}

// checkAnswer reads one answer as its property keeps it.
func checkAnswer(q fillField, v any) (any, string) {
	str := func() (string, bool) {
		s, ok := v.(string)
		return strings.TrimSpace(s), ok
	}
	option := func(id string) bool {
		for _, o := range q.Options {
			if o.ID == id {
				return true
			}
		}
		return false
	}
	switch q.Type {
	case "title", planner.PropText, planner.PropURL:
		s, ok := str()
		limit := 2000 // a property's text, at most
		if q.Type == "title" {
			limit = 300
		}
		if !ok || len(s) > limit {
			return nil, fmt.Sprintf("a text of at most %d characters", limit)
		}
		if q.Type == planner.PropURL && !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			return nil, "a web address (https://…)"
		}
		return s, ""
	case planner.PropNumber:
		switch x := v.(type) {
		case float64:
			return x, ""
		case string:
			n, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(x), ",", "."), 64)
			if err == nil {
				return n, ""
			}
		}
		return nil, "a number"
	case planner.PropCheckbox:
		b, ok := v.(bool)
		if !ok {
			return nil, "yes or no"
		}
		return b, ""
	case planner.PropDate:
		s, ok := str()
		if !ok || !planner.ValidDate(s) {
			return nil, "a date (YYYY-MM-DD)"
		}
		return s, ""
	case planner.PropSelect, planner.PropPerson:
		s, ok := str()
		if !ok || !option(s) {
			return nil, "one of the choices"
		}
		return s, ""
	case planner.PropMulti:
		arr, ok := v.([]any)
		if !ok || len(arr) > 50 {
			return nil, "some of the choices"
		}
		out := []string{}
		for _, x := range arr {
			s, ok := x.(string)
			if !ok || !option(s) {
				return nil, "some of the choices"
			}
			out = append(out, s)
		}
		return out, ""
	}
	return nil, "can't be answered here"
}

// getFormToFill is a form as the workspace's people answer it.
//
//	@Summary	A form, to answer
//	@Tags		forms
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Form id"
//	@Success	200	{object}	fillForm
//	@Router		/forms/{id} [get]
func (h *Handlers) getFormToFill(c *gin.Context) {
	f, p, ok := h.formAccess(c, models.WSViewer)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.fillable(f, p, false))
}

// answerForm is one of the workspace's people answering (any role: viewers
// too — a form is how they add a row).
//
//	@Summary	Answer a form
//	@Tags		forms
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Form id"
//	@Param		body	body		answerRequest	true	"The answers, by property"
//	@Success	201		{object}	map[string]any
//	@Router		/forms/{id}/answers [post]
func (h *Handlers) answerForm(c *gin.Context) {
	f, p, ok := h.formAccess(c, models.WSViewer)
	if !ok {
		return
	}
	h.answer(c, f, p, false, auth.UserID(c))
}

// publicForm finds a public form by its link (off, closed to the public or
// gone: not found).
func (h *Handlers) publicForm(c *gin.Context) (models.Form, models.Page, bool) {
	c.Header("X-Robots-Tag", "noindex, nofollow")
	var f models.Form
	var p models.Page
	if !feature.Enabled(h.db, feature.PublicForms) || h.db.First(&f, "token_hash = ? AND public = ?", linkHash(c.Param("token")), true).Error != nil ||
		h.db.First(&p, "id = ? AND trashed_at IS NULL", f.PageID).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "this form isn't here (taken down, or the link is mistyped)"})
		return f, p, false
	}
	return f, p, true
}

// publicFormOf is a public form, to answer (no account).
//
//	@Summary	A public form
//	@Tags		forms
//	@Produce	json
//	@Param		token	path		string	true	"The form's link"
//	@Success	200		{object}	fillForm
//	@Router		/f/{token} [get]
func (h *Handlers) publicFormOf(c *gin.Context) {
	f, p, ok := h.publicForm(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.fillable(f, p, true))
}

// answerPublicForm is someone with the link answering.
//
//	@Summary	Answer a public form
//	@Tags		forms
//	@Accept		json
//	@Produce	json
//	@Param		token	path		string			true	"The form's link"
//	@Param		body	body		answerRequest	true	"The answers, by property"
//	@Success	201		{object}	map[string]any
//	@Router		/f/{token} [post]
func (h *Handlers) answerPublicForm(c *gin.Context) {
	f, p, ok := h.publicForm(c)
	if !ok {
		return
	}
	h.answer(c, f, p, true, "")
}
