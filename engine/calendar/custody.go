package calendar

import (
	"sort"
	"time"
)

// Effective is an agreed pattern and the day it takes effect (it applies until the next one takes over).
type Effective struct {
	From    string  `json:"from"` // date
	Pattern Pattern `json:"pattern"`
}

// Override is an agreed change: from From until To, the children are with Parent. Later overrides win.
type Override struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Parent string    `json:"parent"`
	Source string    `json:"source"` // the change request it came from
}

// Handover is a moment care changes hands.
type Handover struct {
	Time   time.Time `json:"time"`
	From   string    `json:"from"`
	To     string    `json:"to"`
	Source string    `json:"source,omitempty"` // a change request, when that's why
}

// Day is one calendar day: who has the children as it starts and as it ends, and any handovers during it.
type Day struct {
	Date      string     `json:"date"`
	Start     string     `json:"start"` // parent at 00:00 ("" = no schedule)
	End       string     `json:"end"`   // parent at the end of the day
	Handovers []Handover `json:"handovers,omitempty"`
	Changed   bool       `json:"changed,omitempty"` // an agreed change applies on this day
}

// Plan combines the agreed patterns and changes into "who has the children when".
type Plan struct {
	Patterns  []Effective
	Overrides []Override
	Zone      *time.Location // the zone days are shown in
}

func (p *Plan) pattern(t time.Time) *Pattern {
	var found *Pattern
	sort.SliceStable(p.Patterns, func(i, j int) bool { return p.Patterns[i].From < p.Patterns[j].From })
	for i := range p.Patterns {
		e := &p.Patterns[i]
		from, err := time.ParseInLocation(DateLayout, e.From, e.Pattern.location())
		if err != nil || t.Before(from) {
			continue
		}
		found = &e.Pattern
	}
	return found
}

// ParentAt says who has the children at t, and which agreed change (if any) is why.
func (p *Plan) ParentAt(t time.Time) (parent, source string) {
	for i := len(p.Overrides) - 1; i >= 0; i-- {
		o := p.Overrides[i]
		if !t.Before(o.From) && t.Before(o.To) {
			return o.Parent, o.Source
		}
	}
	if pat := p.pattern(t); pat != nil {
		return pat.ParentAt(t), ""
	}
	return "", ""
}

// Days works out every day from first to last (inclusive dates).
func (p *Plan) Days(first, last time.Time) []Day {
	zone := p.Zone
	if zone == nil {
		zone = time.UTC
	}
	var out []Day
	for d := civil(first); !d.After(civil(last)); d = d.AddDate(0, 0, 1) {
		start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, zone)
		end := start.AddDate(0, 0, 1)
		day := Day{Date: d.Format(DateLayout)}
		day.Start, _ = p.ParentAt(start)
		day.End, _ = p.ParentAt(end.Add(-time.Second))

		// Every moment care could change hands today: pattern handovers, patterns taking effect, change edges.
		moments := map[time.Time]bool{}
		for _, e := range p.Patterns {
			moments[e.Pattern.HandoverOn(start.In(e.Pattern.location()))] = true
			if from, err := time.ParseInLocation(DateLayout, e.From, e.Pattern.location()); err == nil {
				moments[from] = true
			}
		}
		for _, o := range p.Overrides {
			moments[o.From], moments[o.To] = true, true
			if o.From.Before(end) && o.To.After(start) {
				day.Changed = true
			}
		}
		var ts []time.Time
		for t := range moments {
			if t.After(start) && t.Before(end) {
				ts = append(ts, t)
			}
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
		for _, t := range ts {
			before, _ := p.ParentAt(t.Add(-time.Second))
			after, src := p.ParentAt(t)
			if before != after && after != "" {
				day.Handovers = append(day.Handovers, Handover{Time: t, From: before, To: after, Source: src})
			}
		}
		out = append(out, day)
	}
	return out
}
