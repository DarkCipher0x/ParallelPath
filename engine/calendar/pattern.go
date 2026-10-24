// Package calendar is the arithmetic behind ParallelPath's calendar: parenting-time patterns and their templates,
// who has the children at any moment (patterns plus agreed changes), and repeating events. It has no I/O and knows
// nothing about encryption; the engine stores everything as signed, encrypted items and asks this package to read
// them.
package calendar

import (
	"errors"
	"fmt"
	"time"

	_ "time/tzdata" // time zones work the same on Windows, Android and Linux
)

// DateLayout is how days are written everywhere: 2026-10-24.
const DateLayout = "2006-01-02"

// Segment is a run of consecutive days with one parent.
type Segment struct {
	Parent string `json:"parent"` // person ID
	Days   int    `json:"days"`
}

// Pattern is a repeating parenting-time schedule. Care changes hands at Exchange (local time in Zone) on the first
// day of each segment; the cycle repeats from Start for ever.
type Pattern struct {
	Start    string    `json:"start"`    // first day of the cycle (a date)
	Exchange string    `json:"exchange"` // "18:00"
	Zone     string    `json:"zone"`     // IANA time zone, e.g. Europe/London
	Cycle    []Segment `json:"cycle"`
}

// Validate checks a pattern can be used.
func (p *Pattern) Validate() error {
	if _, err := time.Parse(DateLayout, p.Start); err != nil {
		return errors.New("calendar: the start date isn't a date")
	}
	if _, _, err := clock(p.Exchange); err != nil {
		return err
	}
	if _, err := time.LoadLocation(p.Zone); err != nil {
		return fmt.Errorf("calendar: unknown time zone %q", p.Zone)
	}
	if len(p.Cycle) == 0 || len(p.Cycle) > 56 {
		return errors.New("calendar: a schedule needs between 1 and 56 parts")
	}
	total := 0
	for _, s := range p.Cycle {
		if s.Parent == "" || s.Days < 1 || s.Days > 366 {
			return errors.New("calendar: every part needs a parent and 1-366 days")
		}
		total += s.Days
	}
	if total > 366 {
		return errors.New("calendar: a schedule repeats within a year at most")
	}
	return nil
}

// clock parses "18:00".
func clock(s string) (h, m int, err error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, errors.New("calendar: the handover time isn't a time like 18:00")
	}
	return t.Hour(), t.Minute(), nil
}

func (p *Pattern) location() *time.Location {
	loc, err := time.LoadLocation(p.Zone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ParentAt says whose care the children are in at t, or "" before the pattern starts.
func (p *Pattern) ParentAt(t time.Time) string {
	start, err := time.Parse(DateLayout, p.Start)
	if err != nil || len(p.Cycle) == 0 {
		return ""
	}
	h, m, err := clock(p.Exchange)
	if err != nil {
		return ""
	}
	local := t.In(p.location())
	// The "care day" starts at the handover time: before it, it's still the previous day's care.
	day := civil(local)
	if local.Hour()*60+local.Minute() < h*60+m {
		day = day.AddDate(0, 0, -1)
	}
	n := daysBetween(start, day)
	if n < 0 {
		return ""
	}
	total := 0
	for _, s := range p.Cycle {
		total += s.Days
	}
	n %= total
	for _, s := range p.Cycle {
		if n < s.Days {
			return s.Parent
		}
		n -= s.Days
	}
	return ""
}

// HandoverOn returns the instant of the handover time on a local date.
func (p *Pattern) HandoverOn(date time.Time) time.Time {
	h, m, _ := clock(p.Exchange)
	return time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, p.location())
}

// civil is the local calendar date of t, as midnight UTC (so dates compare and subtract exactly).
func civil(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours()/24 + 0.5*sign(b.Sub(a)))
}

func sign(d time.Duration) float64 {
	if d < 0 {
		return -1
	}
	return 1
}
