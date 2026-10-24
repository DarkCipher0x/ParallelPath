package svc

import (
	"fmt"
	"time"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/wire"
	"pp/server/internal/store"
)

// CreateSpace starts a family space or the person's private journal. The creator becomes its first parent and
// uploads epoch-1 grants for their own devices (the epoch key is generated on their device; the server never sees
// it). A family needs an allowance from the operator (Person.FamiliesLeft); a journal is one per person.
func (s *Service) CreateSpace(c Caller, spaceID string, intent *audit.Intent, grants []keys.EpochGrant) error {
	if wire.CheckID(spaceID) != nil || intent == nil || intent.SpaceID != spaceID || intent.Action != audit.SpaceCreate ||
		(intent.Detail != SpaceKindFamily && intent.Detail != SpaceKindJournal) {
		return ErrInvalid
	}
	journal := intent.Detail == SpaceKindJournal
	return s.db.Update(func(tx *store.Tx) error {
		if tx.Has(store.Spaces, []byte(spaceID)) {
			return ErrConflict
		}
		var p Person
		if err := tx.Get(store.Persons, []byte(c.Person), &p); err != nil {
			return ErrUnauthorized
		}
		if journal {
			if s.hasJournal(tx, c.Person) {
				return fmt.Errorf("%w: you already have a journal", ErrConflict)
			}
		} else {
			left := s.familiesLeft(tx, &p)
			if left < 1 {
				return fmt.Errorf("%w: starting a new family on this server is up to its administrator", ErrForbidden)
			}
			left--
			p.FamiliesLeft = &left
			if err := tx.Put(store.Persons, []byte(c.Person), p); err != nil {
				return err
			}
		}
		if err := tx.Put(store.Spaces, []byte(spaceID), Space{ID: spaceID, Created: s.now(), CreatedBy: c.Person, Epoch: 1,
			EpochSince: s.now(), Journal: journal}); err != nil {
			return err
		}
		m := Member{Person: c.Person, Role: RoleParent, Status: "active", Joined: s.now()}
		if err := tx.Put(store.Members, store.Key(spaceID, c.Person), m); err != nil {
			return err
		}
		if err := tx.Put(store.PersonSpaces, store.Key(c.Person, spaceID), true); err != nil {
			return err
		}
		if err := s.storeGrants(tx, c, spaceID, 1, grants); err != nil {
			return err
		}
		_, err := s.appendAudit(tx, c, intent)
		return err
	})
}

// SpaceInvite creates a one-time code to join a space with a role. Only parents may invite.
// Professionals are always read-only and must have an end date.
//
// historyFrom (professionals only, optional) limits what they see to items first created from then on.
func (s *Service) SpaceInvite(c Caller, spaceID, role string, notAfter, historyFrom time.Time, perms *Perms, intent *audit.Intent) (string, time.Time, error) {
	if !ValidRole(role) || intent == nil || intent.SpaceID != spaceID || intent.Action != audit.MemberInvite || intent.Detail != "role="+role {
		return "", time.Time{}, ErrInvalid
	}
	readOnly := role == RoleProfessional || role == RoleChild
	if role == RoleProfessional && (notAfter.IsZero() || notAfter.Before(s.now()) || notAfter.After(s.now().AddDate(2, 0, 0))) {
		return "", time.Time{}, fmt.Errorf("%w: professional access needs an end date within 2 years", ErrInvalid)
	}
	if !historyFrom.IsZero() && (role != RoleProfessional || !historyFrom.Before(notAfter)) {
		return "", time.Time{}, fmt.Errorf("%w: a history start date is for professionals, before their end date", ErrInvalid)
	}
	token := randToken()
	inv := Invite{SpaceID: spaceID, Role: role, CreatedBy: c.Person, Created: s.now(), Expires: s.now().Add(s.cfg.InviteTTL),
		ReadOnly: readOnly, NotAfter: wire.Ms(notAfter)}
	if notAfter.IsZero() {
		inv.NotAfter = time.Time{}
	}
	if !historyFrom.IsZero() {
		inv.HistoryFrom = wire.Ms(historyFrom)
	}
	if perms != nil && role != RoleParent {
		p := *perms
		if err := p.check(role); err != nil {
			return "", time.Time{}, err
		}
		inv.Perms = &p
	}
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		if sp, err := s.approvedParent(tx, c, spaceID); err != nil {
			return err
		} else if sp.Parent != "" {
			return fmt.Errorf("%w: people join a family, not a conversation", ErrInvalid)
		} else if sp.Journal {
			return fmt.Errorf("%w: a journal is private", ErrInvalid)
		}
		if err := tx.Put(store.Invites, s.tokenKey("invite", token), inv); err != nil {
			return err
		}
		if _, err := s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		people, _ = s.activeMembers(tx, spaceID)
		return nil
	})
	if err != nil {
		return "", time.Time{}, err
	}
	s.emit(people, Event{SpaceID: spaceID, Kind: "audit"})
	return keys.InviteCode{Kind: 'J', Space: spaceID, Role: role, Secret: token}.String(), inv.Expires, nil
}

