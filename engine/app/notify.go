package app

import (
	"encoding/json"
	"fmt"
	"time"

	"pp/core/envelope"
	"pp/engine/vault"
)

// Notice is something to tell the person about outside the screen they're on: the app shows it as a system
// notification (event "notify"). Preview is the message text itself; apps only show it when the person has chosen
// to (Settings), because notifications can show on a locked screen.
type Notice struct {
	Space   string    `json:"space"`
	Family  string    `json:"family"`
	Kind    string    `json:"kind"` // message | calendar | expense | document | info | checkin | reminder
	Title   string    `json:"title"`
	Body    string    `json:"body"`              // never contains content: safe for a locked screen
	Preview string    `json:"preview,omitempty"` // the content, for people who turned previews on
	Item    string    `json:"item,omitempty"`
	Time    time.Time `json:"time"`
}

// notifyNew turns items that just arrived from other people into notices. Items fetched by a device's first sync
// are history, not news, and aren't announced.
func (e *Engine) notifyNew(space string, recs []itemRec) {
	if len(recs) == 0 {
		return
	}
	fresh := map[uint64]bool{}
	for _, r := range recs {
		if r.Item.Person != e.person {
			fresh[r.Seq] = true
		}
	}
	if len(fresh) == 0 {
		return
	}
	all, err := e.loadDecoded(space)
	if err != nil {
		return
	}
	names := e.namesFor(space, all)
	if names.journal {
		return
	}
	family := names.space
	if family == "" {
		family = "Family space"
	}
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v != nil && e.familyOf(v, space) != "" {
		family += " · private"
	}
	for _, d := range all {
		if !fresh[d.rec.Seq] || d.body == nil {
			continue
		}
		n := describeNew(d, names.of(d.rec.Item.Person))
		if n == nil {
			continue
		}
		n.Space, n.Family, n.Item, n.Time = space, family, d.rec.Item.ItemID, d.rec.Item.Created
		e.listen("notify", n)
	}
}

// describeNew says what a new item is, in words fit for a notification (nil: not worth one).
func describeNew(d *decoded, who string) *Notice {
	it, b := &d.rec.Item, d.body
	answer := it.Ref != ""
	edited := it.Rev > 0
	switch it.Kind {
	case envelope.KindMessage:
		if edited {
			return nil // edits show in the conversation; they don't need a ping
		}
		n := &Notice{Kind: "message", Title: who, Body: "New message"}
		switch {
		case b.Text != "":
			n.Preview = b.Text
		case len(b.Attachments) > 0:
			n.Body = "Sent a file"
			n.Preview = "📎 " + b.Attachments[0].Name
		}
		return n
	case envelope.KindEvent:
		if edited {
			return &Notice{Kind: "calendar", Title: who, Body: "Changed an event on the calendar"}
		}
		return &Notice{Kind: "calendar", Title: who, Body: "Added an event to the calendar"}
	case envelope.KindSchedule:
		if answer {
			return &Notice{Kind: "calendar", Title: who, Body: "Answered a parenting schedule"}
		}
		return &Notice{Kind: "calendar", Title: who, Body: "Proposed a parenting schedule - it needs your answer"}
	case envelope.KindSwap:
		if answer {
			return &Notice{Kind: "calendar", Title: who, Body: "Answered a change request"}
		}
		return &Notice{Kind: "calendar", Title: who, Body: "Asked to change parenting time - it needs your answer"}
	case envelope.KindExpense:
		if answer {
			return &Notice{Kind: "expense", Title: who, Body: "Updated an expense"}
		}
		if edited {
			return &Notice{Kind: "expense", Title: who, Body: "Changed an expense"}
		}
		return &Notice{Kind: "expense", Title: who, Body: "Added an expense - it needs your answer"}
	case envelope.KindFile:
		if edited {
			return &Notice{Kind: "document", Title: who, Body: "Updated a document"}
		}
		return &Notice{Kind: "document", Title: who, Body: "Added a document"}
	case envelope.KindRecord, envelope.KindInfo: // info bank entries (older ones are records)
		var r struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(b.Data, &r) == nil && (r.Type == "child" || r.Type == "info") {
			return &Notice{Kind: "info", Title: who, Body: "Updated the info bank"}
		}
	case envelope.KindCheckIn:
		return &Notice{Kind: "checkin", Title: who, Body: "Checked in"}
	}
	return nil
}

// SetTimeZone tells the engine the device's time zone (IANA name), for reminders. The app calls it after unlocking.
func (e *Engine) SetTimeZone(zone string) error {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return fmt.Errorf("unknown time zone")
	}
	e.mu.Lock()
	e.zone = loc
	e.mu.Unlock()
	return nil
}

// reminders runs while the engine is unlocked: every 30 s it looks for events whose reminder time has come.
func (e *Engine) reminders(stop <-chan struct{}) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		e.checkReminders(time.Now())
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// checkReminders announces each occurrence's reminder once, from its reminder time until the event starts.
func (e *Engine) checkReminders(now time.Time) {
	e.remindMu.Lock()
	defer e.remindMu.Unlock()
	e.mu.Lock()
	v, loc := e.v, e.zone
	e.mu.Unlock()
	if v == nil {
		return
	}
	if loc == nil {
		loc = time.Local
	}
	spaces, err := e.Spaces()
	if err != nil {
		return
	}
	first, last := now.In(loc).Format("2006-01-02"), now.In(loc).AddDate(0, 0, 8).Format("2006-01-02")
	for _, s := range spaces {
		cal, err := e.Calendar(s.ID, first, last, loc.String())
		if err != nil {
			continue
		}
		for _, ev := range cal.Events {
			if ev.Cancelled || ev.Data.Remind <= 0 {
				continue
			}
			due := ev.Start.Add(-time.Duration(ev.Data.Remind) * time.Minute)
			if now.Before(due) || !now.Before(ev.Start) {
				continue
			}
			key := fmt.Sprintf("reminded/%s/%s/%d", s.ID, ev.ID, ev.Start.Unix())
			var done bool
			if v.Get(vault.State, key, &done) == nil {
				continue
			}
			_ = v.Put(vault.State, key, true)
			when := ev.Start.In(loc).Format("15:04")
			if ev.Data.AllDay {
				when = "today"
				if ev.Start.In(loc).Format("2006-01-02") != now.In(loc).Format("2006-01-02") {
					when = ev.Start.In(loc).Format("Mon 2 Jan")
				}
			} else if ev.Start.In(loc).Format("2006-01-02") != now.In(loc).Format("2006-01-02") {
				when = ev.Start.In(loc).Format("Mon 2 Jan 15:04")
			}
			e.listen("notify", &Notice{Space: s.ID, Family: s.Name, Kind: "reminder", Title: "Reminder: " + when,
				Body: "An event on the calendar", Preview: ev.Data.Title, Item: ev.ID, Time: now})
		}
	}
}
