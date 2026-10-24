package svc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"

	"pp/core/wire"
	"pp/server/internal/store"
)

// Calls: the server never sees a call. It relays the (end-to-end encrypted) call-setup messages between a family's
// devices, and hands out short-lived credentials for its TURN relay, which forwards media it can't decrypt.

// Signal is one relayed call-setup message.
type Signal struct {
	SpaceID    string `json:"space_id"`
	FromPerson string `json:"from_person"`
	FromDevice string `json:"from_device"`
	ToDevice   string `json:"to_device,omitempty"` // empty: every device in the family
	Payload    []byte `json:"payload"`             // sealed by the sender with a key from the family's epoch key
}

// MaxSignal is the largest call-setup message (an SDP offer is a few KB).
const MaxSignal = 64 << 10

// Relay delivers a call-setup message from an active member to the family's connected devices (or one device).
// A device may send at most signalBurst messages in any signalWindow (a call needs a few dozen to connect).
func (s *Service) Relay(c Caller, spaceID, toDevice string, payload []byte) error {
	if !s.signals.allow(c.Device) {
		return ErrRateLimited
	}
	if len(payload) == 0 || len(payload) > MaxSignal || (toDevice != "" && wire.CheckID(toDevice) != nil) {
		return ErrInvalid
	}
	var people []Member
	err := s.db.View(func(tx *store.Tx) error {
		m, err := s.member(tx, spaceID, c.Person)
		if err != nil {
			return err
		}
		var sp Space
		if err := tx.Get(store.Spaces, []byte(spaceID), &sp); err != nil {
			return ErrNotFound
		}
		// Calls follow the family's permissions: in a private conversation, the caller must be allowed to call
		// everyone in it; a call to the whole family needs FamilyCall (or being allowed to call anyone).
		if sp.Parent != "" {
			if err := s.conversationAllows(tx, &sp, c.Person, true); err != nil {
				return err
			}
		} else if p := permsOf(m, &sp); m.Role != RoleParent && !p.FamilyCall && p.Call != ReachAnyone {
			return ErrForbidden
		}
		people, err = s.activeMembers(tx, spaceID)
		return err
	})
	if err != nil {
		return err
	}
	s.emit(people, Event{SpaceID: spaceID, Kind: "signal",
		Signal: &Signal{SpaceID: spaceID, FromPerson: c.Person, FromDevice: c.Device, ToDevice: toDevice, Payload: payload}})
	return nil
}

// TurnCredentials are time-limited credentials for the server's TURN relay (the "TURN REST API" scheme: the
// username carries the expiry and the person, the password is an HMAC of it under the server's TURN secret).
func TurnCredentials(secret []byte, person string, ttl time.Duration) (user, pass string) {
	user = strconv.FormatInt(time.Now().Add(ttl).Unix(), 10) + ":" + person
	return user, TurnPassword(secret, user)
}

// TurnPassword is the password for a TURN username.
func TurnPassword(secret []byte, user string) string {
	m := hmac.New(sha1.New, secret) // SHA-1 is what TURN's long-term credential scheme is defined with
	m.Write([]byte(user))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// TurnUserValid reports whether a TURN username hasn't expired and names a person who is still allowed in.
func (s *Service) TurnUserValid(user string) bool {
	exp, person, ok := strings.Cut(user, ":")
	if !ok {
		return false
	}
	t, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > t || wire.CheckID(person) != nil {
		return false
	}
	active := false
	_ = s.db.View(func(tx *store.Tx) error {
		var p Person
		if tx.Get(store.Persons, []byte(person), &p) == nil {
			active = p.Status == "active" || p.Status == ""
		}
		return nil
	})
	return active
}

// Config is the service's settings (the call relay's port and secret, limits).
func (s *Service) Config() Config {
	c := s.cfg
	c.TurnPort = int(s.turnPort.Load())
	return c
}

// SetTurnPort records where the call relay is listening once it has started.
func (s *Service) SetTurnPort(p int) { s.turnPort.Store(int64(p)) }

const (
	signalBurst  = 400
	signalWindow = time.Minute
)

// signalLimit counts each device's call-setup messages in the current window.
type signalLimit struct {
	mu    sync.Mutex
	start time.Time
	count map[string]int
}

func (l *signalLimit) allow(device string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Since(l.start) > signalWindow || l.count == nil {
		l.start, l.count = time.Now(), map[string]int{}
	}
	l.count[device]++
	return l.count[device] <= signalBurst
}