// JoinExisting lets an already-registered person accept a space invite with their existing account.
func (s *Service) JoinExisting(c Caller, invite string, joinIntent *audit.Intent) (*EnrolResult, error) {
	res := &EnrolResult{PersonID: c.Person}
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		inv, tk, err := s.lookupInvite(tx, invite)
		if err != nil {
			return err
		}
		if inv.SpaceID == "" {
			return ErrInvalid
		}
		if tx.Has(store.Members, store.Key(inv.SpaceID, c.Person)) {
			return ErrConflict
		}
		m := Member{Person: c.Person, Role: inv.Role, Status: "active", Joined: s.now(), ReadOnly: inv.ReadOnly, NotAfter: inv.NotAfter,
			HistoryFrom: inv.HistoryFrom, Perms: inv.Perms, InvitedBy: inv.CreatedBy}
		if err := tx.Put(store.Members, store.Key(inv.SpaceID, c.Person), m); err != nil {
			return err
		}
		if err := tx.Put(store.PersonSpaces, store.Key(c.Person, inv.SpaceID), true); err != nil {
			return err
		}
		if joinIntent == nil || joinIntent.SpaceID != inv.SpaceID || joinIntent.Action != audit.MemberJoin || joinIntent.Detail != "role="+inv.Role {
			return ErrInvalid
		}
		if _, err := s.appendAudit(tx, c, joinIntent); err != nil {
			return err
		}
		res.SpaceID, res.Role = inv.SpaceID, inv.Role
		people, _ = s.activeMembers(tx, inv.SpaceID)
		return tx.Delete(store.Invites, tk)
	})
	if err != nil {
		return nil, err
	}
	s.emit(people, Event{SpaceID: res.SpaceID, Kind: "member"})
	return res, nil
}

// SpaceView is what a member sees about a space.
type SpaceView struct {
	Space   Space    `json:"space"`
	Members []Member `json:"members"`
	Me      Member   `json:"me"`
	// RotateDue tells parents' devices to move the space to a new epoch key: the current one is over a month old,
	// or a device that holds it no longer should (revoked, or its person removed or out of their access window).
	RotateDue bool `json:"rotate_due,omitempty"`
	// KeyedDevices (parents) are the devices a rotation must re-key: they hold the current epoch and still may.
	KeyedDevices []string `json:"keyed_devices,omitempty"`
	// FamilyKeyed (conversations) are the participants' devices approved in the family.
	FamilyKeyed []string `json:"family_keyed,omitempty"`
}

// Spaces lists the spaces the caller belongs to.
func (s *Service) Spaces(c Caller) ([]SpaceView, error) {
	var out []SpaceView
	err := s.db.View(func(tx *store.Tx) error {
		var ids []string
		tx.Keys(store.PersonSpaces, append([]byte(c.Person), '/'), func(k []byte) bool {
			ids = append(ids, string(k[len(c.Person)+1:]))
			return true
		})
		for _, id := range ids {
			v, err := s.spaceView(tx, c, id)
			if err == nil {
				out = append(out, *v)
			}
		}
		return nil
	})
	return out, err
}

// SpaceDetail returns one space.
func (s *Service) SpaceDetail(c Caller, spaceID string) (*SpaceView, error) {
	var v *SpaceView
	err := s.db.View(func(tx *store.Tx) (err error) { v, err = s.spaceView(tx, c, spaceID); return })
	return v, err
}

