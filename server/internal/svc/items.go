package svc

import (
	"slices"
	"time"

	"pp/core/audit"
	"pp/core/envelope"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/server/internal/store"
)

func verifyGrantSig(g *keys.EpochGrant, from *keys.VerifiedDevice) error {
	return g.VerifySignature(from)
}

// PostItem stores a new item or a new revision of one. The server can't read it, but checks that:
//   - the caller is an active, writing member, and the item is signed by the caller's own device
//   - it's encrypted under the space's current epoch
//   - a revision follows the latest stored revision exactly (same author, hash-linked, no gaps)
//   - the audit intent (item.create / item.revise) names this exact revision
//
// The item and its audit entry are stored in one transaction: either both exist or neither does.
func (s *Service) PostItem(c Caller, it *envelope.Item, intent *audit.Intent) (*ItemRec, *audit.Entry, error) {
	if it == nil || intent == nil || it.SpaceID != intent.SpaceID {
		return nil, nil, ErrInvalid
	}
	if it.Person != c.Person || it.Device != c.Device {
		return nil, nil, ErrForbidden
	}
	want := audit.ItemCreate
	if it.Rev > 0 {
		want = audit.ItemRevise
	}
	h := it.Hash()
	if intent.Action != want || intent.ItemID != it.ItemID || intent.ItemRev != it.Rev || !pqc.Equal(intent.ItemHash, h[:]) ||
		intent.Detail != "kind="+it.Kind {
		return nil, nil, ErrInvalid
	}
	var rec *ItemRec
	var entry *audit.Entry
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		m, err := s.member(tx, it.SpaceID, c.Person)
		if err != nil {
			return err
		}
		var sp Space
		if err := tx.Get(store.Spaces, []byte(it.SpaceID), &sp); err != nil {
			return ErrNotFound
		}
		if err := s.mayPost(tx, m, &sp, it.Kind); err != nil {
			return err
		}
		if it.Epoch != sp.Epoch {
			return ErrConflict // keys were rotated: the client fetches the new epoch and re-seals
		}
		dev, err := deviceCert(tx, c.Device)
		if err != nil {
			return err
		}
		if envelope.VerifySignature(it, dev) != nil {
			return ErrInvalid
		}
		revKey := store.KeySeq(it.Rev, it.SpaceID, it.ItemID)
		if tx.Has(store.ItemIndex, revKey) {
			return ErrConflict
		}
		if it.Rev > 0 {
			var prevSeq uint64
			if err := tx.Get(store.ItemIndex, store.KeySeq(it.Rev-1, it.SpaceID, it.ItemID), &prevSeq); err != nil {
				return ErrConflict
			}
			var prev ItemRec
			if err := tx.Get(store.Items, store.KeySeq(prevSeq, it.SpaceID), &prev); err != nil {
				return err
			}
			if err := envelope.Follows(&prev.Item, it); err != nil {
				return ErrInvalid
			}
		} else if it.Ref != "" {
			// Reactions and replies must point at an item that exists in this space.
			if !hasItem(tx, it.SpaceID, it.Ref) {
				return ErrInvalid
			}
		}
		seq, err := tx.NextSeq("items/" + it.SpaceID)
		if err != nil {
			return err
		}
		rec = &ItemRec{Seq: seq, Item: *it}
		if err := tx.Put(store.Items, store.KeySeq(seq, it.SpaceID), rec); err != nil {
			return err
		}
		if err := tx.Put(store.ItemIndex, revKey, seq); err != nil {
			return err
		}
		if entry, err = s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		people, _ = s.activeMembers(tx, it.SpaceID)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	s.emit(people, Event{SpaceID: it.SpaceID, Kind: "item", Seq: rec.Seq})
	return rec, entry, nil
}

// mayPost applies the family's permissions to adding an item of a kind (see Perms).
func (s *Service) mayPost(tx *store.Tx, m *Member, sp *Space, kind string) error {
	if sp.Parent != "" { // a private conversation: chat only, while the family still allows it
		switch kind {
		case envelope.KindMessage, envelope.KindReaction, envelope.KindCall, envelope.KindRecord:
		default:
			return ErrForbidden
		}
		return s.conversationAllows(tx, sp, m.Person, false)
	}
	switch {
	case m.Role == RoleParent:
		return nil
	case kind == envelope.KindRecord:
		return nil // their name in the family (and older info bank entries)
	case parentsOnly(kind):
		return ErrForbidden
	}
	sec := sectionOf(kind)
	if sec == "" || !slices.Contains(permsOf(m, sp).Add, sec) {
		return ErrForbidden
	}
	return nil
}

