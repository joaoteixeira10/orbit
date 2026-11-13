package planner

import (
	"math"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestAverageAndNeeded: marks on different scales and weights average as
// fractions; what the next mark must be follows from the weights.
func TestAverageAndNeeded(t *testing.T) {
	if _, ok := Average(nil); ok {
		t.Fatal("no grades has no average")
	}
	g := []models.Grade{
		{Score: 12, OutOf: 20, Weight: 1},  // 0.6
		{Score: 80, OutOf: 100, Weight: 3}, // 0.8
		{Score: 5, OutOf: 0, Weight: 1},    // ignored: out of nothing
	}
	avg, ok := Average(g)
	if !ok || !near(avg, (0.6+2.4)/4) {
		t.Fatalf("average = %v, %v", avg, ok)
	}
	// Aim at 15/20 (0.75) with an exam weighing 4 (as much as all the rest):
	// (0.75·8 − 3.0) / 4 = 0.75.
	if n := Needed(g, 0.75, 4); !near(n, 0.75) {
		t.Fatalf("needed = %v", n)
	}
	// Aim at 10/20 (0.5): already there whatever the exam — (4 − 3) / 4.
	if n := Needed(g, 0.5, 4); !near(n, 0.25) {
		t.Fatalf("needed for 0.5 = %v", n)
	}
	// Out of reach: 19/20 with a small test.
	if n := Needed(g, 0.95, 1); n <= 1 {
		t.Fatalf("needed for 0.95 = %v, want > 1", n)
	}
	// With no marks yet, the next one is the average.
	if n := Needed(nil, 0.7, 2); !near(n, 0.7) {
		t.Fatalf("needed with none = %v", n)
	}
}

// TestClassEvent: a weekly class starts on the first of its weekday from
// the term start, at its local time (kept across a DST change), and ends with
// the term.
func TestClassEvent(t *testing.T) {
	lis := mustLoc(t, "Europe/Lisbon")
	start, end := "2026-09-15", "2026-12-18" // a Tuesday … a Friday
	c := models.Course{Name: "Maths", Room: "B2", Color: "#007AFF", Teacher: "Ms Costa", TZ: "Europe/Lisbon", TermStart: &start, TermEnd: &end}
	c.ID, c.WorkspaceID = "c1", "w1"
	s := models.CourseSlot{Weekday: 1, Start: "09:00", End: "10:30", Kind: "Lab"} // Mondays
	var ev models.Event
	if err := ClassEvent(c, s, time.Now(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Title != "Maths · Lab" || ev.Location != "B2" || ev.CourseID != "c1" || ev.Notes != "Ms Costa" {
		t.Fatalf("event: %+v", ev)
	}
	first := ev.StartsAt.In(lis)
	if first.Format("2006-01-02 15:04 Mon") != "2026-09-21 09:00 Mon" || ev.EndsAt.Sub(ev.StartsAt) != 90*time.Minute {
		t.Fatalf("first class %v – %v", first, ev.EndsAt.In(lis))
	}
	occ := Occurrences(ev, ev.StartsAt, time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC))
	if len(occ) != 13 { // 21 Sep … 14 Dec
		t.Fatalf("classes in the term: %d", len(occ))
	}
	for _, o := range occ {
		if l := o.In(lis); l.Hour() != 9 || l.Weekday() != time.Monday {
			t.Fatalf("a class at %v", l)
		}
	}
	if ev.Until == nil || ev.Until.After(time.Date(2026, 12, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("until %v", ev.Until)
	}

	// Cancelling a class survives a change of room, not of time.
	ev.ExDates = EncodeExDates([]time.Time{occ[2]})
	s.Room = "Lab 3"
	_ = ClassEvent(c, s, time.Now(), &ev)
	if ev.Location != "Lab 3" || len(DecodeExDates(ev.ExDates)) != 1 {
		t.Fatalf("room change: %q, %q", ev.Location, ev.ExDates)
	}
	s.Start = "08:30"
	_ = ClassEvent(c, s, time.Now(), &ev)
	if ev.ExDates != "" {
		t.Fatal("a new time should forget the cancelled classes")
	}

	// No term: from the day it's made, for good; Sunday is 7.
	c.TermStart, c.TermEnd = nil, nil
	since := time.Date(2026, 10, 14, 12, 0, 0, 0, lis) // a Wednesday
	if err := ClassEvent(c, models.CourseSlot{Weekday: 7, Start: "18:00", End: "19:00"}, since, &ev); err != nil {
		t.Fatal(err)
	}
	if got := ev.StartsAt.In(lis).Format("2006-01-02 Mon"); got != "2026-10-18 Sun" || ev.Until != nil {
		t.Fatalf("no-term class: %s, until %v", got, ev.Until)
	}
	if err := ClassEvent(c, models.CourseSlot{Weekday: 2, Start: "10:00", End: "09:00"}, since, &ev); err == nil {
		t.Fatal("end before start accepted")
	}
}

func TestCourseTag(t *testing.T) {
	for in, want := range map[string]string{"Físico-Química": "físico-química", " Maths  A ": "maths-a", "C++ / C#": "c-c"} {
		if got := CourseTag(in); got != want {
			t.Errorf("CourseTag(%q) = %q, want %q", in, got, want)
		}
	}
}
