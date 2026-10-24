package calendar

import (
	"errors"
	"time"
)

// Repeat frequencies for events.
const (
	Never       = ""
	Daily       = "daily"
	Weekly      = "weekly"
	Fortnightly = "fortnightly"
	Monthly     = "monthly" // same day of the month (months without that day are skipped)
	Yearly      = "yearly"
)

// Repeat says how an event repeats.
type Repeat struct {
	Every string `json:"every,omitempty"`
	Until string `json:"until,omitempty"` // last date it can occur (inclusive); empty = for ever
}

// Validate checks a repeat rule.
func (r Repeat) Validate() error {
	switch r.Every {
	case Never, Daily, Weekly, Fortnightly, Monthly, Yearly:
	default:
		return errors.New("calendar: unknown repeat")
	}
	if r.Until != "" {
		if _, err := time.Parse(DateLayout, r.Until); err != nil {
			return errors.New("calendar: the repeat end isn't a date")
		}
	}
	return nil
}

// Occurrences lists the start times of an event that begins at start and repeats by r, for occurrences that
// overlap [from, to). length is how long each occurrence lasts.
func Occurrences(start time.Time, length time.Duration, r Repeat, from, to time.Time) []time.Time {
	var until time.Time
	if r.Until != "" {
		if u, err := time.ParseInLocation(DateLayout, r.Until, start.Location()); err == nil {
			until = u.AddDate(0, 0, 1)
		}
	}
	var out []time.Time
	for n := 0; n < 5000; n++ {
		var t time.Time
		switch r.Every {
		case Never:
			if n > 0 {
				return out
			}
			t = start
		case Daily:
			t = start.AddDate(0, 0, n)
		case Weekly:
			t = start.AddDate(0, 0, 7*n)
		case Fortnightly:
			t = start.AddDate(0, 0, 14*n)
		case Monthly:
			t = start.AddDate(0, n, 0)
			if t.Day() != start.Day() { // e.g. the 31st in a 30-day month
				continue
			}
		case Yearly:
			t = start.AddDate(n, 0, 0)
			if t.Day() != start.Day() { // 29 February
				continue
			}
		default:
			return out
		}
		if !t.Before(to) || (!until.IsZero() && !t.Before(until)) {
			return out
		}
		if t.Add(length).After(from) {
			out = append(out, t)
		}
	}
	return out
}
