package audit

import (
	"testing"
	"time"

	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
)

type fixture struct {
	server  *pqc.SigningKey
	person  string
	dev     *keys.Device
	devices DeviceKeys
	space   string
}

func newFixture(t *testing.T) fixture {
	s, _ := pqc.NewSigningKey()
	id, _ := keys.NewIdentity()
	d, _ := keys.NewDevice()
	return fixture{server: s, person: id.PersonID(), dev: d, space: wire.NewID(),
		devices: func(dev string) (pqc.VerifyKey, bool) {
			if dev == d.DeviceID() {
				return d.Sign.Public(), true
			}
			return pqc.VerifyKey{}, false
		}}
}

func (f fixture) chain(t *testing.T, n int) []Entry {
	var out []Entry
	var head *Entry
	for i := 0; i < n; i++ {
		in, err := SignIntent(f.dev.Sign, Intent{SpaceID: f.space, Action: ItemCreate, Person: f.person,
			Device: f.dev.DeviceID(), ItemID: wire.NewID(), Detail: "kind=message", ClientTime: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		e, err := Append(f.server, head, *in, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, *e)
		head = e
	}
	return out
}

func TestChainVerifies(t *testing.T) {
	f := newFixture(t)
	c := f.chain(t, 5)
	h, err := Verify(f.space, c, f.server.Public(), f.devices, nil)
	if err != nil {
		t.Fatal(err)
	}
	if h.Seq != 5 {
		t.Fatal("head seq")
	}
	// Re-verifying against the head seen earlier is fine.
	if _, err := Verify(f.space, c, f.server.Public(), f.devices, h); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyFrom(t *testing.T) {
	f := newFixture(t)
	c := f.chain(t, 6)
	h3, err := Verify(f.space, c[:3], f.server.Public(), f.devices, nil)
	if err != nil {
		t.Fatal(err)
	}
	h, err := VerifyFrom(f.space, h3, c[2:], f.server.Public(), f.devices)
	if err != nil || h.Seq != 6 {
		t.Fatalf("incremental verify: %v", err)
	}
	// Starting anywhere but the remembered head fails.
	if _, err := VerifyFrom(f.space, h3, c[3:], f.server.Public(), f.devices); err == nil {
		t.Fatal("gap accepted")
	}
	// A different chain at the same position fails.
	other := f.chain(t, 6)
	if _, err := VerifyFrom(f.space, h3, other[2:], f.server.Public(), f.devices); err == nil {
		t.Fatal("rewritten chain accepted")
	}
}

func TestChainDetectsTampering(t *testing.T) {
	f := newFixture(t)
	c := f.chain(t, 4)
	h, _ := Verify(f.space, c, f.server.Public(), f.devices, nil)

	cases := map[string]func([]Entry) []Entry{
		"entry removed":      func(c []Entry) []Entry { return append(append([]Entry{}, c[:1]...), c[2:]...) },
		"entries reordered":  func(c []Entry) []Entry { d := append([]Entry{}, c...); d[1], d[2] = d[2], d[1]; return d },
		"detail changed":     func(c []Entry) []Entry { d := append([]Entry{}, c...); d[2].Detail = "kind=expense"; return d },
		"server sig dropped": func(c []Entry) []Entry { d := append([]Entry{}, c...); d[3].ServerSig = nil; return d },
		"cut short":          func(c []Entry) []Entry { return c[:2] },
	}
	for name, mutate := range cases {
		if _, err := Verify(f.space, mutate(c), f.server.Public(), f.devices, h); err == nil {
			t.Errorf("%s: not detected", name)
		}
	}
	// A completely rebuilt chain (same length, re-signed by the server) is caught by the remembered head.
	f2 := f
	rebuilt := f2.chain(t, 4)
	if _, err := Verify(f.space, rebuilt, f.server.Public(), f.devices, h); err == nil {
		t.Error("rewritten log not detected")
	}
	// An entry from an unknown device fails.
	other, _ := keys.NewDevice()
	in, _ := SignIntent(other.Sign, Intent{SpaceID: f.space, Action: ItemView, Person: f.person, Device: other.DeviceID(), ClientTime: time.Now()})
	last := c[len(c)-1]
	e, _ := Append(f.server, &last, *in, time.Now())
	if _, err := Verify(f.space, append(c, *e), f.server.Public(), f.devices, nil); err == nil {
		t.Error("unknown device accepted")
	}
}

func TestOperatorEntries(t *testing.T) {
	f := newFixture(t)
	op := func(space string) []Entry {
		in, err := SignIntent(f.server, Intent{SpaceID: space, Action: AdminAction, Person: Operator, Device: Operator,
			Detail: "settings: changed", ClientTime: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		e, err := Append(f.server, nil, *in, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return []Entry{*e}
	}
	if _, err := Verify("", op(""), f.server.Public(), f.devices, nil); err != nil {
		t.Fatalf("an operator entry in the server chain: %v", err)
	}
	if _, err := Verify(f.space, op(f.space), f.server.Public(), f.devices, nil); err == nil {
		t.Fatal("an operator entry was accepted in a family's chain")
	}
	// Signed by anything but the server key, it fails.
	other, _ := pqc.NewSigningKey()
	in, _ := SignIntent(other, Intent{Action: AdminAction, Person: Operator, Device: Operator, ClientTime: time.Now()})
	e, _ := Append(f.server, nil, *in, time.Now())
	if _, err := Verify("", []Entry{*e}, f.server.Public(), f.devices, nil); err == nil {
		t.Fatal("an operator entry signed by another key was accepted")
	}
}
