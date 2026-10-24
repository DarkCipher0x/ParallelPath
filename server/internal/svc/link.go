package svc

import (
	"encoding/hex"
	"sync"
	"time"
)

// Linking a new device to an existing person: a short-lived relay for two messages, both end-to-end encrypted by
// the devices with a key from the one-time link code (which the server never sees - slots are named by a hash of
// it). The person's existing device opens the slot, so only that person can read the request and answer it; the new
// device posts its public keys; the existing device answers with a certificate for it and the person's keys. Slots
// live in memory only, for ten minutes, and the answer can be fetched once.

const (
	linkTTL     = 10 * time.Minute
	maxLinkBody = 64 << 10
	maxLinks    = 1000
)

type linkSlot struct {
	owner           string // the person linking a device
	request, answer []byte
	created         time.Time
}

type links struct {
	mu    sync.Mutex
	slots map[string]*linkSlot
}

func (l *links) get(id string) *linkSlot {
	if l.slots == nil {
		l.slots = map[string]*linkSlot{}
	}
	for k, s := range l.slots {
		if time.Since(s.created) > linkTTL {
			delete(l.slots, k)
		}
	}
	return l.slots[id]
}

func linkID(id string) bool { b, err := hex.DecodeString(id); return err == nil && len(b) == 16 }

// LinkOpen opens a slot for the caller's link code.
func (s *Service) LinkOpen(c Caller, id string) error {
	if !linkID(id) {
		return ErrInvalid
	}
	s.links.mu.Lock()
	defer s.links.mu.Unlock()
	if s.links.get(id) != nil {
		return ErrConflict
	}
	if len(s.links.slots) >= maxLinks {
		return ErrRateLimited
	}
	s.links.slots[id] = &linkSlot{owner: c.Person, created: time.Now()}
	return nil
}

// LinkRequest stores a new device's (encrypted) request in an open slot (once).
func (s *Service) LinkRequest(id string, payload []byte) error {
	if !linkID(id) || len(payload) == 0 || len(payload) > maxLinkBody {
		return ErrInvalid
	}
	s.links.mu.Lock()
	defer s.links.mu.Unlock()
	slot := s.links.get(id)
	if slot == nil {
		return ErrNotFound
	}
	if slot.request != nil {
		return ErrConflict
	}
	slot.request = payload
	return nil
}

func (s *Service) ownSlot(c Caller, id string) (*linkSlot, error) {
	if !linkID(id) {
		return nil, ErrInvalid
	}
	slot := s.links.get(id)
	if slot == nil || slot.owner != c.Person {
		return nil, ErrNotFound
	}
	return slot, nil
}

// LinkPending returns the request in the caller's own slot (404 until the new device has asked).
func (s *Service) LinkPending(c Caller, id string) ([]byte, error) {
	s.links.mu.Lock()
	defer s.links.mu.Unlock()
	slot, err := s.ownSlot(c, id)
	if err != nil {
		return nil, err
	}
	if slot.request == nil {
		return nil, ErrNotFound
	}
	return slot.request, nil
}

// LinkAnswer stores the existing device's (encrypted) answer in its own slot.
func (s *Service) LinkAnswer(c Caller, id string, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxLinkBody {
		return ErrInvalid
	}
	s.links.mu.Lock()
	defer s.links.mu.Unlock()
	slot, err := s.ownSlot(c, id)
	if err != nil {
		return err
	}
	if slot.request == nil || slot.answer != nil {
		return ErrConflict
	}
	slot.answer = payload
	return nil
}

// LinkResult hands the answer to the new device, once, and closes the slot (409 while not answered yet).
func (s *Service) LinkResult(id string) ([]byte, error) {
	if !linkID(id) {
		return nil, ErrInvalid
	}
	s.links.mu.Lock()
	defer s.links.mu.Unlock()
	slot := s.links.get(id)
	if slot == nil {
		return nil, ErrNotFound
	}
	if slot.answer == nil {
		return nil, ErrConflict
	}
	delete(s.links.slots, id)
	return slot.answer, nil
}
