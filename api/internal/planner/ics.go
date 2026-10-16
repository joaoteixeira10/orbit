package planner

import (
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// ---- Import (calendar subscriptions) ---------------------------------------

// ExternalOccurrence is one occurrence read from an ICS feed.
type ExternalOccurrence struct {
	UID      string
	Title    string
	Location string
	Notes    string
	Start    time.Time
	End      time.Time
	AllDay   bool
}

// MaxFeedOccurrences bounds what one subscription stores.
const MaxFeedOccurrences = 5000

// The work of expanding a feed's repeating events is bounded: every step
// counts, also those before the window (a series from years ago is walked
// from its start). One series gets seriesSteps (a daily series for over a
// century; anything denser that old is dropped), the whole feed feedSteps,
// so one file can't keep the refresh busy however its rules are written,
// and one bad series doesn't starve the rest.
const (
	seriesSteps = 50_000
	feedSteps   = 1_000_000
)

// windowsZones maps the Windows time-zone names Outlook / Exchange put in
// TZID to IANA names (the common ones; others fall back to the default zone).
var windowsZones = map[string]string{
	"GMT Standard Time":              "Europe/London",
	"Greenwich Standard Time":        "Atlantic/Reykjavik",
	"W. Europe Standard Time":        "Europe/Berlin",
	"Central Europe Standard Time":   "Europe/Budapest",
	"Romance Standard Time":          "Europe/Paris",
	"Central European Standard Time": "Europe/Warsaw",
	"E. Europe Standard Time":        "Europe/Chisinau",
	"FLE Standard Time":              "Europe/Kiev",
	"GTB Standard Time":              "Europe/Bucharest",
	"Russian Standard Time":          "Europe/Moscow",
	"Azores Standard Time":           "Atlantic/Azores",
	"Cape Verde Standard Time":       "Atlantic/Cape_Verde",
	"E. South America Standard Time": "America/Sao_Paulo",
	"Eastern Standard Time":          "America/New_York",
	"Central Standard Time":          "America/Chicago",
	"Mountain Standard Time":         "America/Denver",
	"Pacific Standard Time":          "America/Los_Angeles",
	"India Standard Time":            "Asia/Kolkata",
	"China Standard Time":            "Asia/Shanghai",
	"Tokyo Standard Time":            "Asia/Tokyo",
	"AUS Eastern Standard Time":      "Australia/Sydney",
	"UTC":                            "UTC",
}

// fixZones rewrites TZID parameters Go can't load (Windows names) to IANA
// names, or drops them (the time is then read in the default zone).
func fixZones(comp *ical.Component) {
	for name, props := range comp.Props {
		for i := range props {
			tz := props[i].Params.Get(ical.ParamTimezoneID)
			if tz == "" {
				continue
			}
			tz = strings.Trim(tz, `"`)
			if _, err := time.LoadLocation(tz); err == nil {
				props[i].Params.Set(ical.ParamTimezoneID, tz)
				continue
			}
			if iana, ok := windowsZones[tz]; ok {
				props[i].Params.Set(ical.ParamTimezoneID, iana)
			} else {
				props[i].Params.Del(ical.ParamTimezoneID)
			}
		}
		comp.Props[name] = props
	}
	for _, child := range comp.Children {
		fixZones(child)
	}
}

func text(comp *ical.Component, name string) string {
	s, _ := comp.Props.Text(name)
	return strings.TrimSpace(s)
}

// ParseFeed reads an ICS calendar and returns the occurrences that overlap
// [from, to), repeating series expanded, edited occurrences (RECURRENCE-ID)
// in place of the ones they replace, cancelled ones left out. Times without a
// zone are read in loc.
func ParseFeed(r io.Reader, from, to time.Time, loc *time.Location) ([]ExternalOccurrence, error) {
	cal, err := ical.NewDecoder(r).Decode()
	if err != nil {
		// Not the decoder's message: it can quote the answer, and a
		// subscription may point at a private address.
		return nil, errors.New("the address didn't return an iCalendar (.ics) file")
	}
	fixZones(cal.Component)

	// Occurrences replaced by an edited copy: uid → set of original starts.
	overridden := map[string]map[int64]bool{}
	for _, ev := range cal.Events() {
		if p := ev.Props.Get(ical.PropRecurrenceID); p != nil {
			if t, err := p.DateTime(loc); err == nil {
				if p.ValueType() == ical.ValueDate || len(p.Value) == 8 {
					t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
				}
				uid := text(ev.Component, ical.PropUID)
				if overridden[uid] == nil {
					overridden[uid] = map[int64]bool{}
				}
				overridden[uid][t.Unix()] = true
			}
		}
	}

	var out []ExternalOccurrence
	budget := feedSteps
	for _, ev := range cal.Events() {
		if st, _ := ev.Status(); st == ical.EventCancelled {
			continue
		}
		startProp := ev.Props.Get(ical.PropDateTimeStart)
		if startProp == nil {
			continue
		}
		start, err := ev.DateTimeStart(loc)
		if err != nil {
			continue
		}
		end, err := ev.DateTimeEnd(loc)
		if err != nil || end.Before(start) {
			end = start
		}
		allDay := startProp.ValueType() == ical.ValueDate || len(startProp.Value) == 8
		if allDay {
			// All-day dates are floating: keep the calendar date, at UTC midnight.
			start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
			end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
			if !end.After(start) {
				end = start.AddDate(0, 0, 1)
			}
		}
		base := ExternalOccurrence{
			UID:      text(ev.Component, ical.PropUID),
			Title:    text(ev.Component, ical.PropSummary),
			Location: text(ev.Component, ical.PropLocation),
			Notes:    text(ev.Component, ical.PropDescription),
			AllDay:   allDay,
		}
		dur := end.Sub(start)

		var starts []time.Time
		setLoc := loc
		if allDay {
			setLoc = time.UTC // dates repeat as dates: UTC midnights, like start
		}
		set, err := ev.RecurrenceSet(setLoc)
		if err != nil || set == nil || ev.Props.Get(ical.PropRecurrenceID) != nil {
			if Overlaps(start, end, from, to) {
				starts = []time.Time{start}
			}
		} else if subHourly(set) {
			continue // every minute / second: no calendar means it, and it would eat the budget
		} else {
			starts = expand(set, dur, from, to, &budget)
		}
		for _, s := range starts {
			if overridden[base.UID][s.Unix()] && ev.Props.Get(ical.PropRecurrenceID) == nil {
				continue
			}
			o := base
			o.Start, o.End = s.UTC(), s.Add(dur).UTC()
			out = append(out, o)
			if len(out) >= MaxFeedOccurrences {
				return sortOccurrences(out), nil
			}
		}
	}
	return sortOccurrences(out), nil
}

func expand(set *rrule.Set, dur time.Duration, from, to time.Time, budget *int) []time.Time {
	var out []time.Time
	it := set.Iterator()
	for steps := 0; ; steps++ {
		if *budget <= 0 || steps >= seriesSteps {
			return out
		}
		*budget--
		t, ok := it()
		if !ok || !t.Before(to) || len(out) >= maxExpand {
			return out
		}
		if Overlaps(t, t.Add(dur), from, to) {
			out = append(out, t)
		}
	}
}

// subHourly reports a series that repeats every minute or second.
func subHourly(set *rrule.Set) bool {
	r := set.GetRRule()
	return r != nil && (r.OrigOptions.Freq == rrule.MINUTELY || r.OrigOptions.Freq == rrule.SECONDLY)
}

func sortOccurrences(o []ExternalOccurrence) []ExternalOccurrence {
	sort.SliceStable(o, func(i, j int) bool { return o[i].Start.Before(o[j].Start) })
	return o
}

// ---- Export (calendar feeds) ------------------------------------------------

// FeedEvent is an Orbit event as written to a feed.
type FeedEvent struct {
	UID      string
	Title    string
	Notes    string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
	TZ       string // the zone a repeating timed event repeats in
	RRule    string
	ExDates  []time.Time
	Updated  time.Time
}

// FeedTask is an open task with a due date, written as an event: all-day, or
// a 30-minute slot at its floating due time.
type FeedTask struct {
	UID     string
	Title   string
	Notes   string
	Due     string // YYYY-MM-DD
	DueTime string // HH:MM or ""
	Updated time.Time
}

// icsEscape escapes TEXT values (RFC 5545 §3.3.11).
func icsEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// fold writes one content line folded at 75 octets (RFC 5545 §3.1), without
// splitting a UTF-8 sequence.
func fold(b *strings.Builder, line string) {
	limit := 75 // continuation lines start with a space, so they hold 74
	for len(line) > limit {
		cut := limit
		for cut > 0 && line[cut]&0xC0 == 0x80 {
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		limit = 74
	}
	b.WriteString(line)
	b.WriteString("\r\n")
}

const (
	icsUTC   = "20060102T150405Z"
	icsLocal = "20060102T150405"
	icsDate  = "20060102"
)

// BuildFeed writes an iCalendar document. now stamps DTSTAMP.
func BuildFeed(name string, events []FeedEvent, tasks []FeedTask, now time.Time) []byte {
	var b strings.Builder
	w := func(l string) { fold(&b, l) }
	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//Orbit//Planner//EN")
	w("CALSCALE:GREGORIAN")
	w("X-WR-CALNAME:" + icsEscape(name))
	w("NAME:" + icsEscape(name))
	w("REFRESH-INTERVAL;VALUE=DURATION:PT30M")
	w("X-PUBLISHED-TTL:PT30M")
	stamp := now.UTC().Format(icsUTC)
	for _, e := range events {
		w("BEGIN:VEVENT")
		w("UID:" + e.UID)
		w("DTSTAMP:" + stamp)
		if !e.Updated.IsZero() {
			w("LAST-MODIFIED:" + e.Updated.UTC().Format(icsUTC))
		}
		switch {
		case e.AllDay:
			w("DTSTART;VALUE=DATE:" + e.Start.UTC().Format(icsDate))
			w("DTEND;VALUE=DATE:" + e.End.UTC().Format(icsDate))
		case e.RRule != "" && e.TZ != "" && e.TZ != "UTC":
			// A timed series repeats in its own zone (keeps 09:00 across DST).
			loc := location(e.TZ)
			w("DTSTART;TZID=" + e.TZ + ":" + e.Start.In(loc).Format(icsLocal))
			w("DTEND;TZID=" + e.TZ + ":" + e.End.In(loc).Format(icsLocal))
		default:
			w("DTSTART:" + e.Start.UTC().Format(icsUTC))
			w("DTEND:" + e.End.UTC().Format(icsUTC))
		}
		if e.RRule != "" {
			w("RRULE:" + e.RRule)
			for _, x := range e.ExDates {
				switch {
				case e.AllDay:
					w("EXDATE;VALUE=DATE:" + x.UTC().Format(icsDate))
				case e.TZ != "" && e.TZ != "UTC":
					w("EXDATE;TZID=" + e.TZ + ":" + x.In(location(e.TZ)).Format(icsLocal))
				default:
					w("EXDATE:" + x.UTC().Format(icsUTC))
				}
			}
		}
		w("SUMMARY:" + icsEscape(e.Title))
		if e.Location != "" {
			w("LOCATION:" + icsEscape(e.Location))
		}
		if e.Notes != "" {
			w("DESCRIPTION:" + icsEscape(e.Notes))
		}
		w("END:VEVENT")
	}
	for _, t := range tasks {
		d, err := time.Parse(DateLayout, t.Due)
		if err != nil {
			continue
		}
		w("BEGIN:VEVENT")
		w("UID:" + t.UID)
		w("DTSTAMP:" + stamp)
		if t.DueTime != "" {
			start, err := time.Parse(DateLayout+" 15:04", t.Due+" "+t.DueTime)
			if err != nil {
				start = d
			}
			// Floating local time: the subscriber's calendar shows it in its own zone.
			w("DTSTART:" + start.Format(icsLocal))
			w("DTEND:" + start.Add(30*time.Minute).Format(icsLocal))
		} else {
			w("DTSTART;VALUE=DATE:" + d.Format(icsDate))
			w("DTEND;VALUE=DATE:" + d.AddDate(0, 0, 1).Format(icsDate))
		}
		w("SUMMARY:" + icsEscape("☐ "+t.Title))
		if t.Notes != "" {
			w("DESCRIPTION:" + icsEscape(t.Notes))
		}
		w("TRANSP:TRANSPARENT")
		w("END:VEVENT")
	}
	w("END:VCALENDAR")
	return []byte(b.String())
}
