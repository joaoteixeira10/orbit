package planner

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/teambition/rrule-go"
)

// maxExpand bounds how many occurrences one series yields per query, so a
// FREQ=MINUTELY rule can't make a request (or a feed) huge.
const maxExpand = 2000

// CleanRRule validates a recurrence rule ("FREQ=WEEKLY;BYDAY=MO,WE"; an
// optional "RRULE:" prefix is dropped) and returns it normalised. Sub-daily
// frequencies are refused: a planner repeats by days, weeks, months, years.
func CleanRRule(s string) (string, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "RRULE:"))
	if s == "" {
		return "", nil
	}
	if len(s) > 300 {
		return "", errors.New("the repeat rule is too long")
	}
	opt, err := rrule.StrToROption(s)
	if err != nil {
		return "", errors.New("invalid repeat rule: " + err.Error())
	}
	switch opt.Freq {
	case rrule.DAILY, rrule.WEEKLY, rrule.MONTHLY, rrule.YEARLY:
	default:
		return "", errors.New("repeat daily, weekly, monthly or yearly")
	}
	if opt.Interval < 0 || opt.Interval > 999 {
		return "", errors.New("the repeat interval must be between 1 and 999")
	}
	return s, nil
}

// location returns the zone a series repeats in (UTC when unknown).
func location(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}

// eventSet builds the rrule set of a repeating event, anchored at its start
// in its own zone, minus its removed occurrences.
func eventSet(ev models.Event) (*rrule.Set, error) {
	loc := location(ev.TZ)
	if ev.AllDay {
		loc = time.UTC
	}
	opt, err := rrule.StrToROptionInLocation(ev.RRule, loc)
	if err != nil {
		return nil, err
	}
	opt.Dtstart = ev.StartsAt.In(loc)
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil, err
	}
	set := &rrule.Set{}
	set.RRule(r)
	for _, x := range DecodeExDates(ev.ExDates) {
		set.ExDate(x.In(loc))
	}
	return set, nil
}

// Occurrences returns the starts of an event's occurrences that overlap
// [from, to): the event itself, or each occurrence of its series.
func Occurrences(ev models.Event, from, to time.Time) []time.Time {
	dur := ev.EndsAt.Sub(ev.StartsAt)
	set, err := (*rrule.Set)(nil), error(nil)
	if ev.RRule != "" {
		set, err = eventSet(ev)
	}
	if ev.RRule == "" || err != nil {
		// A single event (or a rule that no longer parses: its first one).
		if Overlaps(ev.StartsAt, ev.EndsAt, from, to) {
			return []time.Time{ev.StartsAt}
		}
		return nil
	}
	var out []time.Time
	it := set.Iterator()
	for {
		t, ok := it()
		if !ok || !t.Before(to) || len(out) >= maxExpand {
			break
		}
		if Overlaps(t, t.Add(dur), from, to) {
			out = append(out, t.UTC())
		}
	}
	return out
}

// Overlaps reports whether [start, end) meets [from, to). A zero-length
// entry counts when it starts inside the range.
func Overlaps(start, end, from, to time.Time) bool {
	if !start.Before(to) {
		return false
	}
	if end.Equal(start) {
		return !start.Before(from)
	}
	return end.After(from)
}

// SeriesUntil is the end of a series' last occurrence, for range queries
// (nil = it never ends). Non-repeating events end at EndsAt.
func SeriesUntil(ev models.Event) *time.Time {
	if ev.RRule == "" {
		end := ev.EndsAt
		return &end
	}
	set, err := eventSet(ev)
	if err != nil {
		end := ev.EndsAt
		return &end
	}
	opt, _ := rrule.StrToROption(ev.RRule)
	if opt == nil || (opt.Until.IsZero() && opt.Count == 0) {
		return nil
	}
	all := set.All()
	if len(all) == 0 {
		end := ev.EndsAt
		return &end
	}
	end := all[len(all)-1].Add(ev.EndsAt.Sub(ev.StartsAt)).UTC()
	return &end
}

// DecodeExDates / EncodeExDates convert an event's removed occurrences.
func DecodeExDates(s string) []time.Time {
	var raw []string
	if s == "" || json.Unmarshal([]byte(s), &raw) != nil {
		return nil
	}
	out := make([]time.Time, 0, len(raw))
	for _, r := range raw {
		if t, err := time.Parse(time.RFC3339, r); err == nil {
			out = append(out, t)
		}
	}
	return out
}

