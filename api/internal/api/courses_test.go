package api

import (
	"net/http"
	"testing"
	"time"
)

type courseOut struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Tag       string   `json:"tag"`
	OpenTasks int64    `json:"open_tasks"`
	Average   *float64 `json:"average"`
	Target    *float64 `json:"target"`
	Slots     []struct {
		ID      string `json:"id"`
		Weekday int    `json:"weekday"`
		Start   string `json:"start"`
		EventID string `json:"event_id"`
	} `json:"slots"`
}

type calOut struct {
	Events []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Start    string `json:"start"`
		Location string `json:"location"`
		CourseID string `json:"course_id"`
	} `json:"events"`
}

// TestCourses: a course's weekly classes are on the calendar, follow the
// course, can be cancelled one at a time but not edited as events; its
// tagged tasks are counted; deleting it takes its classes away.
func TestCourses(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"School","kind":"school"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "add bob")

	body := `{"name":"Maths","code":"MAT","room":"B2","color":"#007AFF","tz":"Europe/Lisbon",
		"term_start":"2026-09-14","term_end":"2026-12-18",
		"slots":[{"weekday":1,"start":"09:00","end":"10:30"},{"weekday":3,"start":"14:00","end":"15:00","room":"Lab 1","kind":"Lab"}]}`
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/courses", body, bob), http.StatusForbidden, "viewer creates")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/courses", `{"name":"X","slots":[{"weekday":1,"start":"10:00","end":"09:00"}]}`, alice), http.StatusBadRequest, "end before start")
	w := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/courses", body, alice)
	expect(t, w, http.StatusCreated, "create")
	var co courseOut
	decode(t, w, &co)
	if co.Tag != "mat" || len(co.Slots) != 2 || co.Slots[0].EventID == "" {
		t.Fatalf("course: %+v", co)
	}

	// The week of 21 September: Monday's class and Wednesday's lab.
	week := "/api/v1/workspaces/" + ws.ID + "/calendar?from=2026-09-21T00:00:00Z&to=2026-09-28T00:00:00Z"
	var cal calOut
	decode(t, authdo(r, "GET", week, "", bob), &cal)
	if len(cal.Events) != 2 || cal.Events[0].Title != "Maths" || cal.Events[0].Start != "2026-09-21T08:00:00Z" ||
		cal.Events[1].Title != "Maths · Lab" || cal.Events[1].Location != "Lab 1" || cal.Events[0].CourseID != co.ID {
		t.Fatalf("calendar: %+v", cal.Events)
	}
	// After the term: nothing.
	var after calOut
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/calendar?from=2027-01-04T00:00:00Z&to=2027-01-11T00:00:00Z", "", alice), &after)
	if len(after.Events) != 0 {
		t.Fatalf("after the term: %+v", after.Events)
	}

	// A class is the course's: not edited as an event, but one can be cancelled.
	ev := cal.Events[0].ID
	expect(t, authdo(r, "PUT", "/api/v1/events/"+ev, `{"title":"Other"}`, alice), http.StatusConflict, "edit class")
	expect(t, authdo(r, "DELETE", "/api/v1/events/"+ev, "", alice), http.StatusConflict, "delete class series")
	expect(t, authdo(r, "DELETE", "/api/v1/events/"+ev+"?occurrence=2026-09-21T08:00:00Z", "", alice), http.StatusNoContent, "cancel one")
	decode(t, authdo(r, "GET", week, "", alice), &cal)
	if len(cal.Events) != 1 {
		t.Fatalf("after cancelling: %+v", cal.Events)
	}

	// Renaming the course renames its classes; keeping Monday's slot (by id)
	// keeps it cancelled; dropping Wednesday's removes it.
	upd := `{"name":"Mathematics","slots":[{"id":"` + co.Slots[0].ID + `","weekday":1,"start":"09:00","end":"10:30"}]}`
	w = authdo(r, "PUT", "/api/v1/courses/"+co.ID, upd, alice)
	expect(t, w, http.StatusOK, "update")
	decode(t, w, &co)
	if len(co.Slots) != 1 {
		t.Fatalf("slots: %+v", co.Slots)
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/calendar?from=2026-09-21T00:00:00Z&to=2026-10-05T00:00:00Z", "", alice), &cal)
	if len(cal.Events) != 1 || cal.Events[0].Title != "Mathematics" || cal.Events[0].Start != "2026-09-28T08:00:00Z" {
		t.Fatalf("two weeks after the update: %+v", cal.Events)
	}

	// Tasks tagged #mat are the course's.
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Homework"}`, alice), http.StatusCreated, "list")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Exercises","tags":["mat"]}`, alice), http.StatusCreated, "task")
	var list []courseOut
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/courses", "", bob), &list)
	if len(list) != 1 || list[0].OpenTasks != 1 {
		t.Fatalf("courses: %+v", list)
	}

	// Deleting the course takes its classes away.
	expect(t, authdo(r, "DELETE", "/api/v1/courses/"+co.ID, "", alice), http.StatusNoContent, "delete")
	decode(t, authdo(r, "GET", week, "", alice), &cal)
	if len(cal.Events) != 0 {
		t.Fatalf("classes left: %+v", cal.Events)
	}
}

// TestGrades: each member's grades are their own (viewers too); averages
// are weighted and on the workspace's scale; goals follow a scale change.
func TestGrades(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"School","kind":"school"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "add bob")
	var co courseOut
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/courses", `{"name":"Physics"}`, alice), &co)

	// Alice: 12/20 (weight 1) and 80/100 (weight 3) → (0.6 + 2.4) / 4 = 0.75 → 15/20.
	expect(t, authdo(r, "POST", "/api/v1/courses/"+co.ID+"/grades", `{"title":"Test 1","score":12}`, alice), http.StatusCreated, "grade 1")
	w := authdo(r, "POST", "/api/v1/courses/"+co.ID+"/grades", `{"title":"Project","score":80,"out_of":100,"weight":3,"date":"2026-10-10"}`, alice)
	expect(t, w, http.StatusCreated, "grade 2")
	var g struct {
		ID string `json:"id"`
	}
	decode(t, w, &g)
	expect(t, authdo(r, "POST", "/api/v1/courses/"+co.ID+"/grades", `{"title":"Bad","score":50}`, alice), http.StatusBadRequest, "far above the scale")
	// Bob, a viewer, keeps his own.
	expect(t, authdo(r, "POST", "/api/v1/courses/"+co.ID+"/grades", `{"title":"Test 1","score":18}`, bob), http.StatusCreated, "bob's grade")
	expect(t, authdo(r, "PUT", "/api/v1/courses/"+co.ID+"/goal", `{"target":16}`, alice), http.StatusNoContent, "goal")
	expect(t, authdo(r, "PUT", "/api/v1/courses/"+co.ID+"/goal", `{"target":21}`, alice), http.StatusBadRequest, "goal above scale")

	var list []courseOut
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/courses", "", alice), &list)
	if list[0].Average == nil || *list[0].Average != 15 || list[0].Target == nil || *list[0].Target != 16 {
		t.Fatalf("alice's course: %+v", list[0])
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/courses", "", bob), &list)
	if list[0].Average == nil || *list[0].Average != 18 || list[0].Target != nil {
		t.Fatalf("bob's course: %+v", list[0])
	}
	var mine []struct {
		Title string `json:"title"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/grades", "", bob), &mine)
	if len(mine) != 1 {
		t.Fatalf("bob sees %d grades", len(mine))
	}
	// Nobody else's grade can be touched.
	expect(t, authdo(r, "PUT", "/api/v1/grades/"+g.ID, `{"score":20}`, bob), http.StatusNotFound, "bob edits alice's")
	expect(t, authdo(r, "DELETE", "/api/v1/grades/"+g.ID, "", bob), http.StatusNotFound, "bob deletes alice's")
	expect(t, authdo(r, "PUT", "/api/v1/grades/"+g.ID, `{"score":100}`, alice), http.StatusOK, "alice edits")

	// The owner moves to percentages: 1 + 3 → (0.6 + 3) / 4 = 90%; the goal 16/20 is 80%.
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID, `{"grade_scale":7}`, alice), http.StatusBadRequest, "bad scale")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID, `{"grade_scale":100}`, alice), http.StatusOK, "scale")
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/courses", "", alice), &list)
	if *list[0].Average != 90 || *list[0].Target != 80 {
		t.Fatalf("on 100: average %v, target %v", *list[0].Average, *list[0].Target)
	}
}