func (s *Service) spaceView(tx *store.Tx, c Caller, id string) (*SpaceView, error) {
	me, err := s.member(tx, id, c.Person)
	if err != nil {
		return nil, err
	}
	var sp Space
	if err := tx.Get(store.Spaces, []byte(id), &sp); err != nil {
		return nil, ErrNotFound
	}
	v := &SpaceView{Space: sp, Me: *me}
	err = store.Scan(tx, store.Members, append([]byte(id), '/'), nil, func(_ []byte, m *Member) bool {
		m.HasKeys = hasCurrentKeys(tx, id, sp.Epoch, m.Person)
		if sp.Parent == "" {
			p := permsOf(m, &sp) // everyone sees the effective permissions (the apps show what's allowed)
			m.Perms = &p
		}
		v.Members = append(v.Members, *m) // removed members stay listed (history), with their status
		return true
	})
	v.Me.HasKeys = hasCurrentKeys(tx, id, sp.Epoch, c.Person)
	if sp.Parent == "" {
		p := permsOf(me, &sp)
		v.Me.Perms = &p
	}
	if me.Role == RoleParent || me.Role == RoleParticipant {
		v.RotateDue = s.rotateDue(tx, &sp)
		v.KeyedDevices = s.keyedDevices(tx, id, sp.Epoch)
	}
	if sp.Parent != "" {
		// A participant's devices approved in the family since the conversation began (a linked phone, a recovered
		// device once re-approved) may be given the conversation's keys by any participant.
		var fam Space
		if tx.Get(store.Spaces, []byte(sp.Parent), &fam) == nil {
			in := map[string]bool{}
			for _, m := range v.Members {
				if m.Status == "active" {
					in[m.Person] = true
				}
			}
			for _, d := range s.keyedDevices(tx, sp.Parent, fam.Epoch) {
				if cert, err := deviceCert(tx, d); err == nil && in[cert.PersonID] {
					v.FamilyKeyed = append(v.FamilyKeyed, d)
				}
			}
		}
	}
	return v, err
}

// rotateDue reports whether the space's current epoch key should be replaced (see SpaceView.RotateDue).
func (s *Service) rotateDue(tx *store.Tx, sp *Space) bool {
	now := s.now()
	if now.Sub(sp.epochStart()) > RotateAfter {
		return true
	}
	due := false
	_ = store.Scan(tx, store.Grants, append(store.Key(sp.ID, string(store.U64(sp.Epoch))), '/'), nil, func(_ []byte, g *keys.EpochGrant) bool {
		cert, err := deviceCert(tx, g.ToDevice)
		if err != nil {
			due = true // revoked
			return false
		}
		var m Member
		if tx.Get(store.Members, store.Key(sp.ID, cert.PersonID), &m) != nil || !m.Active(now) || !personActive(tx, cert.PersonID) {
			due = true
			return false
		}
		return true
	})
	return due
}

// SetChildrenChat lets (or stops) members with the child role send messages and reactions. Parents only.
func (s *Service) SetChildrenChat(c Caller, spaceID string, on bool, intent *audit.Intent) error {
	want := fmt.Sprintf("children_chat=%t", on)
	if intent == nil || intent.SpaceID != spaceID || intent.Action != audit.SpaceSettings || intent.Detail != want {
		return ErrInvalid
	}
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		sp, err := s.approvedParent(tx, c, spaceID)
		if err != nil {
			return err
		}
		sp.ChildrenChat = on
		if err := tx.Put(store.Spaces, []byte(spaceID), sp); err != nil {
			return err
		}
		if _, err := s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		people, _ = s.activeMembers(tx, spaceID)
		return nil
	})
	if err == nil {
		s.emit(people, Event{SpaceID: spaceID, Kind: "member"})
	}
	return err
}

// approvedParent checks the caller is a parent whose device holds the family's current key - i.e. they've been
// approved. A parent who joined with an invite but wasn't approved yet has no parent powers: otherwise an invite
// code that leaked could be used to throw the real parents out or take over the family's keys.
func (s *Service) approvedParent(tx *store.Tx, c Caller, spaceID string) (*Space, error) {
	me, err := s.member(tx, spaceID, c.Person)
	if err != nil {
		return nil, err
	}
	var sp Space
	if err := tx.Get(store.Spaces, []byte(spaceID), &sp); err != nil {
		return nil, ErrNotFound
	}
	if me.Role != RoleParent || !holdsEpoch(tx, spaceID, sp.Epoch, c.Device) {
		return nil, ErrForbidden
	}
	return &sp, nil
}

