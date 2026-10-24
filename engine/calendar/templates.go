package calendar

import (
	"errors"
	"time"
)

// Template is a common parenting-time arrangement, for two parents ("A" and "B").
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Monday: the pattern only makes sense starting on a Monday (the chosen start moves back to that week's Monday).
	Monday bool `json:"monday"`
	cycle  []Segment
}

const a, b = "A", "B"

// Templates are the arrangements offered when setting up a schedule (as family courts and mediators commonly use).
var Templates = []Template{
	{ID: "alternating-weeks", Name: "Alternating weeks", Description: "A week with each parent, changing over on the same day each week.",
		cycle: []Segment{{a, 7}, {b, 7}}},
	{ID: "2-2-3", Name: "2-2-3", Description: "Two days, two days, three days - so each parent has every other weekend. Repeats every two weeks.",
		Monday: true, cycle: []Segment{{a, 2}, {b, 2}, {a, 3}, {b, 2}, {a, 2}, {b, 3}}},
	{ID: "2-2-5-5", Name: "2-2-5-5", Description: "Each parent has the same two weekdays every week and alternate long weekends.",
		Monday: true, cycle: []Segment{{a, 2}, {b, 2}, {a, 5}, {b, 5}}},
	{ID: "3-4-4-3", Name: "3-4-4-3", Description: "Three days then four, swapping each week. Repeats every two weeks.",
		Monday: true, cycle: []Segment{{a, 3}, {b, 4}, {a, 4}, {b, 3}}},
	{ID: "every-other-weekend", Name: "Every other weekend", Description: "Parent A has the weekdays; parent B every other weekend, Friday handover to Sunday.",
		Monday: true, cycle: []Segment{{a, 4}, {b, 2}, {a, 8}}},
	{ID: "two-weeks", Name: "Two weeks each", Description: "Two weeks with each parent.",
		cycle: []Segment{{a, 14}, {b, 14}}},
}

// TemplateByID finds a template.
func TemplateByID(id string) (*Template, bool) {
	for i := range Templates {
		if Templates[i].ID == id {
			return &Templates[i], true
		}
	}
	return nil, false
}

// Build makes a pattern from a template for two parents, starting on (or, for Monday-based templates, in the week
// of) start. For "every other weekend" the handover is at the given time on Friday and Sunday.
func (t *Template) Build(parentA, parentB, start, exchange, zone string) (*Pattern, error) {
	if parentA == "" || parentB == "" || parentA == parentB {
		return nil, errors.New("calendar: choose two different parents")
	}
	day, err := time.Parse(DateLayout, start)
	if err != nil {
		return nil, errors.New("calendar: the start date isn't a date")
	}
	if t.Monday {
		day = day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	}
	p := &Pattern{Start: day.Format(DateLayout), Exchange: exchange, Zone: zone}
	for _, s := range t.cycle {
		who := parentA
		if s.Parent == b {
			who = parentB
		}
		p.Cycle = append(p.Cycle, Segment{Parent: who, Days: s.Days})
	}
	return p, p.Validate()
}
