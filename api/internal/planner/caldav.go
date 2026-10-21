package planner

import (
	"bytes"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
)

// ---- CalDAV objects (two-way sync) --------------------------------------------

// RemoteEvent is what Orbit reads from a calendar object on a CalDAV server:
// its event (the series — occurrences edited in another app stay there).
type RemoteEvent struct {
	UID      string
	Title    string
	Location string
	Notes    string
	Start    time.Time
	End      time.Time
	AllDay   bool
	TZ       string // the zone DTSTART is in ("" for UTC or floating)
	RRule    string // "" when none, or one Orbit can't repeat
	ExDates  []time.Time
	Modified time.Time // LAST-MODIFIED (zero when not said)
}

// ErrNoEvent: the object holds no event (a to-do, a journal entry).
var ErrNoEvent = errors.New("not an event")

// master is the object's event: the one without RECURRENCE-ID.
func master(cal *ical.Calendar) *ical.Event {
	for _, ev := range cal.Events() {
		if ev.Props.Get(ical.PropRecurrenceID) == nil {
			e := ev
			return &e
		}
	}
	return nil
}

// ParseObject reads one calendar object. Times without a zone are read in loc.
func ParseObject(data []byte, loc *time.Location) (RemoteEvent, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return RemoteEvent{}, errors.New("not an iCalendar object")
	}
	fixZones(cal.Component)
	ev := master(cal)
	if ev == nil {
		return RemoteEvent{}, ErrNoEvent
	}
	return readEvent(ev, loc)
}

func readEvent(ev *ical.Event, loc *time.Location) (RemoteEvent, error) {
	startProp := ev.Props.Get(ical.PropDateTimeStart)
	if startProp == nil {
		return RemoteEvent{}, errors.New("the event has no start")
	}
	start, err := ev.DateTimeStart(loc)
	if err != nil {
		return RemoteEvent{}, err
	}
	end, err := ev.DateTimeEnd(loc)
	if err != nil || end.Before(start) {
		end = start
	}
	r := RemoteEvent{
		UID:      text(ev.Component, ical.PropUID),
		Title:    text(ev.Component, ical.PropSummary),
		Location: text(ev.Component, ical.PropLocation),
		Notes:    text(ev.Component, ical.PropDescription),
		AllDay:   startProp.ValueType() == ical.ValueDate || len(startProp.Value) == 8,
	}
	if r.AllDay {
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
		if !end.After(start) {
			end = start.AddDate(0, 0, 1)
		}
	} else if tz := startProp.Params.Get(ical.ParamTimezoneID); tz != "" {
		r.TZ = tz
	}
	r.Start, r.End = start.UTC(), end.UTC()
	if t, err := ev.Props.DateTime(ical.PropLastModified, time.UTC); err == nil {
		r.Modified = t
	}
	if p := ev.Props.Get(ical.PropRecurrenceRule); p != nil {
		if rule, err := CleanRRule(p.Value); err == nil {
			r.RRule = rule
		}
	}
	for _, p := range ev.Props.Values(ical.PropExceptionDates) {
		for _, v := range strings.Split(p.Value, ",") {
			one := p
			one.Value = strings.TrimSpace(v)
			t, err := one.DateTime(loc)
			if err != nil {
				continue
			}
			if r.AllDay || len(one.Value) == 8 {
				t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			}
			r.ExDates = append(r.ExDates, t.UTC())
		}
	}
	return r, nil
}

// BuildObject writes an Orbit event as a new calendar object.
func BuildObject(e FeedEvent, now time.Time) []byte {
	var b strings.Builder
	w := func(l string) { fold(&b, l) }
	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//Orbit//Planner//EN")
	e.Zoned = true
	writeEvent(w, e, now.UTC().Format(icsUTC))
	w("END:VCALENDAR")
	return []byte(b.String())
}

// MergeObject writes an Orbit event's changes into the object it came from:
// only what differs from what the object says is rewritten, so what Orbit
// doesn't keep (alarms, attendees, edited occurrences, a repeat rule it
// can't follow) stays as it is. changed is false when there's nothing to do.
func MergeObject(data []byte, e FeedEvent, loc *time.Location, now time.Time) (out []byte, changed bool, err error) {
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, false, errors.New("not an iCalendar object")
	}
	fixZones(cal.Component)
	ev := master(cal)
	if ev == nil {
		return nil, false, ErrNoEvent
	}
	was, err := readEvent(ev, loc)
	if err != nil {
		return nil, false, err
	}
	setText := func(name, old, v string) {
		if old == v {
			return
		}
		changed = true
		if v == "" {
			ev.Props.Del(name)
		} else {
			ev.Props.SetText(name, v)
		}
	}
	setText(ical.PropSummary, was.Title, e.Title)
	setText(ical.PropLocation, was.Location, e.Location)
	setText(ical.PropDescription, was.Notes, e.Notes)

	zone := e.TZ
	if e.AllDay || zone == "UTC" {
		zone = ""
	}
	timing := !was.Start.Equal(e.Start) || !was.End.Equal(e.End) || was.AllDay != e.AllDay || (!e.AllDay && was.TZ != zone && zone != "")
	if timing {
		changed = true
		ev.Props.Del(ical.PropDuration)
		ev.Props.Set(timeProp(ical.PropDateTimeStart, e.Start, e.AllDay, zone))
		ev.Props.Set(timeProp(ical.PropDateTimeEnd, e.End, e.AllDay, zone))
	}
	if was.RRule != e.RRule {
		changed = true
		if e.RRule == "" {
			ev.Props.Del(ical.PropRecurrenceRule)
		} else {
			p := ical.NewProp(ical.PropRecurrenceRule)
			p.SetValueType(ical.ValueRecurrence)
			p.Value = e.RRule
			ev.Props.Set(p)
		}
	}
	if timing || !sameTimes(was.ExDates, e.ExDates) {
		changed = true
		ev.Props.Del(ical.PropExceptionDates)
		if e.RRule != "" {
			for _, x := range e.ExDates {
				ev.Props.Add(timeProp(ical.PropExceptionDates, x, e.AllDay, zone))
			}
		}
	}
	if !changed {
		return data, false, nil
	}
	ev.Props.SetDateTime(ical.PropDateTimeStamp, now.UTC())
	ev.Props.SetDateTime(ical.PropLastModified, now.UTC())
	if timing {
		seq := 0
		if p := ev.Props.Get(ical.PropSequence); p != nil {
			seq, _ = p.Int()
		}
		p := ical.NewProp(ical.PropSequence)
		p.Value = strconv.Itoa(seq + 1)
		ev.Props.Set(p)
	}
	// Put the master back (Events() hands out copies of the components).
	for i, child := range cal.Children {
		if child.Name == ical.CompEvent && child.Props.Get(ical.PropRecurrenceID) == nil {
			cal.Children[i] = ev.Component
			break
		}
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

// timeProp is a DTSTART / DTEND / EXDATE: a date, a time in a zone, or UTC.
func timeProp(name string, t time.Time, allDay bool, zone string) *ical.Prop {
	p := ical.NewProp(name)
	switch {
	case allDay:
		p.SetDate(t.UTC())
	case zone != "":
		p.SetValueType(ical.ValueDateTime)
		p.Params.Set(ical.ParamTimezoneID, zone)
		p.Value = t.In(location(zone)).Format(icsLocal)
	default:
		p.SetDateTime(t.UTC())
	}
	return p
}

func sameTimes(a, b []time.Time) bool {
	key := func(ts []time.Time) []int64 {
		out := make([]int64, len(ts))
		for i, t := range ts {
			out[i] = t.Unix()
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(key(a), key(b))
}
