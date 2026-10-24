package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"pp/core/envelope"
	"pp/engine/calendar"
)

// The calendar is made of ordinary family items (end-to-end encrypted, signed, never deleted):
//
//   - event     (kind "event"):    something on a date - school, medical, an activity - with optional repeats
//   - schedule  (kind "schedule"): a proposed parenting-time pattern; it applies once every other parent agrees
//   - change    (kind "swap"):     a request to change parenting time on given dates; applies once agreed
//   - answer    (same kind, Ref):  a parent's yes or no to one exact revision of a schedule or change
//   - check-in  (kind "checkin"):  "I'm here" at a handover, with the time and (on phones) the location
//
// Editing a proposal makes a new revision, so earlier answers no longer count: the other parents have to agree to
// exactly what's being asked. Everything stays in the history and the audit log.

// EventData is an event's content.
type EventData struct {
	Title    string          `json:"title"`
	Start    time.Time       `json:"start"`
	End      time.Time       `json:"end"`
	AllDay   bool            `json:"all_day,omitempty"`
	Zone     string          `json:"zone"`
	Location string          `json:"location,omitempty"`
	Category string          `json:"category,omitempty"` // school, medical, activity, holiday, handover, other
	Children []string        `json:"children,omitempty"`
	Repeat   calendar.Repeat `json:"repeat,omitempty"`
	Remind   int             `json:"remind,omitempty"` // minutes before (shown in the app)
}

// ScheduleData is a proposed parenting-time pattern.
type ScheduleData struct {
	Template string           `json:"template"` // a calendar.Templates ID, or "custom"
	From     string           `json:"from"`     // the date it takes effect
	Pattern  calendar.Pattern `json:"pattern"`
}

// Change is one part of a change request: from From until To, the children are with Parent.
type Change struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Parent string    `json:"parent"`
}

// SwapData is a change request.
type SwapData struct {
	Changes []Change `json:"changes"`
}

// AnswerData is a parent's answer to one exact revision of a schedule or change request.
type AnswerData struct {
	Answer string `json:"answer"` // accept | decline
	Rev    uint64 `json:"rev"`
	Hash   string `json:"hash"` // base64 of that revision's hash
}

// CheckInData is a check-in.
type CheckInData struct {
	At       time.Time `json:"at"`
	Place    string    `json:"place,omitempty"`
	Lat      *float64  `json:"lat,omitempty"`
	Lon      *float64  `json:"lon,omitempty"`
	Accuracy float64   `json:"accuracy,omitempty"` // metres
}

// ---- views the apps show

// Person is someone named in the calendar.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Me   bool   `json:"me"`
}

// Answer is a parent's answer, as shown.
type Answer struct {
	Person  string    `json:"person"`
	Name    string    `json:"name"`
	Answer  string    `json:"answer"`
	Comment string    `json:"comment,omitempty"`
	Time    time.Time `json:"time"`
}

// Proposal is a schedule or change request and where it stands.
type Proposal struct {
	ID         string        `json:"id"`
	Kind       string        `json:"kind"` // schedule | change
	Rev        uint64        `json:"rev"`
	Author     string        `json:"author"`
	AuthorName string        `json:"author_name"`
	Mine       bool          `json:"mine"`
	Created    time.Time     `json:"created"`
	Updated    time.Time     `json:"updated"`
	Note       string        `json:"note,omitempty"`
	Reason     string        `json:"reason,omitempty"` // why it was last changed or withdrawn
	State      string        `json:"state"`            // pending | agreed | declined | withdrawn
	Agreed     time.Time     `json:"agreed,omitempty"`
	Waiting    []string      `json:"waiting,omitempty"` // names of parents yet to answer
	NeedsMe    bool          `json:"needs_me"`          // I'm a parent who hasn't answered
	Answers    []Answer      `json:"answers"`
	History    []Revision    `json:"history"`
	Schedule   *ScheduleData `json:"schedule,omitempty"`
	Changes    []Change      `json:"changes,omitempty"`
	Problem    string        `json:"problem,omitempty"`
}

