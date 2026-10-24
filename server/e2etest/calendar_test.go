package e2etest

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pp/engine/app"
	"pp/engine/calendar"
)

// family sets up two approved parents in one family.
func family(t *testing.T) (alice, bob *app.Engine, space, aliceID, bobID string) {
	return familyWith(t, nil, nil)
}

func familyWith(t *testing.T, la, lb app.Listener) (alice, bob *app.Engine, space, aliceID, bobID string) {
	ts := startServer(t)
	inv, _, err := ts.svc.CreateServerInvite()
	if err != nil {
		t.Fatal(err)
	}
	alice, ares := setupWith(t, ts, inv, "Alice", "windows", la)
	space = ares.SpaceID
	code := call[map[string]any](t, alice, "invite", `{"space":"`+space+`","role":"parent"}`)["code"].(string)
	bob, bres := setupWith(t, ts, code, "Bob", "android", lb)
	waitFor(t, "alice sees bob waiting", func() bool {
		alice.SyncAll()
		s, _ := alice.Spaces()
		for _, m := range s[0].Members {
			if m.Person == bres.Person && m.NeedsKeys {
				return true
			}
		}
		return false
	})
	call[map[string]bool](t, alice, "approve", `{"space":"`+space+`","person":"`+bres.Person+`"}`)
	waitFor(t, "bob has keys", func() bool {
		bob.SyncAll()
		s, _ := bob.Spaces()
		return len(s) == 1 && !s[0].Waiting
	})
	return alice, bob, space, ares.Person, bres.Person
}