// personActive reports whether an account may sign in (the server's operator can block one).
func personActive(tx *store.Tx, person string) bool {
	var p Person
	return tx.Get(store.Persons, []byte(person), &p) == nil && p.Status == "active"
}

// holdsEpoch reports whether a device was given an epoch's key.
func holdsEpoch(tx *store.Tx, space string, epoch uint64, device string) bool {
	return tx.Has(store.Grants, store.Key(space, string(store.U64(epoch)), device))
}

func hasCurrentKeys(tx *store.Tx, space string, epoch uint64, person string) bool {
	found := false
	tx.Keys(store.PersonDevices, append([]byte(person), '/'), func(k []byte) bool {
		dev := string(k[len(person)+1:])
		if _, err := deviceCert(tx, dev); err != nil {
			return true // a revoked device's keys don't count
		}
		if tx.Has(store.Grants, store.Key(space, string(store.U64(epoch)), dev)) {
			found = true
			return false
		}
		return true
	})
	return found
}

// RemoveMember ends someone's access (approved parents only). The clients then rotate the epoch so the removed
// person's devices can't read anything new. An approved parent can't be removed by the other parent - neither
// co-parent can lock the other out of the family's record (the server's operator can block an account if it's ever
// needed); a parent who joined but was never approved can be removed.
func (s *Service) RemoveMember(c Caller, spaceID, person string, intent *audit.Intent) error {
	if intent == nil || intent.SpaceID != spaceID || intent.Action != audit.MemberRemove || intent.Detail != "person="+person {
		return ErrInvalid
	}
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		sp, err := s.approvedParent(tx, c, spaceID)
		if err != nil {
			return err
		}
		if person == c.Person {
			return ErrForbidden
		}
		var m Member
		if err := tx.Get(store.Members, store.Key(spaceID, person), &m); err != nil {
			return ErrNotFound
		}
		if m.Role == RoleParent && m.Status == "active" && hasCurrentKeys(tx, spaceID, sp.Epoch, person) {
			return fmt.Errorf("%w: an approved parent can't be removed by the other parent", ErrForbidden)
		}
		m.Status, m.Removed = "removed", s.now()
		if err := tx.Put(store.Members, store.Key(spaceID, person), m); err != nil {
			return err
		}
		s.endConversationsOf(tx, spaceID, person)
		if _, err := s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		people, _ = s.activeMembers(tx, spaceID)
		return nil
	})
	if err == nil {
		s.emit(append(people, Member{Person: person}), Event{SpaceID: spaceID, Kind: "member"})
	}
	return err
}

// ShareKeys uploads epoch grants: either for a newly joined member's devices (same epoch, action keys.grant) or
// for every remaining device after a rotation (epoch = current+1, action epoch.rotate).
func (s *Service) ShareKeys(c Caller, spaceID string, epoch uint64, grants []keys.EpochGrant, intent *audit.Intent) error {
	if intent == nil || intent.SpaceID != spaceID {
		return ErrInvalid
	}
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		me, err := s.member(tx, spaceID, c.Person)
		if err != nil {
			return err
		}
		if !CanWrite(me.Role) || me.ReadOnly {
			return ErrForbidden
		}
		var sp Space
		if err := tx.Get(store.Spaces, []byte(spaceID), &sp); err != nil {
			return ErrNotFound
		}
		// Only a device that holds a key may hand it on, and only one that holds the current key may replace it:
		// otherwise someone without the keys could plant a key they made (and so read what's written under it).
		switch {
		case intent.Action == audit.EpochRotate && !holdsEpoch(tx, spaceID, sp.Epoch, c.Device),
			intent.Action == audit.KeysGrant && !holdsEpoch(tx, spaceID, epoch, c.Device):
			return ErrForbidden
		case epoch == sp.Epoch+1 && intent.Action == audit.EpochRotate && (me.Role == RoleParent || me.Role == RoleParticipant && sp.Parent != ""):
			if err := s.requireAllDevices(tx, spaceID, epoch, grants); err != nil {
				return err
			}
			sp.Epoch, sp.EpochSince = epoch, s.now()
			if err := tx.Put(store.Spaces, []byte(spaceID), sp); err != nil {
				return err
			}
		case epoch >= 1 && epoch <= sp.Epoch && intent.Action == audit.KeysGrant:
		default:
			return ErrInvalid
		}
		if err := s.storeGrants(tx, c, spaceID, epoch, grants); err != nil {
			return err
		}
		if _, err := s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		people, _ = s.activeMembers(tx, spaceID)
		return nil
	})
	if err == nil {
		s.emit(people, Event{SpaceID: spaceID, Kind: "grant"})
	}
	return err
}

