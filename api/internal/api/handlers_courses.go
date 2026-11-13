package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// School: courses (the Courses module) and grades (the Grades module).
// A course and its weekly timetable belong to the workspace — editors change
// them, and each weekly class is a repeating event the course keeps on the
// calendar. Grades and goals are personal: each member sees and changes only
// their own, viewers too.

type courseResponse struct {
	models.Course
	Slots      []models.CourseSlot `json:"slots"`
	OpenTasks  int64               `json:"open_tasks"` // not done, tagged #tag
	Average    *float64            `json:"average"`    // the caller's, on the workspace's scale
	GradeCount int                 `json:"grade_count"`
	Target     *float64            `json:"target"` // the caller's goal, on the scale
}

type slotRequest struct {
	ID      string `json:"id"` // an existing class, kept (with its cancelled dates)
	Weekday int    `json:"weekday" binding:"min=1,max=7"`
	Start   string `json:"start" binding:"required"`
	End     string `json:"end" binding:"required"`
	Room    string `json:"room" binding:"max=60"`
	Kind    string `json:"kind" binding:"max=40"`
}

type courseRequest struct {
	Name      *string        `json:"name" binding:"omitempty,max=120"`
	Code      *string        `json:"code" binding:"omitempty,max=16"`
	Teacher   *string        `json:"teacher" binding:"omitempty,max=120"`
	Room      *string        `json:"room" binding:"omitempty,max=60"`
	Color     *string        `json:"color"`
	Tag       *string        `json:"tag" binding:"omitempty,max=40"`
	TermStart *string        `json:"term_start"` // "" clears
	TermEnd   *string        `json:"term_end"`
	TZ        *string        `json:"tz"`
	Weight    *float64       `json:"weight" binding:"omitempty,min=0,max=100"`
	Notes     *string        `json:"notes" binding:"omitempty,max=20000"`
	Slots     *[]slotRequest `json:"slots" binding:"omitempty,max=40,dive"`
}

// applyCourse validates req onto co; it returns a message for a 400.
func applyCourse(req courseRequest, co *models.Course) string {
	trim := func(p *string, dst *string) {
		if p != nil {
			*dst = strings.TrimSpace(*p)
		}
	}
	trim(req.Name, &co.Name)
	trim(req.Code, &co.Code)
	trim(req.Teacher, &co.Teacher)
	trim(req.Room, &co.Room)
	if req.Notes != nil {
		co.Notes = *req.Notes
	}
	if co.Name == "" {
		return "give the course a name"
	}
	if req.Color != nil {
		if *req.Color != "" && !planner.ValidColor(*req.Color) {
			return "color must be #rrggbb"
		}
		co.Color = *req.Color
	}
	if req.Tag != nil {
		co.Tag = planner.CourseTag(*req.Tag)
	}
	if co.Tag == "" {
		co.Tag = planner.CourseTag(co.Code)
		if co.Tag == "" {
			co.Tag = planner.CourseTag(co.Name)
		}
	}
	date := func(p *string, dst **string) string {
		if p == nil {
			return ""
		}
		if *p == "" {
			*dst = nil
			return ""
		}
		if !planner.ValidDate(*p) {
			return "term dates are YYYY-MM-DD"
		}
		v := *p
		*dst = &v
		return ""
	}
	if msg := date(req.TermStart, &co.TermStart); msg != "" {
		return msg
	}
	if msg := date(req.TermEnd, &co.TermEnd); msg != "" {
		return msg
	}
	if co.TermStart != nil && co.TermEnd != nil && *co.TermEnd < *co.TermStart {
		return "the term can't end before it starts"
	}
	if req.TZ != nil {
		if _, err := time.LoadLocation(*req.TZ); err != nil || *req.TZ == "Local" || *req.TZ == "" {
			return "tz must be an IANA time zone, like Europe/Lisbon"
		}
		co.TZ = *req.TZ
	}
	if co.TZ == "" {
		co.TZ = "UTC"
	}
	if req.Weight != nil {
		co.Weight = *req.Weight
	}
	return ""
}