func calendarOf(t *testing.T, e *app.Engine, space, from, to string) *app.CalendarView {
	t.Helper()
	e.SyncAll()
	v, err := e.Calendar(space, from, to, "Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func dayOf(v *app.CalendarView, date string) calendar.Day {
	for _, d := range v.Days {
		if d.Date == date {
			return d
		}
	}
	return calendar.Day{}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestCalendarEndToEnd(t *testing.T) {
	alice, bob, space, aliceID, bobID := family(t)
	london, _ := time.LoadLocation("Europe/London")

	// Alice proposes alternating weeks with Friday handovers. Bob is asked; nothing applies yet.
	propose := map[string]any{"space": space, "template": "alternating-weeks", "parent_a": aliceID, "parent_b": bobID,
		"start": "2026-11-06", "exchange": "18:00", "zone": "Europe/London", "note": "Like we agreed at mediation"}
	sched := call[map[string]string](t, alice, "propose_schedule", mustJSON(propose))["item"]
	var v *app.CalendarView
	waitFor(t, "bob sees the proposal", func() bool {
		v = calendarOf(t, bob, space, "2026-11-01", "2026-11-30")
		return len(v.Schedules) == 1 && v.Schedules[0].NeedsMe
	})
	if v.Schedules[0].State != "pending" || v.NeedsMe != 1 || dayOf(v, "2026-11-10").Start != "" {
		t.Fatalf("pending schedule applied: %+v", v.Schedules[0])
	}
	if _, err := bob.Call("answer", []byte(`{"space":"`+space+`","item":"`+sched+`","accept":false,"comment":""}`)); err == nil {
		t.Fatal("a decline without a reason was accepted")
	}
	if _, err := alice.Call("answer", []byte(`{"space":"`+space+`","item":"`+sched+`","accept":true}`)); err == nil {
		t.Fatal("alice answered her own proposal")
	}
	call[map[string]bool](t, bob, "answer", `{"space":"`+space+`","item":"`+sched+`","accept":true,"comment":"Agreed"}`)
	waitFor(t, "the schedule is agreed for both", func() bool {
		a := calendarOf(t, alice, space, "2026-11-01", "2026-11-30")
		b := calendarOf(t, bob, space, "2026-11-01", "2026-11-30")
		return a.Schedules[0].State == "agreed" && b.Schedules[0].State == "agreed"
	})
	v = calendarOf(t, alice, space, "2026-11-01", "2026-11-30")
	if d := dayOf(v, "2026-11-10"); d.Start != aliceID || d.End != aliceID {
		t.Fatalf("week one is alice's: %+v", d)
	}
	if d := dayOf(v, "2026-11-13"); len(d.Handovers) != 1 || d.Handovers[0].To != bobID {
		t.Fatalf("friday handover to bob: %+v", d)
	}

	// Changing an agreed proposal needs a reason, and it needs agreeing again (the old answer was to another version).
	propose["item"], propose["reason"], propose["exchange"] = sched, "Earlier handover suits school pick-up", "15:30"
	if _, err := alice.Call("propose_schedule", []byte(strings.Replace(mustJSON(propose), `"reason":"Earlier handover suits school pick-up"`, `"reason":""`, 1))); err == nil {
		t.Fatal("a change without a reason was accepted")
	}
	call[map[string]string](t, alice, "propose_schedule", mustJSON(propose))
	waitFor(t, "the change needs agreeing again", func() bool {
		v = calendarOf(t, bob, space, "2026-11-01", "2026-11-30")
		return v.Schedules[0].State == "pending" && v.Schedules[0].Rev == 1 && v.Schedules[0].NeedsMe
	})
	if len(v.Schedules[0].History) != 2 || v.Schedules[0].Reason != "Earlier handover suits school pick-up" {
		t.Fatalf("history: %+v", v.Schedules[0])
	}
	call[map[string]bool](t, bob, "answer", `{"space":"`+space+`","item":"`+sched+`","accept":true,"comment":""}`)
	waitFor(t, "agreed again", func() bool {
		return calendarOf(t, alice, space, "2026-11-01", "2026-11-30").Schedules[0].State == "agreed"
	})

	// Bob asks to have the children on Alice's weekend; Alice declines (with a reason), then he asks for another.
	ask := func(from, to, note string) string {
		in := app.SwapInput{Space: space, Note: note, Changes: []app.Change{{From: at(london, from), To: at(london, to), Parent: bobID}}}
		return call[map[string]string](t, bob, "request_change", mustJSON(in))["item"]
	}
	first := ask("2026-11-07 10:00", "2026-11-08 17:00", "My mum's 70th")
	waitFor(t, "alice sees the request", func() bool {
		v = calendarOf(t, alice, space, "2026-11-01", "2026-11-30")
		return len(v.Changes) == 1 && v.Changes[0].NeedsMe
	})
	call[map[string]bool](t, alice, "answer", `{"space":"`+space+`","item":"`+first+`","accept":false,"comment":"We're away that weekend"}`)
	waitFor(t, "declined", func() bool {
		v = calendarOf(t, bob, space, "2026-11-01", "2026-11-30")
		return v.Changes[0].State == "declined" && len(v.Changes[0].Answers) == 1 && v.Changes[0].Answers[0].Comment == "We're away that weekend"
	})
	if d := dayOf(v, "2026-11-07"); d.Changed || d.Start != aliceID {
		t.Fatalf("a declined change applied: %+v", d)
	}
	second := ask("2026-11-21 10:00", "2026-11-22 17:00", "Swap for the 70th instead?")
	waitFor(t, "alice sees the second request", func() bool {
		return len(calendarOf(t, alice, space, "2026-11-01", "2026-11-30").Changes) == 2
	})
	call[map[string]bool](t, alice, "answer", `{"space":"`+space+`","item":"`+second+`","accept":true,"comment":"Yes, have a lovely time"}`)
	waitFor(t, "the change applies", func() bool {
		v = calendarOf(t, bob, space, "2026-11-01", "2026-11-30")
		d := dayOf(v, "2026-11-21")
		return d.Changed && len(d.Handovers) == 1 && d.Handovers[0].To == bobID && d.Handovers[0].Source == second
	})

	// Events: Alice adds a weekly club; Bob can't edit it; Alice moves it (with a reason) and cancels one.
	ev := app.EventInput{Space: space, Notes: "Bring shin pads", Data: app.EventData{Title: "Football club", Category: "activity",
		Start: at(london, "2026-11-04 16:00"), End: at(london, "2026-11-04 17:00"), Zone: "Europe/London", Children: []string{"Mia"},
		Repeat: calendar.Repeat{Every: calendar.Weekly, Until: "2026-11-25"}}}
	club := call[map[string]string](t, alice, "save_event", mustJSON(ev))["item"]
	waitFor(t, "bob sees four club sessions", func() bool {
		n := 0
		for _, e := range calendarOf(t, bob, space, "2026-11-01", "2026-11-30").Events {
			if e.ID == club {
				n++
			}
		}
		return n == 4
	})
	ev.Item, ev.Reason = club, "New coach"
	if _, err := bob.Call("save_event", []byte(mustJSON(ev))); err == nil {
		t.Fatal("bob edited alice's event")
	}
	ev.Data.Start, ev.Data.End = at(london, "2026-11-04 17:00"), at(london, "2026-11-04 18:00")
	call[map[string]string](t, alice, "save_event", mustJSON(ev))
	call[map[string]bool](t, alice, "cancel_event", `{"space":"`+space+`","item":"`+club+`","reason":"Club closed for the term"}`)
	waitFor(t, "bob sees it cancelled, with the reason and history", func() bool {
		for _, e := range calendarOf(t, bob, space, "2026-11-01", "2026-11-30").Events {
			if e.ID == club && e.Cancelled && e.Reason == "Club closed for the term" && len(e.History) == 3 && e.Start.Hour() == 17 {
				return true
			}
		}
		return false
	})

	// A check-in with a location, as phones send it.
	lat, lon := 51.5007, -0.1246 // a public landmark
	call[map[string]bool](t, bob, "check_in", mustJSON(app.CheckInInput{Space: space, Place: "School gate", Lat: &lat, Lon: &lon, Accuracy: 12}))
	now := time.Now().In(london).Format("2006-01-02")
	waitFor(t, "alice sees the check-in", func() bool {
		cv := calendarOf(t, alice, space, now, now)
		return len(cv.CheckIns) == 1 && cv.CheckIns[0].Name == "Bob" && *cv.CheckIns[0].Data.Lat == lat
	})

	// Everything above is in the family's verified audit log.
	log, err := alice.Audit(space)
	if err != nil || !log.Verified {
		t.Fatalf("audit: %v %+v", err, log)
	}
}

func at(loc *time.Location, s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", s, loc)
	return t
}
