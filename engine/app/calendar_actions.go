package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"pp/core/envelope"
	"pp/engine/calendar"
)

// Event categories.
var eventCategories = map[string]bool{"": true, "school": true, "medical": true, "activity": true, "holiday": true, "handover": true,
	"other": true}

// EventInput is a new or edited event.
type EventInput struct {
	Space  string    `json:"space"`
	Item   string    `json:"item,omitempty"`   // set when editing
	Reason string    `json:"reason,omitempty"` // required when editing
	Data   EventData `json:"data"`
	Notes  string    `json:"notes,omitempty"`
}

func (d *EventData) check() error {
	d.Title = strings.TrimSpace(d.Title)
	switch {
	case d.Title == "" || utf8.RuneCountInString(d.Title) > 120:
		return errors.New("give the event a title (up to 120 characters)")
	case d.Start.IsZero():
		return errors.New("choose when it starts")
	case d.End.Before(d.Start):
		return errors.New("it can't end before it starts")
	case d.End.Sub(d.Start) > 62*24*time.Hour:
		return errors.New("an event can last two months at most")
	case !eventCategories[d.Category]:
		return errors.New("unknown category")
	case utf8.RuneCountInString(d.Location) > 200 || len(d.Children) > 20 || d.Remind < 0 || d.Remind > 10080:
		return errors.New("check the event's details")
	}
	if _, err := time.LoadLocation(d.Zone); err != nil {
		return errors.New("unknown time zone")
	}
	return d.Repeat.Validate()
}

// SaveEvent adds an event, or edits one of yours (with a reason, kept with its history).
func (e *Engine) SaveEvent(in EventInput) (string, error) {
	if err := in.Data.check(); err != nil {
		return "", err
	}
	raw, _ := json.Marshal(in.Data)
	body := envelope.Body{Text: strings.TrimSpace(in.Notes), Data: raw}
	if in.Item == "" {
		it, err := e.post(in.Space, envelope.Draft{Kind: envelope.KindEvent, Body: body})
		if err != nil {
			return "", err
		}
		return it.ItemID, nil
	}
	body.Reason = strings.TrimSpace(in.Reason)
	return in.Item, e.reviseKind(in.Space, in.Item, envelope.KindEvent, body)
}

// CancelEvent cancels one of your events (it stays visible, marked cancelled, with the reason).
func (e *Engine) CancelEvent(space, item, reason string) error {
	return e.reviseKind(space, item, envelope.KindEvent, envelope.Body{Retracted: true, Reason: strings.TrimSpace(reason)})
}

// reviseKind is revise for calendar items: your own, of the right kind, with a reason.
func (e *Engine) reviseKind(space, item, kind string, b envelope.Body) error {
	if b.Reason == "" {
		return errors.New("a reason is required - it's kept with the history")
	}
	if utf8.RuneCountInString(b.Reason) > envelope.MaxReason {
		return errors.New("the reason is too long")
	}
	cur, err := e.latest(space, item)
	if err != nil {
		return err
	}
	if cur.rec.Item.Kind != kind || cur.rec.Item.Ref != "" {
		return errors.New("that isn't something that can be changed here")
	}
	if cur.rec.Item.Person != e.person {
		return errors.New("only the person who made it can change it")
	}
	if cur.body != nil && cur.body.Retracted {
		return errors.New("it was withdrawn and can't be changed again")
	}
	_, err = e.post(space, envelope.Draft{Prev: &cur.rec.Item, Body: b})
	return err
}

// requireParent checks you're a parent in the family (schedules and changes are between parents).
func (e *Engine) requireParent(space string) (map[string]bool, error) {
	sv, err := e.spaceView(space)
	if err != nil {
		return nil, err
	}
	if sv.Me.Role != "parent" {
		return nil, errors.New("only parents can do that")
	}
	parents := map[string]bool{}
	for _, m := range sv.Members {
		if m.Role == "parent" && m.Status == "active" {
			parents[m.Person] = true
		}
	}
	return parents, nil
}

// ScheduleInput proposes a parenting-time schedule from a template (or a custom cycle).
type ScheduleInput struct {
	Space    string             `json:"space"`
	Item     string             `json:"item,omitempty"`   // set when changing your proposal
	Reason   string             `json:"reason,omitempty"` // required when changing
	Template string             `json:"template"`
	ParentA  string             `json:"parent_a"`
	ParentB  string             `json:"parent_b"`
	Start    string             `json:"start"`
	From     string             `json:"from,omitempty"` // takes effect (default: start)
	Exchange string             `json:"exchange"`
	Zone     string             `json:"zone"`
	Cycle    []calendar.Segment `json:"cycle,omitempty"` // custom only
	Note     string             `json:"note,omitempty"`
}

