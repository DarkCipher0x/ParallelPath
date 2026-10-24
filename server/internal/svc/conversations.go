package svc

import (
	"fmt"
	"slices"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/wire"
	"pp/server/internal/store"
)

// Private conversations: a space of their own inside a family, for some of its members (two parents and a
// mediator, a child and one parent...). They have their own epoch keys, granted only to the participants'
// devices, so nobody else in the family - and not the server - can read them. Who may be in one follows the
// family's permissions (Perms.Message), checked when it's made and again on every post, so taking a permission
// away takes effect at once.

// MaxParticipants is the largest private conversation.
const MaxParticipants = 8

// CreateConversation makes a private conversation in a family between the caller and others. grants carry the
// conversation's first epoch key (made on the caller's device) to every participant's devices.
func (s *Service) CreateConversation(c Caller, family, convID string, participants []string, intent *audit.Intent, grants []keys.EpochGrant) error {
	if wire.CheckID(convID) != nil || intent == nil || intent.SpaceID != convID || intent.Action != audit.SpaceCreate ||
		intent.Detail != "parent="+family {
		return ErrInvalid
	}
	people := []string{c.Person}
	for _, p := range participants {
		if wire.CheckID(p) != nil || slices.Contains(people, p) {
			return ErrInvalid
		}
		people = append(people, p)
	}
	if len(people) < 2 || len(people) > MaxParticipants {
		return fmt.Errorf("%w: a conversation has 2 to %d people", ErrInvalid, MaxParticipants)
	}
	var notify []Member
	err := s.db.Update(func(tx *store.Tx) error {
		if tx.Has(store.Spaces, []byte(convID)) {
			return ErrConflict
		}
		var fam Space
		if err := tx.Get(store.Spaces, []byte(family), &fam); err != nil || fam.Parent != "" || fam.Journal {
			return ErrNotFound
		}
		members := make([]Member, 0, len(people))
		for _, p := range people {
			m, err := s.member(tx, family, p)
			if err != nil {
				return ErrForbidden
			}
			if !hasCurrentKeys(tx, family, fam.Epoch, p) {
				return fmt.Errorf("%w: everyone in a conversation must have been approved in the family", ErrForbidden)
			}
			members = append(members, *m)
		}
		if !holdsEpoch(tx, family, fam.Epoch, c.Device) {
			return ErrForbidden
		}
		if err := mayTalk(members, &fam, 0, len(members)); err != nil {
			return err
		}
		now := s.now()
		if err := tx.Put(store.Spaces, []byte(convID), Space{ID: convID, Created: now, CreatedBy: c.Person, Epoch: 1, EpochSince: now,
			Parent: family}); err != nil {
			return err
		}
		for _, fm := range members {
			m := Member{Person: fm.Person, Role: RoleParticipant, Status: "active", Joined: now, NotAfter: fm.NotAfter}
			if err := tx.Put(store.Members, store.Key(convID, fm.Person), m); err != nil {
				return err
			}
			if err := tx.Put(store.PersonSpaces, store.Key(fm.Person, convID), true); err != nil {
				return err
			}
			notify = append(notify, m)
		}
		if err := s.storeGrants(tx, c, convID, 1, grants); err != nil {
			return err
		}
		_, err := s.appendAudit(tx, c, intent)
		return err
	})
	if err == nil {
		s.emit(notify, Event{SpaceID: convID, Kind: "member"})
	}
	return err
}

// mayTalk checks the family's permissions allow members[from:to] (each of them) to message every other member.
func mayTalk(members []Member, fam *Space, from, to int) error {
	for i := from; i < to; i++ {
		a := &members[i]
		p := permsOf(a, fam)
		for j := range members {
			b := &members[j]
			if i == j {
				continue
			}
			if !reaches(p.Message, p.MessageWith, b.Person, b.Role) {
				return fmt.Errorf("%w: the family's permissions don't allow these people a private conversation", ErrForbidden)
			}
		}
	}
	return nil
}

// conversationAllows checks, for a post or a call in a conversation, that the family still allows the caller to
// reach everyone else in it (call: by the Call rule, else the Message rule).
func (s *Service) conversationAllows(tx *store.Tx, conv *Space, person string, call bool) error {
	var fam Space
	if err := tx.Get(store.Spaces, []byte(conv.Parent), &fam); err != nil {
		return ErrNotFound
	}
	me, err := s.member(tx, conv.Parent, person)
	if err != nil {
		return ErrForbidden // no longer in the family
	}
	p := permsOf(me, &fam)
	now := s.now()
	var bad error
	_ = store.Scan(tx, store.Members, append([]byte(conv.ID), '/'), nil, func(_ []byte, m *Member) bool {
		if m.Person == person || !m.Active(now) {
			return true
		}
		var fm Member
		if tx.Get(store.Members, store.Key(conv.Parent, m.Person), &fm) != nil || !fm.Active(now) {
			return true // they've left the family; nothing reaches them anyway
		}
		ok := reaches(p.Message, p.MessageWith, fm.Person, fm.Role)
		if call {
			ok = reaches(p.Call, p.CallWith, fm.Person, fm.Role)
		}
		if !ok {
			bad = fmt.Errorf("%w: the family's permissions no longer allow this", ErrForbidden)
			return false
		}
		return true
	})
	return bad
}

// endConversationsOf marks a person removed from every conversation of a family (when they leave the family);
// the other participants' devices then rotate those conversations' keys (rotateDue).
func (s *Service) endConversationsOf(tx *store.Tx, family, person string) []string {
	var convs []string
	tx.Keys(store.PersonSpaces, append([]byte(person), '/'), func(k []byte) bool {
		id := string(k[len(person)+1:])
		var sp Space
		if tx.Get(store.Spaces, []byte(id), &sp) == nil && sp.Parent == family {
			convs = append(convs, id)
		}
		return true
	})
	for _, id := range convs {
		var m Member
		if tx.Get(store.Members, store.Key(id, person), &m) == nil && m.Status == "active" {
			m.Status, m.Removed = "removed", s.now()
			_ = tx.Put(store.Members, store.Key(id, person), m)
		}
	}
	return convs
}

// SetPermissions changes what a member may do in a family. Only the parent who invited them may (for members from
// in earlier test builds, any approved parent), and parents themselves can't be limited. Recorded in the family's audit log.
func (s *Service) SetPermissions(c Caller, family, person string, p Perms, intent *audit.Intent) error {
	if intent == nil || intent.SpaceID != family || intent.Action != audit.MemberPerms || intent.Detail != "person="+person {
		return ErrInvalid
	}
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		sp, err := s.approvedParent(tx, c, family)
		if err != nil {
			return err
		}
		if sp.Parent != "" {
			return ErrInvalid
		}
		var m Member
		if err := tx.Get(store.Members, store.Key(family, person), &m); err != nil || m.Status != "active" {
			return ErrNotFound
		}
		if m.Role == RoleParent {
			return fmt.Errorf("%w: parents always have every permission", ErrInvalid)
		}
		if m.InvitedBy != "" && m.InvitedBy != c.Person {
			return fmt.Errorf("%w: only the parent who invited them can change their permissions", ErrForbidden)
		}
		if err := p.check(m.Role); err != nil {
			return err
		}
		m.Perms = &p
		if err := tx.Put(store.Members, store.Key(family, person), m); err != nil {
			return err
		}
		if _, err := s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		people, _ = s.activeMembers(tx, family)
		return nil
	})
	if err == nil {
		s.emit(people, Event{SpaceID: family, Kind: "member"})
	}
	return err
}
