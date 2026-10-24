package calendar

import (
	"testing"
	"time"
)

const london = "Europe/London"

func at(t *testing.T, s string) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation(london)
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func weeks(t *testing.T) *Pattern {
	t.Helper()
	tpl, _ := TemplateByID("alternating-weeks")
	p, err := tpl.Build("sam", "alex", "2026-11-06", "18:00", london) // a Friday
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAlternatingWeeks(t *testing.T) {
	p := weeks(t)
	cases := map[string]string{
		"2026-11-06 17:59": "",     // before the schedule starts
		"2026-11-06 18:00": "sam",  // first handover
		"2026-11-13 17:59": "sam",  // a week later, just before the handover
		"2026-11-13 18:00": "alex", // handover
		"2026-11-20 18:00": "sam",  // and back
		"2027-04-02 17:59": "sam",  // after the clocks went forward (28 March 2027), the handover
		"2027-04-02 18:00": "alex", // still happens at 18:00 local time
	}
	for when, want := range cases {
		if got := p.ParentAt(at(t, when)); got != want {
			t.Errorf("%s: got %q, want %q", when, got, want)
		}
	}
}

func TestTemplatesAreBalanced(t *testing.T) {
	for _, tpl := range Templates {
		p, err := tpl.Build("a", "b", "2026-11-04", "09:00", london) // a Wednesday
		if err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		if tpl.Monday && p.Start != "2026-11-02" {
			t.Errorf("%s should start on the Monday, got %s", tpl.ID, p.Start)
		}
		days := map[string]int{}
		for _, s := range p.Cycle {
			days[s.Parent] += s.Days
		}
		if tpl.ID != "every-other-weekend" && days["a"] != days["b"] {
			t.Errorf("%s: %d/%d days", tpl.ID, days["a"], days["b"])
		}
	}
	// 2-2-3: each parent has alternate weekends.
	tpl, _ := TemplateByID("2-2-3")
	p, _ := tpl.Build("a", "b", "2026-11-02", "09:00", london)
	if p.ParentAt(at(t, "2026-11-07 12:00")) != "a" || p.ParentAt(at(t, "2026-11-14 12:00")) != "b" {
		t.Error("2-2-3 weekends don't alternate")
	}
	// Every other weekend: b from Friday to Sunday, every other week only.
	tpl, _ = TemplateByID("every-other-weekend")
	p, _ = tpl.Build("a", "b", "2026-11-02", "18:00", london)
	for when, want := range map[string]string{
		"2026-11-06 17:00": "a", "2026-11-06 19:00": "b", "2026-11-08 17:00": "b", "2026-11-08 19:00": "a",
		"2026-11-14 12:00": "a", "2026-11-21 12:00": "b",
	} {
		if got := p.ParentAt(at(t, when)); got != want {
			t.Errorf("every other weekend %s: got %q, want %q", when, got, want)
		}
	}
	if _, err := tpl.Build("a", "a", "2026-11-02", "18:00", london); err == nil {
		t.Error("the same parent twice was accepted")
	}
}

func TestPlanWithChangeAndNewPattern(t *testing.T) {
	p := weeks(t)
	plan := &Plan{Patterns: []Effective{{From: "2026-11-06", Pattern: *p}}, Zone: p.location()}
	// Agreed change: Alex has the children over a weekend that's Sam's.
	plan.Overrides = []Override{{From: at(t, "2026-11-07 10:00"), To: at(t, "2026-11-08 17:00"), Parent: "alex", Source: "req1"}}
	days := plan.Days(at(t, "2026-11-06 00:00"), at(t, "2026-11-14 00:00"))
	if len(days) != 9 {
		t.Fatalf("got %d days", len(days))
	}
	byDate := map[string]Day{}
	for _, d := range days {
		byDate[d.Date] = d
	}
	if d := byDate["2026-11-06"]; d.Start != "" || d.End != "sam" || len(d.Handovers) != 1 || d.Handovers[0].To != "sam" {
		t.Errorf("first day: %+v", d)
	}
	if d := byDate["2026-11-07"]; !d.Changed || len(d.Handovers) != 1 || d.Handovers[0].To != "alex" || d.Handovers[0].Source != "req1" {
		t.Errorf("change day: %+v", d)
	}
	if d := byDate["2026-11-08"]; len(d.Handovers) != 1 || d.Handovers[0].To != "sam" || d.End != "sam" {
		t.Errorf("change ends: %+v", d)
	}
	if d := byDate["2026-11-13"]; len(d.Handovers) != 1 || d.Handovers[0].From != "sam" || d.Handovers[0].To != "alex" {
		t.Errorf("weekly handover: %+v", d)
	}
	if d := byDate["2026-11-10"]; len(d.Handovers) != 0 || d.Start != "sam" || d.End != "sam" {
		t.Errorf("ordinary day: %+v", d)
	}

	// A newly agreed pattern takes over from its start date.
	tpl, _ := TemplateByID("2-2-5-5")
	p2, _ := tpl.Build("alex", "sam", "2026-11-16", "09:00", london)
	plan.Patterns = append(plan.Patterns, Effective{From: "2026-11-16", Pattern: *p2})
	if got, _ := plan.ParentAt(at(t, "2026-11-16 12:00")); got != "alex" {
		t.Errorf("new pattern not in effect: %q", got)
	}
	if got, _ := plan.ParentAt(at(t, "2026-11-15 12:00")); got != "alex" {
		t.Errorf("old pattern before the switch: %q", got) // alternating weeks: 13 Nov 18:00 onwards is Alex
	}
}

func TestValidate(t *testing.T) {
	bad := []Pattern{
		{Start: "06/11/2026", Exchange: "18:00", Zone: london, Cycle: []Segment{{"a", 7}}},
		{Start: "2026-11-06", Exchange: "6pm", Zone: london, Cycle: []Segment{{"a", 7}}},
		{Start: "2026-11-06", Exchange: "18:00", Zone: "Mars/Olympus", Cycle: []Segment{{"a", 7}}},
		{Start: "2026-11-06", Exchange: "18:00", Zone: london},
		{Start: "2026-11-06", Exchange: "18:00", Zone: london, Cycle: []Segment{{"", 7}}},
		{Start: "2026-11-06", Exchange: "18:00", Zone: london, Cycle: []Segment{{"a", 0}}},
		{Start: "2026-11-06", Exchange: "18:00", Zone: london, Cycle: []Segment{{"a", 200}, {"b", 200}}},
	}
	for i, p := range bad {
		if p.Validate() == nil {
			t.Errorf("bad pattern %d accepted", i)
		}
	}
}

func TestOccurrences(t *testing.T) {
	start := at(t, "2026-01-31 15:00")
	from, to := at(t, "2026-01-01 00:00"), at(t, "2026-07-01 00:00")
	got := Occurrences(start, time.Hour, Repeat{Every: Monthly}, from, to)
	want := []string{"2026-01-31", "2026-03-31", "2026-05-31"} // months without a 31st are skipped
	if len(got) != len(want) {
		t.Fatalf("monthly: %v", got)
	}
	for i := range want {
		if got[i].Format(DateLayout) != want[i] {
			t.Errorf("monthly %d: %s", i, got[i])
		}
	}
	w := Occurrences(at(t, "2026-11-02 08:30"), 30*time.Minute, Repeat{Every: Weekly, Until: "2026-11-23"},
		at(t, "2026-11-10 00:00"), at(t, "2026-12-31 00:00"))
	if len(w) != 2 || w[0].Format(DateLayout) != "2026-11-16" || w[1].Format(DateLayout) != "2026-11-23" {
		t.Errorf("weekly with until: %v", w)
	}
	if len(Occurrences(start, time.Hour, Repeat{}, from, to)) != 1 {
		t.Error("a one-off event")
	}
	// An event that started before the window but runs into it still shows.
	long := Occurrences(at(t, "2026-10-31 10:00"), 72*time.Hour, Repeat{}, at(t, "2026-11-01 00:00"), at(t, "2026-11-02 00:00"))
	if len(long) != 1 {
		t.Error("an event overlapping the window")
	}
	if (Repeat{Every: "hourly"}).Validate() == nil || (Repeat{Every: Weekly, Until: "soon"}).Validate() == nil {
		t.Error("bad repeat accepted")
	}
}