// syncClasses makes the course's classes what slots says (when given) and
// keeps every class's repeating event in step with the course.
func syncClasses(tx *gorm.DB, co models.Course, slots *[]slotRequest, uid string) error {
	var have []models.CourseSlot
	tx.Where("course_id = ?", co.ID).Find(&have)
	if slots != nil {
		byID := map[string]models.CourseSlot{}
		for _, s := range have {
			byID[s.ID] = s
		}
		var next []models.CourseSlot
		for _, r := range *slots {
			s, ok := byID[r.ID]
			if ok {
				delete(byID, r.ID)
			} else {
				s = models.CourseSlot{CourseID: co.ID, WorkspaceID: co.WorkspaceID}
			}
			s.Weekday, s.Start, s.End = r.Weekday, r.Start, r.End
			s.Room, s.Kind = strings.TrimSpace(r.Room), strings.TrimSpace(r.Kind)
			if !planner.ValidClock(s.Start) || !planner.ValidClock(s.End) || s.End <= s.Start {
				return badRequest("each class needs a start and a later end (HH:MM)")
			}
			next = append(next, s)
		}
		for _, gone := range byID {
			if gone.EventID != "" {
				if err := tx.Where("id = ?", gone.EventID).Delete(&models.Event{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Delete(&gone).Error; err != nil {
				return err
			}
		}
		have = next
	}
	for i := range have {
		s := &have[i]
		if s.ID == "" {
			if err := tx.Create(s).Error; err != nil {
				return err
			}
		}
		var ev models.Event
		found := s.EventID != "" && tx.First(&ev, "id = ?", s.EventID).Error == nil
		if !found {
			ev = models.Event{CreatedBy: uid}
		}
		since := s.CreatedAt
		if since.IsZero() {
			since = time.Now()
		}
		if err := planner.ClassEvent(co, *s, since, &ev); err != nil {
			return badRequest(err.Error())
		}
		if err := tx.Save(&ev).Error; err != nil {
			return err
		}
		if s.EventID != ev.ID {
			s.EventID = ev.ID
		}
		if err := tx.Save(s).Error; err != nil {
			return err
		}
	}
	return nil
}

// onScale turns a fraction into the workspace's scale, to two decimals.
func onScale(f float64, scale int) float64 {
	return math.Round(f*float64(scale)*100) / 100
}

func (h *Handlers) courseResponses(c *gin.Context, ws models.Workspace, courses []models.Course) []courseResponse {
	uid := auth.UserID(c)
	ids := make([]string, 0, len(courses))
	for _, co := range courses {
		ids = append(ids, co.ID)
	}
	slots := map[string][]models.CourseSlot{}
	grades := map[string][]models.Grade{}
	goals := map[string]float64{}
	if len(ids) > 0 {
		var ss []models.CourseSlot
		h.db.Where("course_id IN ?", ids).Order("weekday, start").Find(&ss)
		for _, s := range ss {
			slots[s.CourseID] = append(slots[s.CourseID], s)
		}
		var gs []models.Grade
		h.db.Where("course_id IN ? AND owner_id = ?", ids, uid).Find(&gs)
		for _, g := range gs {
			grades[g.CourseID] = append(grades[g.CourseID], g)
		}
		var gl []models.CourseGoal
		h.db.Where("course_id IN ? AND user_id = ?", ids, uid).Find(&gl)
		for _, g := range gl {
			goals[g.CourseID] = g.Target
		}
	}
	out := make([]courseResponse, 0, len(courses))
	for _, co := range courses {
		r := courseResponse{Course: co, Slots: slots[co.ID], GradeCount: len(grades[co.ID])}
		if r.Slots == nil {
			r.Slots = []models.CourseSlot{}
		}
		if avg, ok := planner.Average(grades[co.ID]); ok {
			v := onScale(avg, ws.GradeScale)
			r.Average = &v
		}
		if t, ok := goals[co.ID]; ok {
			v := t
			r.Target = &v
		}
		if co.Tag != "" {
			h.db.Model(&models.Task{}).Where("workspace_id = ? AND completed_at IS NULL AND tags LIKE ? AND "+models.InOpenList, ws.ID, `%"`+co.Tag+`"%`).Count(&r.OpenTasks)
		}
		out = append(out, r)
	}
	return out
}

// listCourses lists a workspace's courses with their classes, and the
// caller's average and goal in each.
//
//	@Summary	Courses of a workspace
//	@Tags		school
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	courseResponse
//	@Router		/workspaces/{ws}/courses [get]
func (h *Handlers) listCourses(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var courses []models.Course
	h.db.Where("workspace_id = ?", ws.ID).Order("rank, created_at").Find(&courses)
	c.JSON(http.StatusOK, h.courseResponses(c, ws, courses))
}

// createCourse adds a course, with its weekly classes (editors).
//
//	@Summary	Create a course
//	@Tags		school
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		courseRequest	true	"Course"
//	@Success	201		{object}	courseResponse
//	@Router		/workspaces/{ws}/courses [post]
func (h *Handlers) createCourse(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req courseRequest
	if !bindJSON(c, &req) {
		return
	}
	co := models.Course{WorkspaceID: ws.ID, CreatedBy: auth.UserID(c), Weight: 1}
	if msg := applyCourse(req, &co); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	var last struct{ Max *float64 }
	h.db.Model(&models.Course{}).Select("MAX(rank) AS max").Where("workspace_id = ?", ws.ID).Scan(&last)
	co.Rank = planner.Rank(last.Max, nil)
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&co).Error; err != nil {
			return err
		}
		return syncClasses(tx, co, req.Slots, auth.UserID(c))
	})
	if writeTxError(c, h, err, "could not create the course") {
		return
	}
	h.publishWS(c, ws.ID, "courses")
	h.publishWS(c, ws.ID, "events")
	c.JSON(http.StatusCreated, h.courseResponses(c, ws, []models.Course{co})[0])
}

