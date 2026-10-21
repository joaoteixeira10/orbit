package planner

import (
	"strings"
	"testing"
	"time"
)

const remoteSeries = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//macOS//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:abc-123\r\nDTSTAMP:20261001T080000Z\r\nDTSTART;TZID=Europe/Lisbon:20261005T090000\r\nDTEND;TZID=Europe/Lisbon:20261005T103000\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nEXDATE;TZID=Europe/Lisbon:20261012T090000\r\nSUMMARY:Maths\r\nLOCATION:Room 4\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT10M\r\nDESCRIPTION:Maths\r\nEND:VALARM\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:abc-123\r\nDTSTAMP:20261001T080000Z\r\nRECURRENCE-ID;TZID=Europe/Lisbon:20261019T090000\r\nDTSTART;TZID=Europe/Lisbon:20261019T100000\r\nDTEND;TZID=Europe/Lisbon:20261019T113000\r\nSUMMARY:Maths (moved)\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseObject(t *testing.T) {
	r, err := ParseObject([]byte(remoteSeries), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if r.UID != "abc-123" || r.Title != "Maths" || r.Location != "Room 4" || r.TZ != "Europe/Lisbon" || r.RRule != "FREQ=WEEKLY;BYDAY=MO" {
		t.Fatalf("parsed: %+v", r)
	}
	// 09:00 Lisbon in October (summer time) is 08:00 UTC.
	if !r.Start.Equal(time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)) || r.End.Sub(r.Start) != 90*time.Minute {
		t.Errorf("times: %v – %v", r.Start, r.End)
	}
	if len(r.ExDates) != 1 || !r.ExDates[0].Equal(time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("exdates: %v", r.ExDates)
	}
	if _, err := ParseObject([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\nBEGIN:VTODO\r\nUID:t\r\nDTSTAMP:20261001T080000Z\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"), time.UTC); err != ErrNoEvent {
		t.Errorf("a to-do: %v", err)
	}
	day, err := ParseObject(BuildObject(FeedEvent{UID: "d", Title: "Trip", AllDay: true, Start: time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 12, 27, 0, 0, 0, 0, time.UTC)}, time.Now()), time.UTC)
	if err != nil || !day.AllDay || day.End.Sub(day.Start) != 72*time.Hour {
		t.Errorf("all-day round trip: %+v %v", day, err)
	}
	// A timed event keeps its own zone in an object.
	ny, _ := ParseObject(BuildObject(FeedEvent{UID: "n", Title: "Call", TZ: "America/New_York", Start: time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC), End: time.Date(2026, 12, 1, 16, 0, 0, 0, time.UTC)}, time.Now()), time.UTC)
	if ny.TZ != "America/New_York" || !ny.Start.Equal(time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("zoned round trip: %+v", ny)
	}
}

func TestMergeObject(t *testing.T) {
	r, _ := ParseObject([]byte(remoteSeries), time.UTC)
	local := FeedEvent{UID: r.UID, Title: r.Title, Location: r.Location, Start: r.Start, End: r.End, TZ: r.TZ, RRule: r.RRule, ExDates: r.ExDates}
	if _, changed, err := MergeObject([]byte(remoteSeries), local, time.UTC, time.Now()); err != nil || changed {
		t.Fatalf("nothing changed: %v %v", changed, err)
	}
	// A new title only: the alarm and the moved occurrence stay.
	local.Title = "Maths A"
	out, changed, err := MergeObject([]byte(remoteSeries), local, time.UTC, time.Now())
	if err != nil || !changed {
		t.Fatalf("merge: %v %v", changed, err)
	}
	s := string(out)
	for _, want := range []string{"SUMMARY:Maths A", "BEGIN:VALARM", "Maths (moved)", "RRULE:FREQ=WEEKLY;BYDAY=MO", "DTSTART;TZID=Europe/Lisbon:20261005T090000"} {
		if !strings.Contains(s, want) {
			t.Errorf("merged object lacks %q:\n%s", want, s)
		}
	}
	// An hour later, and one more occurrence removed.
	local.Start, local.End = local.Start.Add(time.Hour), local.End.Add(time.Hour)
	local.ExDates = []time.Time{time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)}
	out, _, _ = MergeObject([]byte(remoteSeries), local, time.UTC, time.Now())
	back, err := ParseObject(out, time.UTC)
	if err != nil || !back.Start.Equal(local.Start) || len(back.ExDates) != 1 || !back.ExDates[0].Equal(local.ExDates[0]) || back.Title != "Maths A" {
		t.Errorf("after merge: %+v %v\n%s", back, err, out)
	}
	if !strings.Contains(string(out), "SEQUENCE:1") {
		t.Error("a new time bumps SEQUENCE")
	}
	// A rule Orbit can't follow (hourly) isn't touched by an edit.
	hourly := strings.Replace(remoteSeries, "RRULE:FREQ=WEEKLY;BYDAY=MO", "RRULE:FREQ=HOURLY;COUNT=3", 1)
	h, _ := ParseObject([]byte(hourly), time.UTC)
	if h.RRule != "" {
		t.Fatalf("hourly read as %q", h.RRule)
	}
	out, _, _ = MergeObject([]byte(hourly), FeedEvent{UID: h.UID, Title: "Renamed", Location: h.Location, Start: h.Start, End: h.End, TZ: h.TZ, ExDates: h.ExDates}, time.UTC, time.Now())
	if !strings.Contains(string(out), "RRULE:FREQ=HOURLY;COUNT=3") || !strings.Contains(string(out), "SUMMARY:Renamed") {
		t.Errorf("hourly rule lost:\n%s", out)
	}
}
