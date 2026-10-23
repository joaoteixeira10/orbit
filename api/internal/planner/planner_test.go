package planner

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

// TestCleanRRule: day-or-longer rules are accepted and normalised; sub-daily
// and malformed ones are refused.
func TestCleanRRule(t *testing.T) {
	if got, err := CleanRRule(" RRULE:FREQ=WEEKLY;BYDAY=MO,WE "); err != nil || got != "FREQ=WEEKLY;BYDAY=MO,WE" {
		t.Errorf("weekly: %q %v", got, err)
	}
	if got, err := CleanRRule(""); err != nil || got != "" {
		t.Errorf("empty: %q %v", got, err)
	}
	for _, bad := range []string{"FREQ=HOURLY", "FREQ=MINUTELY;INTERVAL=5", "NOPE", "FREQ=DAILY;INTERVAL=5000"} {
		if _, err := CleanRRule(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// TestOccurrencesKeepLocalTimeAcrossDST: a weekly 09:00 series in Lisbon stays
// at 09:00 local after the clocks change (25 Oct 2026), and a removed
// occurrence is skipped.
func TestOccurrencesKeepLocalTimeAcrossDST(t *testing.T) {
	lis := mustLoc(t, "Europe/Lisbon")
	start := time.Date(2026, 10, 19, 9, 0, 0, 0, lis)
	ev := models.Event{
		StartsAt: start.UTC(), EndsAt: start.Add(time.Hour).UTC(), TZ: "Europe/Lisbon",
		RRule:   "FREQ=WEEKLY",
		ExDates: EncodeExDates([]time.Time{time.Date(2026, 11, 2, 9, 0, 0, 0, lis)}),
	}
	got := Occurrences(ev, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
	if len(got) != 3 { // 19 Oct, 26 Oct, (2 Nov removed), 9 Nov
		t.Fatalf("got %d occurrences: %v", len(got), got)
	}
	for _, o := range got {
		if l := o.In(lis); l.Hour() != 9 || l.Weekday() != time.Monday {
			t.Errorf("occurrence %v is not Monday 09:00 in Lisbon", l)
		}
	}
}

// TestOccurrencesOverlap: a single event is returned when any part of it is
// in the range, and an occurrence that started before the range but is still
// running counts.
func TestOccurrencesOverlap(t *testing.T) {
	s := time.Date(2026, 10, 20, 23, 0, 0, 0, time.UTC)
	ev := models.Event{StartsAt: s, EndsAt: s.Add(2 * time.Hour)}
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if len(Occurrences(ev, day, day.AddDate(0, 0, 1))) != 1 {
		t.Error("an event running past midnight belongs to the next day too")
	}
	if len(Occurrences(ev, day.AddDate(0, 0, 1), day.AddDate(0, 0, 2))) != 0 {
		t.Error("an event that ended isn't in a later range")
	}
}

// TestSeriesUntil: COUNT and UNTIL bound a series; an open-ended one is nil.
func TestSeriesUntil(t *testing.T) {
	s := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	ev := models.Event{StartsAt: s, EndsAt: s.Add(time.Hour), RRule: "FREQ=DAILY;COUNT=3"}
	if u := SeriesUntil(ev); u == nil || !u.Equal(s.AddDate(0, 0, 2).Add(time.Hour)) {
		t.Errorf("COUNT=3 until = %v", u)
	}
	ev.RRule = "FREQ=WEEKLY"
	if SeriesUntil(ev) != nil {
		t.Error("an open-ended series has no end")
	}
	ev.RRule = ""
	if u := SeriesUntil(ev); u == nil || !u.Equal(ev.EndsAt) {
		t.Error("a single event ends at its end")
	}
}

// TestNextDue: repeating tasks advance from their due date.
func TestNextDue(t *testing.T) {
	cases := []struct{ due, rule, want string }{
		{"2026-10-13", "FREQ=WEEKLY", "2026-10-20"},
		{"2026-10-13", "FREQ=DAILY;INTERVAL=3", "2026-10-16"},
		{"2026-01-31", "FREQ=MONTHLY", "2026-03-31"}, // no 31 Feb
		{"2026-10-13", "FREQ=WEEKLY;BYDAY=MO,FR", "2026-10-16"},
	}
	for _, c := range cases {
		if got, ok := NextDue(c.due, c.rule); !ok || got != c.want {
			t.Errorf("NextDue(%s, %s) = %s %v, want %s", c.due, c.rule, got, ok, c.want)
		}
	}
	if _, ok := NextDue("2026-10-13", "FREQ=DAILY;COUNT=1"); ok {
		t.Error("a finished series has no next date")
	}
}

const outlookFeed = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:test\r\n" +
	// Weekly class in a Windows zone; the 2nd occurrence moved, the 3rd cancelled via EXDATE.
	"BEGIN:VEVENT\r\nUID:class-1\r\nDTSTAMP:20261001T000000Z\r\nSUMMARY:Maths\r\nLOCATION:Room 2\\, B\r\n" +
	"DTSTART;TZID=GMT Standard Time:20261005T090000\r\nDTEND;TZID=GMT Standard Time:20261005T103000\r\n" +
	"RRULE:FREQ=WEEKLY;COUNT=4\r\nEXDATE;TZID=GMT Standard Time:20261019T090000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:class-1\r\nDTSTAMP:20261001T000000Z\r\nSUMMARY:Maths (moved)\r\n" +
	"RECURRENCE-ID;TZID=GMT Standard Time:20261012T090000\r\n" +
	"DTSTART;TZID=GMT Standard Time:20261013T140000\r\nDTEND;TZID=GMT Standard Time:20261013T153000\r\nEND:VEVENT\r\n" +
	// An all-day holiday and a cancelled meeting.
	"BEGIN:VEVENT\r\nUID:hol\r\nDTSTAMP:20261001T000000Z\r\nSUMMARY:Holiday\r\nDTSTART;VALUE=DATE:20261008\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:gone\r\nDTSTAMP:20261001T000000Z\r\nSUMMARY:Cancelled\r\nSTATUS:CANCELLED\r\n" +
	"DTSTART:20261009T100000Z\r\nDTEND:20261009T110000Z\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// TestParseFeed: Windows zone names load, a moved occurrence replaces the
// original, EXDATEs and cancelled events are dropped, all-day stays a date.
func TestParseFeed(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got, err := ParseFeed(strings.NewReader(outlookFeed), from, from.AddDate(0, 2, 0), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, o := range got {
		titles = append(titles, o.Start.Format("01-02 15:04")+" "+o.Title)
	}
	// London is UTC+1 until 25 Oct: 09:00 local = 08:00Z; the moved one 14:00 = 13:00Z.
	want := []string{"10-05 08:00 Maths", "10-08 00:00 Holiday", "10-13 13:00 Maths (moved)", "10-26 09:00 Maths"}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Errorf("got  %v\nwant %v", titles, want)
	}
	for _, o := range got {
		if o.Title == "Holiday" && (!o.AllDay || o.End.Sub(o.Start) != 24*time.Hour) {
			t.Errorf("holiday should be one all-day date: %+v", o)
		}
		if o.Title == "Maths" && o.Location != "Room 2, B" {
			t.Errorf("escaped text not decoded: %q", o.Location)
		}
	}
}

// TestFeedRoundTrip: what BuildFeed writes, ParseFeed reads back the same,
// including a long folded line with multi-byte characters.
func TestFeedRoundTrip(t *testing.T) {
	lis := mustLoc(t, "Europe/Lisbon")
	s := time.Date(2026, 10, 19, 9, 0, 0, 0, lis)
	long := strings.Repeat("Reunião de equipa; notas, ", 8)
	ics := BuildFeed("Mine", []FeedEvent{
		{UID: "e1@orbit", Title: "Stand-up", Notes: long, Start: s, End: s.Add(15 * time.Minute), TZ: "Europe/Lisbon", RRule: "FREQ=WEEKLY;COUNT=2"},
		{UID: "e2@orbit", Title: "Trip", Start: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), AllDay: true},
	}, []FeedTask{{UID: "t1@orbit", Title: "Essay", Due: "2026-10-21"}}, time.Now())
	for _, line := range bytes.Split(ics, []byte("\r\n")) {
		if len(line) > 75 {
			t.Fatalf("line longer than 75 octets: %q", line)
		}
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got, err := ParseFeed(bytes.NewReader(ics), from, from.AddDate(0, 1, 0), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("want 2 stand-ups, the trip and the task, got %d: %+v", len(got), got)
	}
	if got[0].Notes != strings.TrimSpace(long) || !got[0].Start.Equal(s) {
		t.Errorf("first occurrence wrong: %+v", got[0])
	}
	if got[1].Title != "☐ Essay" || !got[1].AllDay {
		t.Errorf("task should be an all-day entry: %+v", got[1])
	}
	if got[2].End.Sub(got[2].Start) != 48*time.Hour {
		t.Errorf("trip should span two days: %+v", got[2])
	}
}

// TestCleanColumns: ids are assigned, and a list needs an open and a done column.
func TestCleanColumns(t *testing.T) {
	cols, err := CleanColumns(Columns("A", "B", "C"))
	if err != nil || len(cols) != 3 || cols[0].ID == "" || cols[0].ID == cols[1].ID || !cols[2].Done {
		t.Fatalf("got %+v %v", cols, err)
	}
	if _, err := CleanColumns([]Column{{Name: "Only", Done: true}}); err == nil {
		t.Error("a list without an open column was accepted")
	}
	if _, err := CleanColumns([]Column{{Name: " "}, {Name: "x", Done: true}}); err == nil {
		t.Error("an unnamed column was accepted")
	}
}

// TestModules: only known, available modules are kept, in order, once.
func TestModules(t *testing.T) {
	got, ok := CleanModules([]string{"calendar", "pages", "calendar", "habits", "nope"})
	if ok || strings.Join(got, ",") != "calendar,pages,habits" {
		t.Errorf("got %v %v", got, ok)
	}
	if TemplateFor("school").Kind != "school" || TemplateFor("whatever").Kind != "custom" {
		t.Error("template lookup")
	}
}

// TestDocText: blocks become lines; invalid documents give nothing.
func TestDocText(t *testing.T) {
	doc := `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"Title"}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]},` +
		`{"type":"taskList","content":[{"type":"taskItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Buy milk"}]}]}]}]}`
	if got := DocText(doc); got != "Title\nHello world\nBuy milk" {
		t.Errorf("got %q", got)
	}
	if CheckDoc("") != nil || CheckDoc("# Hi") != nil || CheckDoc(doc) != nil || CheckDoc("\xff") == nil || CheckDoc(strings.Repeat("a", MaxPageBytes+1)) == nil {
		t.Error("CheckDoc")
	}
	if Snippet("aaaa bbbb cccc dddd", "cccc", 9) == "" {
		t.Error("snippet")
	}
}

// TestRank: keys fall between their neighbours.
func TestRank(t *testing.T) {
	a, b := 1024.0, 2048.0
	if r := Rank(&a, &b); r <= a || r >= b {
		t.Errorf("between: %v", r)
	}
	if Rank(nil, &a) >= a || Rank(&b, nil) <= b || Rank(nil, nil) == 0 {
		t.Error("edges")
	}
}

// TestParseFeedHostileRules: a series every second (or hour) since 2000 can't
// keep the refresh busy — sub-hourly series are skipped, every series shares
// one step budget — and the ordinary events of the feed still come through.
func TestParseFeedHostileRules(t *testing.T) {
	feed := strings.ReplaceAll(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//EN
BEGIN:VEVENT
UID:every-second
DTSTART:20000101T000000Z
DTEND:20000101T000001Z
RRULE:FREQ=SECONDLY
SUMMARY:Spin
END:VEVENT
BEGIN:VEVENT
UID:every-hour
DTSTART:20000101T000000Z
DTEND:20000101T003000Z
RRULE:FREQ=HOURLY
SUMMARY:Hourly since 2000
END:VEVENT
BEGIN:VEVENT
UID:weekly
DTSTART:20261005T090000Z
DTEND:20261005T100000Z
RRULE:FREQ=WEEKLY
SUMMARY:Standup
END:VEVENT
END:VCALENDAR
`, "\n", "\r\n")
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	start := time.Now()
	got, err := ParseFeed(strings.NewReader(feed), from, from.AddDate(0, 1, 0), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %s", d)
	}
	weekly := 0
	for _, o := range got {
		switch o.UID {
		case "every-second":
			t.Fatal("a per-second series was expanded")
		case "weekly":
			weekly++
		}
	}
	if weekly < 4 {
		t.Fatalf("the weekly event is missing: %d occurrences", weekly)
	}
}
