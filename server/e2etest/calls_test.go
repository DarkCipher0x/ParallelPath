package e2etest

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"pp/engine/app"
)

type inbox struct {
	mu  sync.Mutex
	evs []map[string]any
}

func (b *inbox) listener() app.Listener {
	return func(event string, payload any) {
		if event != "call" {
			return
		}
		raw, _ := json.Marshal(payload)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		b.mu.Lock()
		b.evs = append(b.evs, m)
		b.mu.Unlock()
	}
}

func (b *inbox) find(typ string) map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.evs {
		if e["type"] == typ {
			return e
		}
	}
	return nil
}

// TestCallSetup: a call's key and every setup message reach the other parent sealed with the family's keys (the
// server relays bytes it can't read), and the call is recorded in the family's history.
func TestCallSetup(t *testing.T) {
	var ai, bi inbox
	alice, bob, space, _, bobID := familyWith(t, ai.listener(), bi.listener())

	started, err := alice.CallStart(space, true)
	if err != nil {
		t.Fatal(err)
	}
	call := started["call"]
	var ring map[string]any
	waitFor(t, "bob's phone rings", func() bool { ring = bi.find("ring"); return ring != nil })
	if ring["call"] != call || ring["from_name"] != "Alice" || ring["video"] != true {
		t.Fatalf("ring: %+v", ring)
	}
	if _, leaked := ring["key"]; leaked {
		t.Fatal("the media key was handed to the app's UI events")
	}
	as, err := alice.CallSession(space, call)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := bob.CallSession(space, call)
	if err != nil {
		t.Fatal(err)
	}
	if as["key"] != bs["key"] || as["key"] == "" || as["outgoing"] != true || bs["outgoing"] != false {
		t.Fatalf("sessions: %+v %+v", as, bs)
	}

	// Bob answers; Alice offers to his device; he answers back; ICE candidates flow.
	if err := bob.CallSignal(app.CallSignalInput{Space: space, Call: call, Type: "accept"}); err != nil {
		t.Fatal(err)
	}
	var acc map[string]any
	waitFor(t, "alice hears the accept", func() bool { acc = ai.find("accept"); return acc != nil })
	bobDevice := acc["from_device"].(string)
	if acc["from_person"] != bobID {
		t.Fatalf("accept: %+v", acc)
	}
	sdp := "v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\ns=-\r\n"
	if err := alice.CallSignal(app.CallSignalInput{Space: space, Call: call, ToDevice: bobDevice, Type: "offer", SDP: sdp}); err != nil {
		t.Fatal(err)
	}
	var offer map[string]any
	waitFor(t, "bob gets the offer", func() bool { offer = bi.find("offer"); return offer != nil })
	if offer["sdp"] != sdp {
		t.Fatal("the offer changed on the way")
	}
	if err := alice.CallSignal(app.CallSignalInput{Space: space, Call: call, Type: "ring"}); err == nil {
		t.Fatal("a ring sent through call_signal")
	}

	// Ending it: Bob hears the hang-up, the key is gone, and the family record has the call (start and end).
	if err := alice.CallEnd(space, call, 75, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob hears the hang-up", func() bool { return bi.find("hangup") != nil })
	if _, err := alice.CallSession(space, call); err == nil {
		t.Fatal("the call key outlived the call")
	}
	waitFor(t, "the call is in the family's record (start and end)", func() bool {
		bob.SyncAll()
		log, err := bob.Audit(space)
		if err != nil {
			return false
		}
		n := 0
		for _, r := range log.Rows {
			if strings.Contains(r.What, "call record") {
				n++
			}
		}
		return n >= 2
	})
}
