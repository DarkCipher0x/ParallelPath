package svc

import (
	"errors"
	"testing"
	"time"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/wire"
	"pp/server/internal/store"
)

// Only the operator brings families onto the server: a server invite gives one family, parents can't start more
// or hand that on, and a journal never takes members.
func TestFamilyAllowance(t *testing.T) {
	s, _ := newService(t)
	alice, bob := newPerson(t, keys.KindWindows), newPerson(t, keys.KindAndroid)
	create := func(p *person, detail string) (string, error) {
		space := wire.NewID()
		g, _ := keys.Grant(p.dev, p.vcert, space, 1, keys.NewEpochKey(), false, time.Time{}, time.Now())
		return space, s.CreateSpace(p.caller, space, p.intent(space, audit.SpaceCreate, "", 0, nil, detail), []keys.EpochGrant{*g})
	}
	inv, _, _ := s.CreateServerInvite()
	if _, err := s.Enrol(inv, *alice.cert, nil); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Account(alice.caller); err != nil || !a.CanStartFamily || a.FamiliesLeft != 1 {
		t.Fatalf("server invite: %+v %v", a, err)
	}
	family, err := create(alice, SpaceKindFamily)
	if err != nil {
		t.Fatal(err)
	}
	// One family per server invite: a second needs the operator.
	if _, err := create(alice, SpaceKindFamily); !errors.Is(err, ErrForbidden) {
		t.Fatalf("second family allowed: %v", err)
	}
	if a, _ := s.Account(alice.caller); a.CanStartFamily {
		t.Fatal("still may start a family")
	}
	// Her journal is fine (once), and can't take members.
	journal, err := create(alice, SpaceKindJournal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := create(alice, SpaceKindJournal); err == nil {
		t.Fatal("second journal")
	}
	if _, _, err := s.SpaceInvite(alice.caller, journal, RoleParent, time.Time{}, time.Time{}, nil,
		alice.intent(journal, audit.MemberInvite, "", 0, nil, "role=parent")); err == nil {
		t.Fatal("invite into a journal")
	}
	if _, err := create(alice, "kind=other"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown kind: %v", err)
	}

	// Bob joins her family as a parent: he can't start a family of his own.
	jInv, _, err := s.SpaceInvite(alice.caller, family, RoleParent, time.Time{}, time.Time{}, nil,
		alice.intent(family, audit.MemberInvite, "", 0, nil, "role=parent"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enrol(jInv, *bob.cert, bob.intent(family, audit.MemberJoin, "", 0, nil, "role=parent")); err != nil {
		t.Fatal(err)
	}
	if _, err := create(bob, SpaceKindFamily); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a parent started a family: %v", err)
	}
	if _, err := create(bob, SpaceKindJournal); err != nil {
		t.Fatal(err)
	}

	// The operator lets Bob start one; it's used up after that.
	if err := s.AdminSetFamiliesLeft(bob.caller.Person, 1, "test admin"); err != nil {
		t.Fatal(err)
	}
	if err := s.AdminSetFamiliesLeft(bob.caller.Person, 99, "test admin"); err == nil {
		t.Fatal("out of range")
	}
	if _, err := create(bob, SpaceKindFamily); err != nil {
		t.Fatal(err)
	}
	if _, err := create(bob, SpaceKindFamily); !errors.Is(err, ErrForbidden) {
		t.Fatal("allowance not used up")
	}
	people, _ := s.AdminPeople()
	for _, p := range people {
		if p.FamiliesLeft != 0 {
			t.Fatalf("%s has %d left", p.ID, p.FamiliesLeft)
		}
	}
}

// Accounts from earlier test builds have no allowance recorded: someone in no family yet keeps their one; anyone in a
// family (with others) gets none.
func TestFamilyAllowanceLegacy(t *testing.T) {
	s, _ := newService(t)
	alice := newPerson(t, keys.KindWindows)
	inv, _, _ := s.CreateServerInvite()
	if _, err := s.Enrol(inv, *alice.cert, nil); err != nil {
		t.Fatal(err)
	}
	legacy := func(id string) {
		_ = s.db.Update(func(tx *store.Tx) error {
			var p Person
			_ = tx.Get(store.Persons, []byte(id), &p)
			p.FamiliesLeft = nil
			return tx.Put(store.Persons, []byte(id), p)
		})
	}
	legacy(alice.caller.Person)
	if a, _ := s.Account(alice.caller); !a.CanStartFamily {
		t.Fatal("legacy account with no family can't start one")
	}
	space := wire.NewID()
	g, _ := keys.Grant(alice.dev, alice.vcert, space, 1, keys.NewEpochKey(), false, time.Time{}, time.Now())
	if err := s.CreateSpace(alice.caller, space, alice.intent(space, audit.SpaceCreate, "", 0, nil, ""), []keys.EpochGrant{*g}); err != nil {
		t.Fatal(err)
	}
	bob := newPerson(t, keys.KindAndroid)
	jInv, _, _ := s.SpaceInvite(alice.caller, space, RoleParent, time.Time{}, time.Time{}, nil,
		alice.intent(space, audit.MemberInvite, "", 0, nil, "role=parent"))
	if _, err := s.Enrol(jInv, *bob.cert, bob.intent(space, audit.MemberJoin, "", 0, nil, "role=parent")); err != nil {
		t.Fatal(err)
	}
	legacy(alice.caller.Person)
	legacy(bob.caller.Person)
	for _, p := range []*person{alice, bob} {
		if a, _ := s.Account(p.caller); a.CanStartFamily {
			t.Fatal("legacy account in a family may start another")
		}
	}
}