// TestCoursePageAndExams: a course's notes page is made once, then reused;
// a task marked as an exam is counted down to on the overview (and stops
// being one when marked back).
func TestCoursePageAndExams(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"School","kind":"school"}`)
	var co courseOut
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/courses", `{"name":"History"}`, alice), &co)
	var p1, p2 struct {
		PageID string `json:"page_id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/courses/"+co.ID+"/page", "", alice), &p1)
	decode(t, authdo(r, "POST", "/api/v1/courses/"+co.ID+"/page", "", alice), &p2)
	if p1.PageID == "" || p1.PageID != p2.PageID {
		t.Fatalf("page: %q %q", p1.PageID, p2.PageID)
	}
	var page struct {
		Title string `json:"title"`
	}
	decode(t, authdo(r, "GET", "/api/v1/pages/"+p1.PageID, "", alice), &page)
	if page.Title != "History" {
		t.Fatalf("page title %q", page.Title)
	}

	var task struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	soon := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"History test","kind":"exam","due":"`+soon+`","tags":["history"]}`, alice), &task)
	if task.Kind != "exam" {
		t.Fatalf("kind %q", task.Kind)
	}
	home := func() int {
		var h struct {
			Exams []struct {
				ID string `json:"id"`
			} `json:"exams"`
		}
		decode(t, authdo(r, "GET", "/api/v1/home?today="+today+"&from="+time.Now().Format(time.RFC3339)+"&to="+time.Now().AddDate(0, 0, 7).Format(time.RFC3339), "", alice), &h)
		return len(h.Exams)
	}
	if home() != 1 {
		t.Fatal("exam not on the overview")
	}
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"kind":"task"}`, alice), http.StatusOK, "not an exam")
	if home() != 0 {
		t.Fatal("still an exam")
	}
}