// Event is one occurrence of an event in the range asked for.
type Event struct {
	ID         string     `json:"id"`
	Rev        uint64     `json:"rev"`
	Start      time.Time  `json:"start"` // this occurrence
	End        time.Time  `json:"end"`
	Data       EventData  `json:"data"`
	Notes      string     `json:"notes,omitempty"`
	Author     string     `json:"author"`
	AuthorName string     `json:"author_name"`
	Mine       bool       `json:"mine"`
	Edited     bool       `json:"edited"`
	Cancelled  bool       `json:"cancelled"`
	Reason     string     `json:"reason,omitempty"`
	History    []Revision `json:"history"`
}

// CheckIn is a check-in as shown.
type CheckIn struct {
	ID       string      `json:"id"`
	Person   string      `json:"person"`
	Name     string      `json:"name"`
	Mine     bool        `json:"mine"`
	Data     CheckInData `json:"data"`
	Note     string      `json:"note,omitempty"`
	Received time.Time   `json:"received"` // when the server got it (signed, in the audit log)
}

// CalendarView is everything for a date range.
type CalendarView struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Parents   []Person       `json:"parents"`
	Days      []calendar.Day `json:"days"`
	Events    []Event        `json:"events"`
	CheckIns  []CheckIn      `json:"checkins"`
	Schedules []Proposal     `json:"schedules"`
	Changes   []Proposal     `json:"changes"`
	NeedsMe   int            `json:"needs_me"` // proposals waiting for my answer
}

// thread collects an item's revisions and the answers that refer to it.
type thread struct {
	revs    []*decoded
	answers []*decoded
}

func (t *thread) latest() *decoded { return t.revs[len(t.revs)-1] }

