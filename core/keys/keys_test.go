package keys

import (
	"testing"
	"time"

	"pp/core/wire"
)

func TestCertifyAndVerify(t *testing.T) {
	id, _ := NewIdentity()
	dev, _ := NewDevice()
	c, err := id.Certify(dev, KindWindows, "Desk PC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if v.PersonID != id.PersonID() || v.DeviceID != dev.DeviceID() {
		t.Fatal("ids")
	}
	// Tampering with any field breaks it.
	c.Name = "Evil PC"
	if _, err := c.Verify(); err == nil {
		t.Fatal("renamed cert accepted")
	}
	// A cert claiming someone else's person ID fails.
	other, _ := NewIdentity()
	c2, _ := id.Certify(dev, KindAndroid, "Phone", time.Now())
	c2.PersonID = other.PersonID()
	if _, err := c2.Verify(); err == nil {
		t.Fatal("cert with foreign person id accepted")
	}
}

func TestEpochGrant(t *testing.T) {
	alice, _ := NewIdentity()
	aDev, _ := NewDevice()
	bob, _ := NewIdentity()
	bDev, _ := NewDevice()
	aCert, _ := alice.Certify(aDev, KindWindows, "A", time.Now())
	bCert, _ := bob.Certify(bDev, KindAndroid, "B", time.Now())
	av, _ := aCert.Verify()
	bv, _ := bCert.Verify()
	space := wire.NewID()
	key := NewEpochKey()
	g, err := Grant(aDev, bv, space, 1, key, false, time.Time{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.Open(bDev, av)
	if err != nil || string(got) != string(key) {
		t.Fatalf("open: %v", err)
	}
	// Wrong recipient device can't open it.
	if _, err := g.Open(aDev, av); err == nil {
		t.Fatal("grant opened by another device")
	}
	// A grant claimed to come from someone else fails.
	if _, err := g.Open(bDev, bv); err == nil {
		t.Fatal("grant accepted from the wrong sender")
	}
	// Changing the epoch breaks both the signature and the wrap context.
	g.Epoch = 2
	if _, err := g.Open(bDev, av); err == nil {
		t.Fatal("re-numbered grant accepted")
	}
}
