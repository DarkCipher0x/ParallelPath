package envelope

import (
	"testing"
	"time"

	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
)

type actor struct {
	id   *keys.Identity
	dev  *keys.Device
	cert *keys.VerifiedDevice
}

func newActor(t *testing.T) actor {
	id, _ := keys.NewIdentity()
	dev, _ := keys.NewDevice()
	c, err := id.Certify(dev, keys.KindAndroid, "phone", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.Verify()
	if err != nil {
		t.Fatal(err)
	}
	return actor{id, dev, v}
}

func TestMessageLifecycle(t *testing.T) {
	a := newActor(t)
	space := wire.NewID()
	ek := pqc.NewKey()
	m0, err := Seal(a.dev, a.id.PersonID(), 1, ek, Draft{SpaceID: space, Kind: KindMessage, Body: Body{Text: "Pick-up at 5? 🙂"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := m0.CheckShape(); err != nil {
		t.Fatal(err)
	}
	b, err := Open(m0, ek, a.cert)
	if err != nil || b.Text != "Pick-up at 5? 🙂" {
		t.Fatalf("open: %v", err)
	}

	// An edit without a reason is refused.
	if _, err := Seal(a.dev, a.id.PersonID(), 1, ek, Draft{SpaceID: space, Prev: m0, Body: Body{Text: "Pick-up at 6?"}}, time.Now()); err != ErrNoReason {
		t.Fatalf("edit without reason: %v", err)
	}
	m1, err := Seal(a.dev, a.id.PersonID(), 1, ek, Draft{SpaceID: space, Prev: m0, Body: Body{Text: "Pick-up at 6?", Reason: "typo in the time"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if m1.Rev != 1 || m1.ItemID != m0.ItemID {
		t.Fatal("revision numbering")
	}
	if err := Follows(m0, m1); err != nil {
		t.Fatal(err)
	}
	b1, err := Open(m1, ek, a.cert)
	if err != nil || b1.Reason != "typo in the time" {
		t.Fatal("edit body")
	}
	// The original is still readable.
	if b0, _ := Open(m0, ek, a.cert); b0.Text != "Pick-up at 5? 🙂" {
		t.Fatal("original lost")
	}
	// Retraction keeps history.
	m2, err := Seal(a.dev, a.id.PersonID(), 1, ek, Draft{SpaceID: space, Prev: m1, Body: Body{Retracted: true, Reason: "sent to the wrong thread"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := Follows(m1, m2); err != nil {
		t.Fatal(err)
	}
	// Someone else can't edit it.
	other := newActor(t)
	if _, err := Seal(other.dev, other.id.PersonID(), 1, ek, Draft{SpaceID: space, Prev: m2, Body: Body{Text: "x", Reason: "y"}}, time.Now()); err != ErrNotAuthor {
		t.Fatalf("foreign edit: %v", err)
	}
	// A skipped revision is detected.
	if Follows(m0, m2) == nil {
		t.Fatal("skipped revision accepted")
	}
}

func TestTamperAndForgery(t *testing.T) {
	a, b := newActor(t), newActor(t)
	space := wire.NewID()
	ek := pqc.NewKey()
	m, _ := Seal(a.dev, a.id.PersonID(), 1, ek, Draft{SpaceID: space, Kind: KindMessage, Body: Body{Text: "hi"}}, time.Now())

	// Claiming another author fails.
	if _, err := Open(m, ek, b.cert); err == nil {
		t.Fatal("opened with the wrong author")
	}
	// Moving the ciphertext to another item fails (header is bound).
	m2 := *m
	m2.ItemID = wire.NewID()
	if _, err := Open(&m2, ek, a.cert); err == nil {
		t.Fatal("moved item accepted")
	}
	// Flipping a ciphertext bit fails.
	m3 := *m
	m3.Ciphertext = append([]byte(nil), m.Ciphertext...)
	m3.Ciphertext[20] ^= 1
	if _, err := Open(&m3, ek, a.cert); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	// A different epoch key can't open it.
	if _, err := Open(m, pqc.NewKey(), a.cert); err == nil {
		t.Fatal("wrong epoch key opened it")
	}
}

func TestReaction(t *testing.T) {
	a := newActor(t)
	space := wire.NewID()
	ek := pqc.NewKey()
	m, _ := Seal(a.dev, a.id.PersonID(), 1, ek, Draft{SpaceID: space, Kind: KindMessage, Body: Body{Text: "hi"}}, time.Now())
	r, err := Seal(a.dev, a.id.PersonID(), 1, ek, Draft{SpaceID: space, Kind: KindReaction, Ref: m.ItemID, Body: Body{Emoji: "👍"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Ref != m.ItemID {
		t.Fatal("ref")
	}
	if b, err := Open(r, ek, a.cert); err != nil || b.Emoji != "👍" {
		t.Fatal("reaction body")
	}
}
