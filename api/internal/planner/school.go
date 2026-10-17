package planner

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// Module keys of the school modules.
const (
	ModCourses = "courses" // subjects, their teachers, rooms and weekly timetable
	ModGrades  = "grades"  // marks per course, weighted averages
)

// GradeScales are what marks can be out of in a workspace.
var GradeScales = []int{20, 100, 10, 5}

// ValidScale reports whether n is one of GradeScales.
func ValidScale(n int) bool {
	for _, s := range GradeScales {
		if s == n {
			return true
		}
	}
	return false
}

// Average is the weighted average of grades as a fraction (0..1): each mark
// as score / out of, weighed by its weight. ok is false with no weight.
func Average(grades []models.Grade) (avg float64, ok bool) {
	var sum, w float64
	for _, g := range grades {
		if g.OutOf <= 0 || g.Weight <= 0 {
			continue
		}
		sum += g.Score / g.OutOf * g.Weight
		w += g.Weight
	}
	if w == 0 {
		return 0, false
	}
	return sum / w, true
}

// Needed is what the next mark (of weight w) must be, as a fraction of its
// maximum, for the average to reach target (a fraction): with the marks so
// far scoring S over a total weight W, (target·(W+w) − S) / w. Over 1 is out
// of reach; 0 or less means any mark will do.
func Needed(grades []models.Grade, target, w float64) float64 {
	if w <= 0 {
		return 0
	}
	var sum, total float64
	for _, g := range grades {
		if g.OutOf <= 0 || g.Weight <= 0 {
			continue
		}
		sum += g.Score / g.OutOf * g.Weight
		total += g.Weight
	}
	return (target*(total+w) - sum) / w
}

var tagRe = regexp.MustCompile(`[^\p{L}\p{N}_-]+`)

// CourseTag makes a task tag from a course's name or code: lower case,
// letters, digits, dashes ("Físico-Química" → "físico-química").
func CourseTag(s string) string {
	t := strings.Trim(tagRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-"), "-")
	if r := []rune(t); len(r) > 40 {
		t = string(r[:40])
	}
	return t
}

// ClassEvent fills ev — the repeating event of one weekly class — from its
// course and slot: every week on the slot's weekday, from the course's term
// start (or since, when there's none) until its term end (or for good).
func ClassEvent(c models.Course, s models.CourseSlot, since time.Time, ev *models.Event) error {
	if s.Weekday < 1 || s.Weekday > 7 || !ValidClock(s.Start) || !ValidClock(s.End) || s.End <= s.Start {
		return errors.New("a class needs a weekday and a start before its end")
	}
	loc := location(c.TZ)
	anchor := since.In(loc)
	if c.TermStart != nil && ValidDate(*c.TermStart) {
		d, _ := time.ParseInLocation(DateLayout, *c.TermStart, loc)
		anchor = d
	}
	day := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, loc)
	want := time.Weekday(s.Weekday % 7) // 7 = Sunday = 0
	for day.Weekday() != want {
		day = day.AddDate(0, 0, 1)
	}
	at := func(hhmm string) time.Time {
		t, _ := time.Parse("15:04", hhmm)
		return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	}
	rule := "FREQ=WEEKLY"
	if c.TermEnd != nil && ValidDate(*c.TermEnd) {
		end, _ := time.ParseInLocation(DateLayout, *c.TermEnd, loc)
		last := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, loc)
		rule += ";UNTIL=" + last.UTC().Format("20060102T150405Z")
	}
	title := c.Name
	if k := strings.TrimSpace(s.Kind); k != "" {
		title += " · " + k
	}
	room := strings.TrimSpace(s.Room)
	if room == "" {
		room = c.Room
	}
	timing := !ev.StartsAt.Equal(at(s.Start).UTC()) || ev.RRule != rule || ev.TZ != loc.String()
	ev.WorkspaceID, ev.CourseID = c.WorkspaceID, c.ID
	ev.Title, ev.Location, ev.Color, ev.Notes = title, room, c.Color, c.Teacher
	ev.StartsAt, ev.EndsAt = at(s.Start).UTC(), at(s.End).UTC()
	ev.AllDay, ev.TZ, ev.RRule = false, loc.String(), rule
	if timing {
		ev.ExDates = "" // cancelled classes belong to the old timing
	}
	ev.Until = SeriesUntil(*ev)
	return nil
}