// Calendar works out the calendar from first to last (dates) in the given time zone (the device's own).
func (e *Engine) Calendar(space, first, last, zone string) (*CalendarView, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, errors.New("unknown time zone")
	}
	from, err1 := time.ParseInLocation(calendar.DateLayout, first, loc)
	to, err2 := time.ParseInLocation(calendar.DateLayout, last, loc)
	if err1 != nil || err2 != nil || to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		return nil, errors.New("choose a date range of up to a year")
	}
	sv, err := e.spaceView(space)
	if err != nil {
		return nil, err
	}
	all, err := e.loadDecoded(space)
	if err != nil {
		return nil, err
	}
	names := e.names(all)
	parents := map[string]bool{}
	view := &CalendarView{From: first, To: last, Days: []calendar.Day{}, Events: []Event{}, CheckIns: []CheckIn{}, Schedules: []Proposal{},
		Changes: []Proposal{}}
	for _, m := range sv.Members {
		if m.Role == "parent" && m.Status == "active" {
			parents[m.Person] = true
			view.Parents = append(view.Parents, Person{ID: m.Person, Name: names.of(m.Person), Me: m.Person == e.person})
		}
	}
	sort.Slice(view.Parents, func(i, j int) bool { return view.Parents[i].Name < view.Parents[j].Name })

	threads := map[string]*thread{}
	var order []string
	var checkins []*decoded
	for _, d := range all {
		it := &d.rec.Item
		switch it.Kind {
		case envelope.KindEvent, envelope.KindSchedule, envelope.KindSwap:
		case envelope.KindCheckIn:
			checkins = append(checkins, d)
			continue
		default:
			continue
		}
		if it.Ref != "" && it.Kind != envelope.KindEvent { // an answer
			t := threads[it.Ref]
			if t == nil {
				t = &thread{}
				threads[it.Ref] = t
			}
			t.answers = append(t.answers, d)
			continue
		}
		t := threads[it.ItemID]
		if t == nil {
			t = &thread{}
			threads[it.ItemID] = t
		}
		if len(t.revs) == 0 {
			order = append(order, it.ItemID)
		}
		t.revs = append(t.revs, d)
	}

	plan := &calendar.Plan{Zone: loc}
	var agreedChanges []Proposal
	for _, id := range order {
		t := threads[id]
		if len(t.revs) == 0 {
			continue
		}
		switch t.latest().rec.Item.Kind {
		case envelope.KindEvent:
			view.Events = append(view.Events, e.eventOccurrences(t, names, from, to.AddDate(0, 0, 1))...)
		case envelope.KindSchedule:
			p := e.proposal(t, "schedule", names, parents)
			view.Schedules = append(view.Schedules, p)
			if p.State == "agreed" && p.Schedule != nil {
				plan.Patterns = append(plan.Patterns, calendar.Effective{From: p.Schedule.From, Pattern: p.Schedule.Pattern})
			}
		case envelope.KindSwap:
			p := e.proposal(t, "change", names, parents)
			view.Changes = append(view.Changes, p)
			if p.State == "agreed" {
				agreedChanges = append(agreedChanges, p)
			}
		}
	}
	// Later agreements win where changes overlap.
	sort.SliceStable(agreedChanges, func(i, j int) bool { return agreedChanges[i].Agreed.Before(agreedChanges[j].Agreed) })
	for _, p := range agreedChanges {
		for _, c := range p.Changes {
			plan.Overrides = append(plan.Overrides, calendar.Override{From: c.From, To: c.To, Parent: c.Parent, Source: p.ID})
		}
	}
	view.Days = plan.Days(from, to)
	sort.Slice(view.Events, func(i, j int) bool { return view.Events[i].Start.Before(view.Events[j].Start) })
	for _, list := range [][]Proposal{view.Schedules, view.Changes} {
		for _, p := range list {
			if p.NeedsMe {
				view.NeedsMe++
			}
		}
	}

	for _, d := range checkins {
		if d.body == nil || d.body.Retracted {
			continue
		}
		var c CheckInData
		if json.Unmarshal(d.body.Data, &c) != nil || c.At.Before(from) || !c.At.Before(to.AddDate(0, 0, 1)) {
			continue
		}
		it := &d.rec.Item
		view.CheckIns = append(view.CheckIns, CheckIn{ID: it.ItemID, Person: it.Person, Name: names.of(it.Person), Mine: it.Person == e.person,
			Data: c, Note: d.body.Text, Received: it.Created})
	}
	return view, nil
}

func history(t *thread) []Revision {
	var h []Revision
	for _, d := range t.revs {
		if d.body != nil {
			h = append(h, Revision{Rev: d.rec.Item.Rev, Time: d.rec.Item.Created, Text: d.body.Text, Reason: d.body.Reason, Retracted: d.body.Retracted})
		}
	}
	return h
}

func (e *Engine) eventOccurrences(t *thread, names nameBook, from, to time.Time) []Event {
	cur := t.latest()
	if cur.body == nil {
		return nil
	}
	var data EventData
	if cur.body.Retracted { // a cancelled event keeps showing, marked, at its last known time
		for i := len(t.revs) - 1; i >= 0; i-- {
			if b := t.revs[i].body; b != nil && !b.Retracted && json.Unmarshal(b.Data, &data) == nil {
				break
			}
		}
	} else if json.Unmarshal(cur.body.Data, &data) != nil {
		return nil
	}
	if data.Start.IsZero() {
		return nil
	}
	it := &cur.rec.Item
	first := t.revs[0].rec.Item
	length := data.End.Sub(data.Start)
	if length < 0 {
		length = 0
	}
	start := data.Start
	if loc, err := time.LoadLocation(data.Zone); err == nil {
		start = start.In(loc) // repeats follow the event's own local time, across clock changes
	}
	var out []Event
	for _, at := range calendar.Occurrences(start, length, data.Repeat, from, to) {
		out = append(out, Event{ID: it.ItemID, Rev: it.Rev, Start: at, End: at.Add(length), Data: data, Notes: cur.body.Text,
			Author: first.Person, AuthorName: names.of(first.Person), Mine: first.Person == e.person, Edited: it.Rev > 0 && !cur.body.Retracted,
			Cancelled: cur.body.Retracted, Reason: cur.body.Reason, History: history(t)})
	}
	return out
}

