package planner

import "testing"

func d(s string) *string { return &s }

// TestCascade: moving a task's end moves what waits on it as many days,
// down the chain; nothing starts before all it waits on is due; done and
// undated tasks stay.
func TestCascade(t *testing.T) {
	tasks := []Dated{
		{ID: "design", Start: d("2026-11-02"), Due: d("2026-11-06")}, // moved: was due the 4th
		{ID: "build", Start: d("2026-11-05"), Due: d("2026-11-10"), WaitsOn: []string{"design"}},
		{ID: "test", Due: d("2026-11-12"), WaitsOn: []string{"build"}},
		{ID: "docs", Start: d("2026-11-20"), Due: d("2026-11-21"), WaitsOn: []string{"build", "copy"}},
		{ID: "copy", Start: d("2026-11-01"), Due: d("2026-11-25")},
		{ID: "old", Start: d("2026-11-05"), Due: d("2026-11-05"), WaitsOn: []string{"design"}, Done: true},
		{ID: "someday", WaitsOn: []string{"design"}},
	}
	got := map[string]Dated{}
	for _, x := range Cascade(tasks, "design", 2) {
		got[x.ID] = x
	}
	check := func(id, start, due string) {
		t.Helper()
		x, ok := got[id]
		if !ok {
			t.Fatalf("%s didn't move", id)
		}
		s := ""
		if x.Start != nil {
			s = *x.Start
		}
		if s != start || *x.Due != due {
			t.Errorf("%s: %s – %s, want %s – %s", id, s, *x.Due, start, due)
		}
	}
	// build: +2 days would be the 7th, but design is due the 6th → the 7th.
	check("build", "2026-11-07", "2026-11-12")
	check("test", "", "2026-11-14")
	// docs waits on copy (due the 25th) too: it goes after that.
	check("docs", "2026-11-26", "2026-11-27")
	for _, id := range []string{"old", "someday", "copy"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s moved", id)
		}
	}
	// Earlier: what waits follows, but not before what it waits on.
	tasks2 := []Dated{
		{ID: "a", Due: d("2026-11-02")},
		{ID: "b", Start: d("2026-11-06"), Due: d("2026-11-08"), WaitsOn: []string{"a"}},
	}
	if out := Cascade(tasks2, "a", -3); len(out) != 1 || *out[0].Start != "2026-11-03" {
		t.Errorf("earlier: %+v", out)
	}
	if out := Cascade(tasks2, "a", 0); out != nil {
		t.Error("no move, no cascade")
	}
}

func TestReaches(t *testing.T) {
	w := map[string][]string{"b": {"a"}, "c": {"b"}}
	if !Reaches(w, []string{"c"}, "a") || Reaches(w, []string{"a"}, "c") {
		t.Error("Reaches")
	}
}
