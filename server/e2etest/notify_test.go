package e2etest

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"pp/engine/app"
)

type notices struct {
	mu   sync.Mutex
	list []app.Notice
}

func (n *notices) listener(event string, payload any) {
	if event != "notify" {
		return
	}
	var x app.Notice
	raw, _ := json.Marshal(payload)
	if json.Unmarshal(raw, &x) == nil {
		n.mu.Lock()
		n.list = append(n.list, x)
		n.mu.Unlock()
	}
}

func (n *notices) find(kind string) *app.Notice {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := range n.list {
		if n.list[i].Kind == kind {
			return &n.list[i]
		}
	}
	return nil
}

// TestNotifications: the other parent's device announces new messages and requests (without content in the
// lock-screen text), never your own, and an event's reminder fires once before it starts.
func TestNotifications(t *testing.T) {
	var forAlice, forBob notices
	alice, bob, space, _, _ := familyWith(t, forAlice.listener, forBob.listener)
	if err := alice.Send(app.SendRequest{Space: space, Text: "Pick-up is at 5 today"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob is told about the message", func() bool { bob.SyncAll(); return forBob.find("message") != nil })
	n := forBob.find("message")
	if n.Title != "Alice" || n.Body != "New message" || n.Preview != "Pick-up is at 5 today" || n.Space != space {
		t.Fatalf("message notice: %+v", n)
	}
	if forAlice.find("message") != nil {
		t.Fatal("alice was notified about her own message")
	}
	if err := bob.SetTimeZone("Europe/London"); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(10 * time.Minute).Truncate(time.Minute)
	ev := app.EventInput{Space: space, Data: app.EventData{Title: "Dentist", Category: "medical", Start: start, End: start.Add(30 * time.Minute),
		Zone: "Europe/London", Remind: 15}}
	if _, err := alice.SaveEvent(ev); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob is told about the event", func() bool { bob.SyncAll(); return forBob.find("calendar") != nil })
	waitFor(t, "bob's reminder fires", func() bool { return forBob.find("reminder") != nil })
	r := forBob.find("reminder")
	if r.Preview != "Dentist" || r.Body != "An event on the calendar" {
		t.Fatalf("reminder: %+v", r)
	}
	time.Sleep(200 * time.Millisecond)
	bob.SyncAll()
	count := 0
	forBob.mu.Lock()
	for _, x := range forBob.list {
		if x.Kind == "reminder" {
			count++
		}
	}
	forBob.mu.Unlock()
	if count != 1 {
		t.Fatalf("the reminder fired %d times", count)
	}
}
