package svc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pp/core/audit"
	"pp/core/envelope"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
	"pp/server/internal/seal"
	"pp/server/internal/store"
)

// person is a minimal test client: an identity, one device, its certificate and a signed-in caller.
type person struct {
	t      *testing.T
	id     *keys.Identity
	dev    *keys.Device
	cert   *keys.DeviceCert
	vcert  *keys.VerifiedDevice
	caller Caller
}

func newPerson(t *testing.T, kind string) *person {
	id, _ := keys.NewIdentity()
	dev, _ := keys.NewDevice()
	c, err := id.Certify(dev, kind, "test device", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := c.Verify()
	return &person{t: t, id: id, dev: dev, cert: c, vcert: v, caller: Caller{Person: id.PersonID(), Device: dev.DeviceID()}}
}

func (p *person) intent(space, action, item string, rev uint64, hash []byte, detail string) *audit.Intent {
	in, err := audit.SignIntent(p.dev.Sign, audit.Intent{SpaceID: space, Action: action, Person: p.caller.Person,
		Device: p.caller.Device, ItemID: item, ItemRev: rev, ItemHash: hash, Detail: detail, ClientTime: time.Now()})
	if err != nil {
		p.t.Fatal(err)
	}
	return in
}

func (p *person) signIn(t *testing.T, s *Service) {
	nonce, err := s.Challenge(p.caller.Device)
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := p.dev.Sign.Sign(PurposeAuth, AuthMessage(s.ServerKey(), p.caller.Device, nonce))
	tok, c, err := s.SignIn(p.caller.Device, sig)
	if err != nil || c.Person != p.caller.Person {
		t.Fatalf("sign in: %v", err)
	}
	if got, err := s.Authenticate(tok); err != nil || got.Device != p.caller.Device {
		t.Fatal("authenticate")
	}
	// A replayed signature (challenge already used) fails.
	if _, _, err := s.SignIn(p.caller.Device, sig); err == nil {
		t.Fatal("replayed sign-in accepted")
	}
}

func newService(t *testing.T) (*Service, string) {
	dir := t.TempDir()
	k, err := seal.Derive(pqc.NewKey())
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "pp.db"), k.Data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db, k.Signing, k.Session, nil, DefaultConfig), dir
}