// courseAccess loads :id and checks the caller's role in its workspace.
func (h *Handlers) courseAccess(c *gin.Context, need string) (models.Course, models.Workspace, bool) {
	var co models.Course
	if h.db.First(&co, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
		return co, models.Workspace{}, false
	}
	ws, _, ok := h.wsAccess(c, co.WorkspaceID, need)
	return co, ws, ok
}

// updateCourse changes a course; slots, when given, is its whole timetable
// (editors).
//
//	@Summary	Update a course
//	@Tags		school
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Course id"
//	@Param		body	body		courseRequest	true	"Fields to change"
//	@Success	200		{object}	courseResponse
//	@Router		/courses/{id} [put]
func (h *Handlers) updateCourse(c *gin.Context) {
	co, ws, ok := h.courseAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req courseRequest
	if !bindJSON(c, &req) {
		return
	}
	if msg := applyCourse(req, &co); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&co).Error; err != nil {
			return err
		}
		return syncClasses(tx, co, req.Slots, auth.UserID(c))
	})
	if writeTxError(c, h, err, "could not update the course") {
		return
	}
	h.publishWS(c, ws.ID, "courses")
	h.publishWS(c, ws.ID, "events")
	c.JSON(http.StatusOK, h.courseResponses(c, ws, []models.Course{co})[0])
}

// deleteCourse deletes a course, its classes on the calendar, and everyone's
// grades in it; its tasks stay (editors).
//
//	@Summary	Delete a course
//	@Tags		school
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Course id"
//	@Success	204
//	@Router		/courses/{id} [delete]
func (h *Handlers) deleteCourse(c *gin.Context) {
	co, ws, ok := h.courseAccess(c, models.WSEditor)
	if !ok {
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error { return deleteCourses(tx, "id = ?", co.ID) })
	if writeTxError(c, h, err, "could not delete the course") {
		return
	}
	h.recordCtx(c, "course.delete", "workspace", ws.ID, gin.H{"course": co.Name})
	h.publishWS(c, ws.ID, "courses")
	h.publishWS(c, ws.ID, "events")
	c.Status(http.StatusNoContent)
}