func hasItem(tx *store.Tx, space, item string) bool {
	found := false
	tx.Keys(store.ItemIndex, append([]byte(space+"/"+item), '/'), func([]byte) bool { found = true; return false })
	return found
}

// Items returns item revisions after the given sequence number (the client's sync cursor), oldest first.
func (s *Service) Items(c Caller, spaceID string, after uint64, limit int) ([]ItemRec, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	var out []ItemRec
	err := s.db.View(func(tx *store.Tx) error {
		m, err := s.member(tx, spaceID, c.Person)
		if err != nil {
			return err
		}
		var sp Space
		if err := tx.Get(store.Spaces, []byte(spaceID), &sp); err != nil {
			return ErrNotFound
		}
		see := map[string]bool{}
		for _, sec := range permsOf(m, &sp).See {
			see[sec] = true
		}
		limited := sp.Parent == "" && m.Role != RoleParent
		return store.Scan(tx, store.Items, append([]byte(spaceID), '/'), store.KeySeq(after+1, spaceID), func(_ []byte, r *ItemRec) bool {
			if !m.HistoryFrom.IsZero() && !s.inWindow(tx, m, &r.Item) {
				return true // outside a professional's history window (the cursor still moves past it)
			}
			if sec := sectionOf(r.Item.Kind); limited && sec != "" && !see[sec] {
				return true // a section the inviting parent didn't share with them
			}
			out = append(out, *r)
			return len(out) < limit
		})
	})
	return out, err
}

// inWindow reports whether a member with a history window may see an item revision: the item was first created
// inside the window, and so was the item it answers or reacts to.
func (s *Service) inWindow(tx *store.Tx, m *Member, it *envelope.Item) bool {
	if !m.Sees(firstCreated(tx, it.SpaceID, it.ItemID, it)) {
		return false
	}
	return it.Ref == "" || m.Sees(firstCreated(tx, it.SpaceID, it.Ref, nil))
}

// firstCreated is when an item's first revision was made (self: the revision at hand, used when it is the first).
func firstCreated(tx *store.Tx, space, item string, self *envelope.Item) time.Time {
	if self != nil && self.Rev == 0 {
		return self.Created
	}
	var seq uint64
	if tx.Get(store.ItemIndex, store.KeySeq(0, space, item), &seq) != nil {
		return time.Time{} // unknown: treated as outside any window
	}
	var r ItemRec
	if tx.Get(store.Items, store.KeySeq(seq, space), &r) != nil {
		return time.Time{}
	}
	return r.Item.Created
}

// Record appends a device-signed intent that isn't tied to storing an item: a read receipt (item.view), a file
// download, a report export, a call start/end.
func (s *Service) Record(c Caller, intent *audit.Intent) (*audit.Entry, error) {
	switch intent.Action {
	case audit.ItemView, audit.BlobDownload, audit.ReportExport, audit.CallStart, audit.CallEnd:
	default:
		return nil, ErrInvalid
	}
	var e *audit.Entry
	var people []Member
	err := s.db.Update(func(tx *store.Tx) error {
		if _, err := s.member(tx, intent.SpaceID, c.Person); err != nil {
			return err
		}
		if intent.Action == audit.ItemView {
			// A receipt names an existing revision, and each person records the first view of it once.
			if !tx.Has(store.ItemIndex, store.KeySeq(intent.ItemRev, intent.SpaceID, intent.ItemID)) {
				return ErrInvalid
			}
			rk := store.Key("view", intent.SpaceID, intent.ItemID, string(store.U64(intent.ItemRev)), c.Person)
			if tx.Has(store.Seqs, rk) {
				return ErrConflict
			}
			if err := tx.Put(store.Seqs, rk, true); err != nil {
				return err
			}
		}
		var err error
		if e, err = s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		people, _ = s.activeMembers(tx, intent.SpaceID)
		return nil
	})
	if err == nil {
		s.emit(people, Event{SpaceID: intent.SpaceID, Kind: "audit", Seq: e.Seq})
	}
	return e, err
}

// Audit returns audit entries from a sequence number (inclusive), oldest first.
func (s *Service) Audit(c Caller, spaceID string, from uint64, limit int) ([]audit.Entry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	if from < 1 {
		from = 1
	}
	var out []audit.Entry
	err := s.db.View(func(tx *store.Tx) error {
		if _, err := s.member(tx, spaceID, c.Person); err != nil {
			return err
		}
		return store.Scan(tx, store.Audit, append([]byte(spaceID), '/'), store.KeySeq(from, spaceID), func(_ []byte, e *audit.Entry) bool {
			out = append(out, *e)
			return len(out) < limit
		})
	})
	return out, err
}