func (p *person) seal(t *testing.T, space string, epoch uint64, ek []byte, d envelope.Draft) (*envelope.Item, *audit.Intent) {
	d.SpaceID = space
	it, err := envelope.Seal(p.dev, p.caller.Person, epoch, ek, d, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	h := it.Hash()
	action := audit.ItemCreate
	if it.Rev > 0 {
		action = audit.ItemRevise
	}
	return it, p.intent(space, action, it.ItemID, it.Rev, h[:], "kind="+it.Kind)
}

func TestFamilyFlow(t *testing.T) {
	s, dir := newService(t)
	alice, bob, pro := newPerson(t, keys.KindWindows), newPerson(t, keys.KindAndroid), newPerson(t, keys.KindWeb)

	// The operator invites Alice; she enrols and signs in.
	inv, _, err := s.CreateServerInvite()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enrol(inv, *alice.cert, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enrol(inv, *bob.cert, nil); err == nil {
		t.Fatal("invite used twice")
	}
	alice.signIn(t, s)

	// Alice creates a space and keeps epoch 1's key on her device.
	space := wire.NewID()
	ek1 := keys.NewEpochKey()
	g, _ := keys.Grant(alice.dev, alice.vcert, space, 1, ek1, false, time.Time{}, time.Now())
	if err := s.CreateSpace(alice.caller, space, alice.intent(space, audit.SpaceCreate, "", 0, nil, ""), []keys.EpochGrant{*g}); err != nil {
		t.Fatal(err)
	}

	// She invites Bob as the other parent; he enrols with the invite and records his own join.
	jInv, _, err := s.SpaceInvite(alice.caller, space, RoleParent, time.Time{}, time.Time{}, nil, alice.intent(space, audit.MemberInvite, "", 0, nil, "role=parent"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Enrol(jInv, *bob.cert, bob.intent(space, audit.MemberJoin, "", 0, nil, "role=parent"))
	if err != nil || res.SpaceID != space {
		t.Fatalf("bob enrol: %v", err)
	}

	// Alice shares the epoch key with Bob's device after checking his certificate.
	certs, err := s.Devices(alice.caller, bob.caller.Person)
	if err != nil || len(certs) != 1 {
		t.Fatal("bob's devices")
	}
	bv, err := certs[0].Verify()
	if err != nil {
		t.Fatal(err)
	}
	gb, _ := keys.Grant(alice.dev, bv, space, 1, ek1, false, time.Time{}, time.Now())
	if err := s.ShareKeys(alice.caller, space, 1, []keys.EpochGrant{*gb}, alice.intent(space, audit.KeysGrant, "", 0, nil, "")); err != nil {
		t.Fatal(err)
	}

	// Alice sends a message with a photo attachment record.
	msg, mi := alice.seal(t, space, 1, ek1, envelope.Draft{Kind: envelope.KindMessage, Body: envelope.Body{Text: "Swap Saturday for Sunday? ðŸ˜Š"}})
	if _, _, err := s.PostItem(alice.caller, msg, mi); err != nil {
		t.Fatal(err)
	}

	// Bob fetches his grant, unwraps the key and reads the message.
	grants, _ := s.MyGrants(bob.caller, space)
	if len(grants) != 1 {
		t.Fatal("bob's grants")
	}
	bk, err := grants[0].Open(bob.dev, alice.vcert)
	if err != nil {
		t.Fatal(err)
	}
	items, _ := s.Items(bob.caller, space, 0, 0)
	if len(items) != 1 {
		t.Fatal("items")
	}
	body, err := envelope.Open(&items[0].Item, bk, alice.vcert)
	if err != nil || body.Text != "Swap Saturday for Sunday? ðŸ˜Š" {
		t.Fatalf("bob reads: %v", err)
	}

	// Read receipt: once only.
	h := msg.Hash()
	if _, err := s.Record(bob.caller, bob.intent(space, audit.ItemView, msg.ItemID, 0, h[:], "")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Record(bob.caller, bob.intent(space, audit.ItemView, msg.ItemID, 0, h[:], "")); err == nil {
		t.Fatal("second receipt accepted")
	}

	// Bob reacts.
	r, ri := bob.seal(t, space, 1, bk, envelope.Draft{Kind: envelope.KindReaction, Ref: msg.ItemID, Body: envelope.Body{Emoji: "ðŸ‘"}})
	if _, _, err := s.PostItem(bob.caller, r, ri); err != nil {
		t.Fatal(err)
	}

	// Alice edits with a reason; Bob can't post a revision of Alice's message.
	m1, m1i := alice.seal(t, space, 1, ek1, envelope.Draft{Prev: msg, Body: envelope.Body{Text: "Swap Saturday for Sunday?", Reason: "removed emoji"}})
	if _, _, err := s.PostItem(alice.caller, m1, m1i); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PostItem(alice.caller, m1, m1i); err == nil {
		t.Fatal("same revision stored twice")
	}
	forged := *m1
	forged.Person, forged.Device = bob.caller.Person, bob.caller.Device
	if _, _, err := s.PostItem(bob.caller, &forged, m1i); err == nil {
		t.Fatal("forged revision accepted")
	}

	// A professional joins read-only and can't post.
	pInv, _, err := s.SpaceInvite(alice.caller, space, RoleProfessional, time.Now().AddDate(0, 3, 0), time.Time{}, nil, alice.intent(space, audit.MemberInvite, "", 0, nil, "role=professional"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enrol(pInv, *pro.cert, pro.intent(space, audit.MemberJoin, "", 0, nil, "role=professional")); err != nil {
		t.Fatal(err)
	}
	pm, pmi := pro.seal(t, space, 1, ek1, envelope.Draft{Kind: envelope.KindMessage, Body: envelope.Body{Text: "hi"}})
	if _, _, err := s.PostItem(pro.caller, pm, pmi); err != ErrForbidden {
		t.Fatalf("professional posted: %v", err)
	}
	// Bob (not a creator but a parent) can't invite a professional without an end date.
	if _, _, err := s.SpaceInvite(bob.caller, space, RoleProfessional, time.Time{}, time.Time{}, nil, bob.intent(space, audit.MemberInvite, "", 0, nil, "role=professional")); err == nil {
		t.Fatal("professional invite without end date accepted")
	}

	// The whole audit chain verifies on the client side.
	entries, err := s.Audit(bob.caller, space, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	devs := map[string]pqc.VerifyKey{alice.caller.Device: alice.dev.Sign.Public(), bob.caller.Device: bob.dev.Sign.Public(), pro.caller.Device: pro.dev.Sign.Public()}
	head, err := audit.Verify(space, entries, s.ServerKey(), func(d string) (pqc.VerifyKey, bool) { k, ok := devs[d]; return k, ok }, nil)
	if err != nil {
		t.Fatal(err)
	}
	// create, invite, join, keys, message, view, reaction, edit, invite pro, pro join = 10
	if head.Seq != 10 {
		t.Fatalf("audit has %d entries", head.Seq)
	}

	// Alice removes the professional and must rotate to every remaining device.
	if err := s.RemoveMember(alice.caller, space, pro.caller.Person, alice.intent(space, audit.MemberRemove, "", 0, nil, "person="+pro.caller.Person)); err != nil {
		t.Fatal(err)
	}
	ek2 := keys.NewEpochKey()
	ga2, _ := keys.Grant(alice.dev, alice.vcert, space, 2, ek2, false, time.Time{}, time.Now())
	if err := s.ShareKeys(alice.caller, space, 2, []keys.EpochGrant{*ga2}, alice.intent(space, audit.EpochRotate, "", 0, nil, "")); err == nil {
		t.Fatal("rotation that leaves Bob out accepted")
	}
	gb2, _ := keys.Grant(alice.dev, bv, space, 2, ek2, false, time.Time{}, time.Now())
	if err := s.ShareKeys(alice.caller, space, 2, []keys.EpochGrant{*ga2, *gb2}, alice.intent(space, audit.EpochRotate, "", 0, nil, "")); err != nil {
		t.Fatal(err)
	}
	// Old-epoch items are now refused; the removed professional can't read anything.
	old, oi := alice.seal(t, space, 1, ek1, envelope.Draft{Kind: envelope.KindMessage, Body: envelope.Body{Text: "x"}})
	if _, _, err := s.PostItem(alice.caller, old, oi); err != ErrConflict {
		t.Fatalf("stale-epoch item: %v", err)
	}
	if _, err := s.Items(pro.caller, space, 0, 0); err == nil {
		t.Fatal("removed member can still read")
	}

	// Blobs: upload needs the encrypted header and is audited; download is audited first.
	blobID := wire.NewID()
	tmp := filepath.Join(dir, "up.tmp")
	os.WriteFile(tmp, append([]byte("PPB1"), make([]byte, 100)...), 0o600)
	if err := s.StoreBlob(alice.caller, dir, space, blobID, tmp, 104, alice.intent(space, audit.BlobUpload, blobID, 0, nil, "size=104")); err != nil {
		t.Fatal(err)
	}
	f, n, err := s.OpenBlob(bob.caller, dir, blobID, bob.intent(space, audit.BlobDownload, blobID, 0, nil, ""))
	if err != nil || n != 104 {
		t.Fatalf("download: %v", err)
	}
	f.Close()
	if _, _, err := s.OpenBlob(pro.caller, dir, blobID, pro.intent(space, audit.BlobDownload, blobID, 0, nil, "")); err == nil {
		t.Fatal("removed member downloaded")
	}
}
