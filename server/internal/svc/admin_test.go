package svc

import (
	"errors"
	"testing"

	"pp/core/keys"
)

func TestAdminOperations(t *testing.T) {
	s, _ := newService(t)
	code, _, err := s.CreateServerInvite()
	if err != nil {
		t.Fatal(err)
	}
	spare, _, _ := s.CreateServerInvite()
	inv, _ := s.AdminInvites()
	if len(inv) != 2 || inv[0].Kind != "server" || inv[0].CreatedBy != "admin" {
		t.Fatalf("invites: %+v", inv)
	}

	alice := newPerson(t, keys.KindWindows)
	if _, err := s.Enrol(code, *alice.cert, nil); err != nil {
		t.Fatal(err)
	}
	alice.signIn(t, s)

	o, err := s.AdminOverview()
	if err != nil || o.People != 1 || o.Devices != 1 || o.SignedIn != 1 || o.Spaces != 0 || o.Invites != 1 {
		t.Fatalf("overview: %+v %v", o, err)
	}
	people, _ := s.AdminPeople()
	if len(people) != 1 || len(people[0].Devices) != 1 || people[0].Devices[0].Name != "test device" || people[0].Spaces != 0 ||
		people[0].Devices[0].LastSignIn.IsZero() {
		t.Fatalf("people: %+v", people)
	}
	spaces, _ := s.AdminSpaces()
	if len(spaces) != 0 { // enrolling makes an account; families are created from the app
		t.Fatalf("spaces: %+v", spaces)
	}

	// Revoking the unused invite makes it unusable.
	inv, _ = s.AdminInvites()
	if err := s.AdminRevokeInvite(inv[0].ID, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enrol(spare, *newPerson(t, keys.KindAndroid).cert, nil); err == nil {
		t.Fatal("a revoked invite still worked")
	}
	if err := s.AdminRevokeInvite("not-an-id", "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatal("a bad invite ID was accepted")
	}

	// Blocking signs the account out and stops new sign-ins; unblocking lets them back in.
	if err := s.AdminSetBlocked(alice.caller.Person, true, "tester"); err != nil {
		t.Fatal(err)
	}
	if o, _ := s.AdminOverview(); o.SignedIn != 0 || o.PeopleBlocked != 1 {
		t.Fatalf("after block: %+v", o)
	}
	nonce, _ := s.Challenge(alice.caller.Device)
	sig, _ := alice.dev.Sign.Sign(PurposeAuth, AuthMessage(s.ServerKey(), alice.caller.Device, nonce))
	if _, _, err := s.SignIn(alice.caller.Device, sig); err == nil {
		t.Fatal("a blocked account signed in")
	}
	if err := s.AdminSetBlocked(alice.caller.Person, false, "tester"); err != nil {
		t.Fatal(err)
	}
	alice.signIn(t, s)

	// Signing out a device ends its sessions; revoking it stops it for good.
	if err := s.AdminSignOutDevice(alice.caller.Device, "tester"); err != nil {
		t.Fatal(err)
	}
	if o, _ := s.AdminOverview(); o.SignedIn != 0 {
		t.Fatal("device still signed in")
	}
	if err := s.AdminRevokeDevice(alice.caller.Device, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Challenge(alice.caller.Device); err == nil {
		t.Fatal("a revoked device got a sign-in challenge")
	}
	if err := s.AdminRevokeDevice(alice.caller.Device, "tester"); err == nil {
		t.Fatal("revoked twice")
	}

	// Every operator action is in the server chain, and the chain verifies.
	entries, problem, err := s.ServerAudit(100)
	if err != nil || problem != "" {
		t.Fatalf("server audit doesn't verify: %v %s", err, problem)
	}
	if len(entries) < 7 || entries[0].Seq < entries[len(entries)-1].Seq {
		t.Fatalf("server audit: %d entries, newest first expected", len(entries))
	}
}