// requireAllDevices checks that a rotation re-keys exactly the devices that hold the current epoch and still may:
// every one of them (so nobody approved is locked out), and no others (a rotation never approves a new device -
// one added by recovery, say, waits for a parent to compare safety numbers and approve it).
func (s *Service) requireAllDevices(tx *store.Tx, spaceID string, epoch uint64, grants []keys.EpochGrant) error {
	keyed := map[string]bool{}
	for _, d := range s.keyedDevices(tx, spaceID, epoch-1) {
		keyed[d] = true
	}
	have := map[string]bool{}
	for _, g := range grants {
		if !keyed[g.ToDevice] {
			return fmt.Errorf("%w: a rotation can't share keys with a device that isn't approved", ErrInvalid)
		}
		have[g.ToDevice] = true
	}
	for d := range keyed {
		if !have[d] {
			return fmt.Errorf("%w: rotation must include every approved device of every member", ErrInvalid)
		}
	}
	return nil
}

// keyedDevices lists the devices that hold an epoch's key and still may: not revoked, of a member with access now.
func (s *Service) keyedDevices(tx *store.Tx, spaceID string, epoch uint64) []string {
	var out []string
	now := s.now()
	_ = store.Scan(tx, store.Grants, append(store.Key(spaceID, string(store.U64(epoch))), '/'), nil, func(_ []byte, g *keys.EpochGrant) bool {
		cert, err := deviceCert(tx, g.ToDevice)
		if err != nil {
			return true
		}
		var m Member
		if tx.Get(store.Members, store.Key(spaceID, cert.PersonID), &m) == nil && m.Active(now) && personActive(tx, cert.PersonID) {
			out = append(out, g.ToDevice)
		}
		return true
	})
	return out
}

// storeGrants validates and saves grants: from the caller's device, to devices of active members, for this space.
func (s *Service) storeGrants(tx *store.Tx, c Caller, spaceID string, epoch uint64, grants []keys.EpochGrant) error {
	if len(grants) == 0 || len(grants) > 500 {
		return ErrInvalid
	}
	from, err := deviceCert(tx, c.Device)
	if err != nil {
		return err
	}
	for i := range grants {
		g := &grants[i]
		if g.SpaceID != spaceID || g.Epoch != epoch || g.FromDevice != c.Device {
			return ErrInvalid
		}
		to, err := deviceCert(tx, g.ToDevice)
		if err != nil {
			return ErrInvalid
		}
		m, err := s.member(tx, spaceID, to.PersonID)
		if err != nil {
			return ErrInvalid
		}
		if m.ReadOnly != g.ReadOnly || !m.NotAfter.Equal(g.NotAfter) {
			return fmt.Errorf("%w: grant terms must match the member's access", ErrInvalid)
		}
		if err := verifyGrantSig(g, from); err != nil {
			return ErrInvalid
		}
		if err := tx.Put(store.Grants, store.Key(spaceID, string(store.U64(epoch)), g.ToDevice), g); err != nil {
			return err
		}
	}
	return nil
}

// MyGrants returns every grant addressed to the caller's device in a space.
func (s *Service) MyGrants(c Caller, spaceID string) ([]keys.EpochGrant, error) {
	var out []keys.EpochGrant
	err := s.db.View(func(tx *store.Tx) error {
		if _, err := s.member(tx, spaceID, c.Person); err != nil {
			return err
		}
		return store.Scan(tx, store.Grants, append([]byte(spaceID), '/'), nil, func(k []byte, g *keys.EpochGrant) bool {
			if g.ToDevice == c.Device {
				out = append(out, *g)
			}
			return true
		})
	})
	return out, err
}