func EncodeExDates(ts []time.Time) string {
	raw := make([]string, len(ts))
	for i, t := range ts {
		raw[i] = t.UTC().Format(time.RFC3339)
	}
	b, _ := json.Marshal(raw)
	return string(b)
}

// DateLayout is the floating date of tasks.
const DateLayout = "2006-01-02"

// NextDue is the next date of a repeating task after due (both YYYY-MM-DD).
// It is counted from the due date, not from the day it was completed, like
// Apple Reminders. ok is false when the series has ended.
func NextDue(due, rule string) (next string, ok bool) {
	d, err := time.ParseInLocation(DateLayout, due, time.UTC)
	if err != nil {
		return "", false
	}
	opt, err := rrule.StrToROptionInLocation(rule, time.UTC)
	if err != nil {
		return "", false
	}
	opt.Dtstart = d
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return "", false
	}
	n := r.After(d, false)
	if n.IsZero() {
		return "", false
	}
	return n.Format(DateLayout), true
}

// ValidDate / ValidClock check a task's floating due date and time.
func ValidDate(s string) bool {
	_, err := time.Parse(DateLayout, s)
	return err == nil
}

func ValidClock(s string) bool {
	_, err := time.Parse("15:04", s)
	return err == nil && len(s) == 5
}

// DaysBetween is the number of days from date a to date b (both
// YYYY-MM-DD; 0 when either is invalid).
func DaysBetween(a, b string) int {
	da, err1 := time.Parse(DateLayout, a)
	db, err2 := time.Parse(DateLayout, b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(db.Sub(da).Hours() / 24)
}

// ShiftDate moves a date by n days.
func ShiftDate(d string, n int) (string, bool) {
	t, err := time.Parse(DateLayout, d)
	if err != nil {
		return "", false
	}
	return t.AddDate(0, 0, n).Format(DateLayout), true
}

// IsOccurrence reports whether at is the start of one of the event's
// occurrences (a removed one is not).
func IsOccurrence(ev models.Event, at time.Time) bool {
	for _, t := range Occurrences(ev, at.Add(-time.Second), at.Add(time.Second)) {
		if t.Equal(at) {
			return true
		}
	}
	return false
}

// SplitRule cuts a series at the occurrence starting at `at`: before is the
// rule of what comes earlier (it now ends just before at), after the rule of
// at and what follows (a COUNT keeps counting what was left). first is true
// when at is the series' first occurrence (nothing comes before: no split).
func SplitRule(ev models.Event, at time.Time) (before, after string, first bool, err error) {
	loc := location(ev.TZ)
	if ev.AllDay {
		loc = time.UTC
	}
	opt, err := rrule.StrToROptionInLocation(ev.RRule, loc)
	if err != nil {
		return "", "", false, err
	}
	opt.Dtstart = ev.StartsAt.In(loc)
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return "", "", false, err
	}
	// Occurrences of the rule (removed ones too: they count for COUNT) before at.
	earlier := r.Between(ev.StartsAt.In(loc), at.In(loc), true)
	n := 0
	for _, t := range earlier {
		if t.Before(at) {
			n++
		}
	}
	if n == 0 {
		return ev.RRule, ev.RRule, true, nil
	}
	var parts []string
	for _, p := range strings.Split(ev.RRule, ";") {
		k := strings.ToUpper(strings.SplitN(p, "=", 2)[0])
		if k != "COUNT" && k != "UNTIL" && strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	base := strings.Join(parts, ";")
	before = base + ";UNTIL=" + at.Add(-time.Second).UTC().Format("20060102T150405Z")
	after = ev.RRule
	if opt.Count > 0 {
		left := opt.Count - n
		if left <= 0 {
			return before, "", false, nil
		}
		after = base + ";COUNT=" + strconv.Itoa(left)
	}
	return before, after, false, nil
}

// SplitExDates parts removed occurrences at at: those before, those from it on.
func SplitExDates(s string, at time.Time) (before, after string) {
	var b, a []time.Time
	for _, x := range DecodeExDates(s) {
		if x.Before(at) {
			b = append(b, x)
		} else {
			a = append(a, x)
		}
	}
	return EncodeExDates(b), EncodeExDates(a)
}