// proposal works out where a schedule or change request stands from its answers.
func (e *Engine) proposal(t *thread, kind string, names nameBook, parents map[string]bool) Proposal {
	first := t.revs[0].rec.Item
	cur := t.latest()
	it := &cur.rec.Item
	p := Proposal{ID: it.ItemID, Kind: kind, Rev: it.Rev, Author: first.Person, AuthorName: names.of(first.Person), Mine: first.Person == e.person,
		Created: first.Created, Updated: it.Created, History: history(t), Answers: []Answer{}}
	if cur.body == nil {
		p.State, p.Problem = "pending", "can't be read on this device"
		return p
	}
	p.Note, p.Reason = cur.body.Text, cur.body.Reason
	if !parents[first.Person] {
		p.State, p.Problem = "declined", "made by someone who isn't a parent in this family"
		return p
	}
	if cur.body.Retracted {
		p.State = "withdrawn"
		for i := len(t.revs) - 1; i >= 0; i-- { // keep showing what was asked
			if b := t.revs[i].body; b != nil && !b.Retracted {
				e.fillProposal(&p, kind, b.Data)
				break
			}
		}
		return p
	}
	if err := e.fillProposal(&p, kind, cur.body.Data); err != nil {
		p.State, p.Problem = "declined", err.Error()
		return p
	}

	// The latest answer of each other parent to exactly this revision.
	h := it.Hash()
	want := base64.StdEncoding.EncodeToString(h[:])
	latest := map[string]*decoded{}
	var latestData = map[string]AnswerData{}
	for _, d := range t.answers {
		a := &d.rec.Item
		if d.body == nil || d.body.Retracted || !parents[a.Person] || a.Person == first.Person {
			continue
		}
		var ad AnswerData
		if json.Unmarshal(d.body.Data, &ad) != nil || (ad.Answer != "accept" && ad.Answer != "decline") {
			continue
		}
		p.Answers = append(p.Answers, Answer{Person: a.Person, Name: names.of(a.Person), Answer: ad.Answer, Comment: d.body.Text, Time: a.Created})
		if ad.Rev != it.Rev || ad.Hash != want {
			continue // an answer to an earlier version
		}
		if prev := latest[a.Person]; prev == nil || a.Created.After(prev.rec.Item.Created) {
			latest[a.Person], latestData[a.Person] = d, ad
		}
	}
	p.State = "agreed"
	for person := range parents {
		if person == first.Person {
			continue
		}
		d, ok := latest[person]
		switch {
		case !ok:
			p.State = "pending"
			p.Waiting = append(p.Waiting, names.of(person))
			if person == e.person {
				p.NeedsMe = true
			}
		case latestData[person].Answer == "decline":
			p.State = "declined"
		default:
			if d.rec.Item.Created.After(p.Agreed) {
				p.Agreed = d.rec.Item.Created
			}
		}
	}
	if p.State == "declined" {
		p.Waiting, p.NeedsMe, p.Agreed = nil, false, time.Time{}
	}
	if p.State == "pending" {
		p.Agreed = time.Time{}
	}
	if len(parents) < 2 && p.State == "agreed" { // a family with one parent so far: nothing to agree with yet
		p.Agreed = it.Created
	}
	sort.Strings(p.Waiting)
	return p
}

func (e *Engine) fillProposal(p *Proposal, kind string, raw json.RawMessage) error {
	if kind == "schedule" {
		var s ScheduleData
		if json.Unmarshal(raw, &s) != nil {
			return errors.New("unreadable schedule")
		}
		if err := s.Pattern.Validate(); err != nil {
			return err
		}
		p.Schedule = &s
		return nil
	}
	var s SwapData
	if json.Unmarshal(raw, &s) != nil || len(s.Changes) == 0 {
		return errors.New("unreadable change request")
	}
	p.Changes = s.Changes
	return nil
}