// ProposeSchedule proposes (or changes your proposal of) a schedule. It applies once the other parents agree.
func (e *Engine) ProposeSchedule(in ScheduleInput) (string, error) {
	parents, err := e.requireParent(in.Space)
	if err != nil {
		return "", err
	}
	var pat *calendar.Pattern
	if in.Template == "custom" {
		pat = &calendar.Pattern{Start: in.Start, Exchange: in.Exchange, Zone: in.Zone, Cycle: in.Cycle}
		err = pat.Validate()
	} else if tpl, ok := calendar.TemplateByID(in.Template); ok {
		pat, err = tpl.Build(in.ParentA, in.ParentB, in.Start, in.Exchange, in.Zone)
	} else {
		err = errors.New("choose a schedule")
	}
	if err != nil {
		return "", errors.New(strings.TrimPrefix(err.Error(), "calendar: "))
	}
	for _, s := range pat.Cycle {
		if !parents[s.Parent] {
			return "", errors.New("a schedule can only share time between the family's parents")
		}
	}
	from := in.From
	if from == "" {
		from = pat.Start
	}
	if _, err := time.Parse(calendar.DateLayout, from); err != nil {
		return "", errors.New("choose the date it starts")
	}
	raw, _ := json.Marshal(ScheduleData{Template: in.Template, From: from, Pattern: *pat})
	body := envelope.Body{Text: strings.TrimSpace(in.Note), Data: raw}
	if in.Item == "" {
		it, err := e.post(in.Space, envelope.Draft{Kind: envelope.KindSchedule, Body: body})
		if err != nil {
			return "", err
		}
		return it.ItemID, nil
	}
	body.Reason = strings.TrimSpace(in.Reason)
	return in.Item, e.reviseKind(in.Space, in.Item, envelope.KindSchedule, body)
}

// SwapInput asks for a change to parenting time.
type SwapInput struct {
	Space   string   `json:"space"`
	Item    string   `json:"item,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Changes []Change `json:"changes"`
	Note    string   `json:"note,omitempty"`
}

// RequestChange asks the other parents to change parenting time (for example a holiday, or swapping weekends).
func (e *Engine) RequestChange(in SwapInput) (string, error) {
	parents, err := e.requireParent(in.Space)
	if err != nil {
		return "", err
	}
	if len(in.Changes) == 0 || len(in.Changes) > 10 {
		return "", errors.New("add the dates you'd like to change")
	}
	for _, c := range in.Changes {
		if !c.To.After(c.From) {
			return "", errors.New("each change needs to end after it starts")
		}
		if c.To.Sub(c.From) > 120*24*time.Hour {
			return "", errors.New("a change can cover four months at most - for longer, propose a new schedule")
		}
		if !parents[c.Parent] {
			return "", errors.New("the children can only be with one of the family's parents")
		}
	}
	if strings.TrimSpace(in.Note) == "" {
		return "", errors.New("say why you're asking - it helps the other parent decide")
	}
	raw, _ := json.Marshal(SwapData{Changes: in.Changes})
	body := envelope.Body{Text: strings.TrimSpace(in.Note), Data: raw}
	if in.Item == "" {
		it, err := e.post(in.Space, envelope.Draft{Kind: envelope.KindSwap, Body: body})
		if err != nil {
			return "", err
		}
		return it.ItemID, nil
	}
	body.Reason = strings.TrimSpace(in.Reason)
	return in.Item, e.reviseKind(in.Space, in.Item, envelope.KindSwap, body)
}

// Withdraw withdraws your schedule proposal or change request (kept in the history with the reason).
func (e *Engine) Withdraw(space, item, reason string) error {
	cur, err := e.latest(space, item)
	if err != nil {
		return err
	}
	return e.reviseKind(space, item, cur.rec.Item.Kind, envelope.Body{Retracted: true, Reason: strings.TrimSpace(reason)})
}

// Answer accepts or declines exactly the current version of another parent's schedule or change request.
func (e *Engine) Answer(space, item string, accept bool, comment string) error {
	if _, err := e.requireParent(space); err != nil {
		return err
	}
	cur, err := e.latest(space, item)
	if err != nil {
		return err
	}
	it := &cur.rec.Item
	if (it.Kind != envelope.KindSchedule && it.Kind != envelope.KindSwap) || it.Ref != "" {
		return errors.New("that can't be answered")
	}
	if it.Person == e.person {
		return errors.New("you can't answer your own request")
	}
	if cur.body == nil || cur.body.Retracted {
		return errors.New("it was withdrawn")
	}
	comment = strings.TrimSpace(comment)
	if !accept && comment == "" {
		return errors.New("say why you're declining - it's kept with the request")
	}
	if utf8.RuneCountInString(comment) > envelope.MaxReason {
		return errors.New("the comment is too long")
	}
	h := it.Hash()
	ad := AnswerData{Answer: "decline", Rev: it.Rev, Hash: base64.StdEncoding.EncodeToString(h[:])}
	if accept {
		ad.Answer = "accept"
	}
	raw, _ := json.Marshal(ad)
	_, err = e.post(space, envelope.Draft{Kind: it.Kind, Ref: item, Body: envelope.Body{Text: comment, Data: raw}})
	return err
}

// CheckInInput records "I'm here".
type CheckInInput struct {
	Space    string   `json:"space"`
	Place    string   `json:"place,omitempty"`
	Lat      *float64 `json:"lat,omitempty"`
	Lon      *float64 `json:"lon,omitempty"`
	Accuracy float64  `json:"accuracy,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// CheckIn records a check-in: now, with the place and (when the device has it) the location.
func (e *Engine) CheckIn(in CheckInInput) error {
	if (in.Lat == nil) != (in.Lon == nil) || (in.Lat != nil && (*in.Lat < -90 || *in.Lat > 90 || *in.Lon < -180 || *in.Lon > 180)) {
		return errors.New("that location isn't valid")
	}
	if utf8.RuneCountInString(in.Place) > 200 || utf8.RuneCountInString(in.Note) > 2000 {
		return errors.New("that's too long")
	}
	raw, _ := json.Marshal(CheckInData{At: time.Now().UTC(), Place: strings.TrimSpace(in.Place), Lat: in.Lat, Lon: in.Lon, Accuracy: in.Accuracy})
	_, err := e.post(in.Space, envelope.Draft{Kind: envelope.KindCheckIn, Body: envelope.Body{Text: strings.TrimSpace(in.Note), Data: raw}})
	return err
}