// deleteCourses deletes the courses matching where, with their classes,
// events, grades and goals.
func deleteCourses(tx *gorm.DB, where string, args ...any) error {
	var ids []string
	tx.Model(&models.Course{}).Where(where, args...).Pluck("id", &ids)
	if len(ids) == 0 {
		return nil
	}
	for _, step := range []func() error{
		func() error { return tx.Where("course_id IN ?", ids).Delete(&models.Event{}).Error },
		func() error { return tx.Where("course_id IN ?", ids).Delete(&models.CourseSlot{}).Error },
		func() error { return tx.Where("course_id IN ?", ids).Delete(&models.Grade{}).Error },
		func() error { return tx.Where("course_id IN ?", ids).Delete(&models.CourseGoal{}).Error },
		func() error { return tx.Where("id IN ?", ids).Delete(&models.Course{}).Error },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// ---- Grades -------------------------------------------------------------------

type gradeRequest struct {
	Title  *string  `json:"title" binding:"omitempty,max=200"`
	Score  *float64 `json:"score" binding:"omitempty,min=0"`
	OutOf  *float64 `json:"out_of" binding:"omitempty,gt=0,max=10000"`
	Weight *float64 `json:"weight" binding:"omitempty,min=0,max=1000"`
	Date   *string  `json:"date"` // "" clears
}

func applyGrade(req gradeRequest, g *models.Grade) string {
	if req.Title != nil {
		g.Title = strings.TrimSpace(*req.Title)
	}
	if g.Title == "" {
		return "give the grade a name (Test 1, Exam…)"
	}
	if req.Score != nil {
		g.Score = *req.Score
	}
	if req.OutOf != nil {
		g.OutOf = *req.OutOf
	}
	if req.Weight != nil {
		g.Weight = *req.Weight
	}
	if g.OutOf <= 0 {
		return "out_of must be more than 0"
	}
	if g.Score > g.OutOf*2 {
		return "the score is far above what it's out of"
	}
	if req.Date != nil {
		if *req.Date == "" {
			g.Date = nil
		} else if planner.ValidDate(*req.Date) {
			d := *req.Date
			g.Date = &d
		} else {
			return "date must be YYYY-MM-DD"
		}
	}
	return ""
}

// selfAccess: the caller may change their own things in a workspace (any
// member, viewers too — not through a read-only token, nor while archived).
func (h *Handlers) selfAccess(c *gin.Context, wsID string) (models.Workspace, bool) {
	ws, _, ok := h.wsAccess(c, wsID, models.WSViewer)
	if !ok {
		return ws, false
	}
	if auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return ws, false
	}
	if ws.ArchivedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the workspace is archived; restore it to make changes"})
		return ws, false
	}
	return ws, true
}

// publishSelf tells only the caller's sessions that part of a workspace
// changed (their grades are theirs alone).
func (h *Handlers) publishSelf(c *gin.Context, wsID, what string) {
	b, _ := json.Marshal(gin.H{"type": "ws", "ws": wsID, "what": what, "origin": c.GetHeader("X-Orbit-Client")})
	h.events.Publish(auth.UserID(c), string(b))
}

// listGrades lists the caller's grades in a workspace (by date).
//
//	@Summary	Your grades in a workspace
//	@Tags		school
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	models.Grade
//	@Router		/workspaces/{ws}/grades [get]
func (h *Handlers) listGrades(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var out []models.Grade
	h.db.Where("workspace_id = ? AND owner_id = ?", ws.ID, auth.UserID(c)).Order("date IS NULL, date, created_at").Find(&out)
	if out == nil {
		out = []models.Grade{}
	}
	c.JSON(http.StatusOK, out)
}

// createGrade adds one of the caller's grades to a course (any member).
//
//	@Summary	Add a grade
//	@Tags		school
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Course id"
//	@Param		body	body		gradeRequest	true	"Grade"
//	@Success	201		{object}	models.Grade
//	@Router		/courses/{id}/grades [post]
func (h *Handlers) createGrade(c *gin.Context) {
	var co models.Course
	if h.db.First(&co, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
		return
	}
	ws, ok := h.selfAccess(c, co.WorkspaceID)
	if !ok {
		return
	}
	var req gradeRequest
	if !bindJSON(c, &req) {
		return
	}
	g := models.Grade{WorkspaceID: ws.ID, CourseID: co.ID, OwnerID: auth.UserID(c), OutOf: float64(ws.GradeScale), Weight: 1}
	if msg := applyGrade(req, &g); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if err := h.db.Create(&g).Error; err != nil {
		h.internalError(c, err, "could not add the grade")
		return
	}
	h.publishSelf(c, ws.ID, "grades")
	c.JSON(http.StatusCreated, g)
}

// gradeAccess loads :id, the caller's own grade.
func (h *Handlers) gradeAccess(c *gin.Context) (models.Grade, bool) {
	var g models.Grade
	if h.db.First(&g, "id = ? AND owner_id = ?", c.Param("id"), auth.UserID(c)).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "grade not found"})
		return g, false
	}
	_, ok := h.selfAccess(c, g.WorkspaceID)
	return g, ok
}

// updateGrade changes one of the caller's grades.
//
//	@Summary	Update a grade
//	@Tags		school
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Grade id"
//	@Param		body	body		gradeRequest	true	"Fields to change"
//	@Success	200		{object}	models.Grade
//	@Router		/grades/{id} [put]
func (h *Handlers) updateGrade(c *gin.Context) {
	g, ok := h.gradeAccess(c)
	if !ok {
		return
	}
	var req gradeRequest
	if !bindJSON(c, &req) {
		return
	}
	if msg := applyGrade(req, &g); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if err := h.db.Save(&g).Error; err != nil {
		h.internalError(c, err, "could not update the grade")
		return
	}
	h.publishSelf(c, g.WorkspaceID, "grades")
	c.JSON(http.StatusOK, g)
}

// deleteGrade deletes one of the caller's grades.
//
//	@Summary	Delete a grade
//	@Tags		school
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Grade id"
//	@Success	204
//	@Router		/grades/{id} [delete]
func (h *Handlers) deleteGrade(c *gin.Context) {
	g, ok := h.gradeAccess(c)
	if !ok {
		return
	}
	h.db.Delete(&g)
	h.publishSelf(c, g.WorkspaceID, "grades")
	c.Status(http.StatusNoContent)
}

type goalRequest struct {
	Target *float64 `json:"target" binding:"omitempty,min=0"` // null clears
}

// setGoal sets (or with null clears) the average the caller aims at in a
// course, on the workspace's scale.
//
//	@Summary	Set your goal in a course
//	@Tags		school
//	@Accept		json
//	@Security	BearerAuth
//	@Param		id		path	string		true	"Course id"
//	@Param		body	body	goalRequest	true	"Target"
//	@Success	204
//	@Router		/courses/{id}/goal [put]
func (h *Handlers) setGoal(c *gin.Context) {
	var co models.Course
	if h.db.First(&co, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
		return
	}
	ws, ok := h.selfAccess(c, co.WorkspaceID)
	if !ok {
		return
	}
	var req goalRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	if req.Target == nil {
		h.db.Where("course_id = ? AND user_id = ?", co.ID, uid).Delete(&models.CourseGoal{})
	} else {
		if *req.Target > float64(ws.GradeScale) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the goal is above the scale"})
			return
		}
		h.db.Save(&models.CourseGoal{CourseID: co.ID, UserID: uid, Target: *req.Target})
	}
	h.publishSelf(c, ws.ID, "courses")
	c.Status(http.StatusNoContent)
}

// coursePage opens a course's notes page: the one it has, or a new page
// named after it (editors).
//
//	@Summary	A course's notes page
//	@Tags		school
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Course id"
//	@Success	200	{object}	map[string]string
//	@Router		/courses/{id}/page [post]
func (h *Handlers) coursePage(c *gin.Context) {
	co, ws, ok := h.courseAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if co.PageID != nil {
		var p models.Page
		if h.db.Select("id", "trashed_at").First(&p, "id = ?", *co.PageID).Error == nil && p.TrashedAt == nil {
			c.JSON(http.StatusOK, gin.H{"page_id": p.ID})
			return
		}
	}
	uid := auth.UserID(c)
	content := planner.PageMarkdown("")
	p := models.Page{WorkspaceID: ws.ID, Title: co.Name, Icon: "📘", Content: content, ContentText: planner.DocText(content),
		Rank: h.siblingRank(h.db, ws.ID, nil, "", ""), CreatedBy: uid, UpdatedBy: uid}
	if err := h.db.Create(&p).Error; err != nil {
		h.internalError(c, err, "could not create the page")
		return
	}
	h.db.Model(&co).Update("page_id", p.ID)
	h.publishWS(c, ws.ID, "pages")
	h.publishWS(c, ws.ID, "courses")
	h.notifyActivity(c, ws, "page_added", p.Title, pageLink(p))
	c.JSON(http.StatusOK, gin.H{"page_id": p.ID})
}
