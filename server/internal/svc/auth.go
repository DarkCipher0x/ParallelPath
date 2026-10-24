package svc

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"time"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/server/internal/store"
)

// Sign-in has no passwords. A device proves it holds its private key by signing a fresh server challenge
// (ML-DSA-87 + Ed25519). The signed message binds the server's own key, so a challenge answered for one server
// can't be relayed to another.

// PurposeAuth and AuthMessage are shared with the clients (core/keys), so both sides sign/verify the same bytes.
const PurposeAuth = keys.PurposeAuth

// AuthMessage is what a device signs to sign in.
var AuthMessage = keys.AuthMessage

func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// tokenKey is how a secret token is stored: a keyed hash, so a database leak reveals no usable tokens.
func (s *Service) tokenKey(kind, token string) []byte {
	h := pqc.Hash(s.sessK, []byte(kind+"|"+token))
	return []byte(hex.EncodeToString(h[:32]))
}

// CreateServerInvite makes a one-time code that lets someone create an account and their own family space.
// Only the server operator can do this (CLI or admin portal).
func (s *Service) CreateServerInvite() (string, time.Time, error) {
	token := randToken()
	inv := Invite{CreatedBy: "admin", Created: s.now(), Expires: s.now().Add(s.cfg.InviteTTL)}
	err := s.db.Update(func(tx *store.Tx) error { return tx.Put(store.Invites, s.tokenKey("invite", token), inv) })
	if err != nil {
		return "", time.Time{}, err
	}
	return keys.InviteCode{Kind: 'S', Secret: token}.String(), inv.Expires, s.AdminAudit("invite.create", "server invite")
}

// lookupInvite parses a code and loads its invite, checking the code's space/role match what was issued.
func (s *Service) lookupInvite(tx *store.Tx, code string) (*Invite, []byte, error) {
	c, err := keys.ParseInvite(code)
	if err != nil {
		return nil, nil, ErrInvalid
	}
	tk := s.tokenKey("invite", c.Secret)
	var inv Invite
	if err := tx.Get(store.Invites, tk, &inv); err != nil {
		return nil, nil, ErrNotFound
	}
	if s.now().After(inv.Expires) {
		return nil, nil, ErrExpired
	}
	if (c.Kind == 'J') != (inv.SpaceID != "") || c.Space != inv.SpaceID || c.Role != inv.Role {
		return nil, nil, ErrInvalid
	}
	return &inv, tk, nil
}

// EnrolResult is returned when an account is created.
type EnrolResult struct {
	PersonID string `json:"person_id"`
	SpaceID  string `json:"space_id,omitempty"` // set when the invite was for a family space
	Role     string `json:"role,omitempty"`
}

// Enrol creates an account from an invite and registers its first device. joinIntent (a signed member.join) is
// required for a space invite, so joining is recorded in the family's audit chain by the person who joined.
func (s *Service) Enrol(invite string, cert keys.DeviceCert, joinIntent *audit.Intent) (*EnrolResult, error) {
	v, err := cert.Verify()
	if err != nil {
		return nil, ErrInvalid
	}
	res := &EnrolResult{PersonID: v.PersonID}
	var people []Member
	err = s.db.Update(func(tx *store.Tx) error {
		inv, tk, err := s.lookupInvite(tx, invite)
		if err != nil {
			return err
		}
		if tx.Has(store.Persons, []byte(v.PersonID)) || tx.Has(store.Devices, []byte(v.DeviceID)) {
			return ErrConflict
		}
		// A server invite brings one new family onto the server; a family invite brings someone into a family.
		families := 0
		if inv.SpaceID == "" {
			families = 1
		}
		if err := tx.Put(store.Persons, []byte(v.PersonID), Person{ID: v.PersonID, Created: s.now(), Status: "active",
			FamiliesLeft: &families}); err != nil {
			return err
		}
		if err := tx.Put(store.Devices, []byte(v.DeviceID), Device{Cert: cert, Added: s.now()}); err != nil {
			return err
		}
		if err := tx.Put(store.PersonDevices, store.Key(v.PersonID, v.DeviceID), true); err != nil {
			return err
		}
		if inv.SpaceID != "" {
			m := Member{Person: v.PersonID, Role: inv.Role, Status: "active", Joined: s.now(), ReadOnly: inv.ReadOnly, NotAfter: inv.NotAfter,
				HistoryFrom: inv.HistoryFrom, Perms: inv.Perms, InvitedBy: inv.CreatedBy}
			if err := tx.Put(store.Members, store.Key(inv.SpaceID, v.PersonID), m); err != nil {
				return err
			}
			if err := tx.Put(store.PersonSpaces, store.Key(v.PersonID, inv.SpaceID), true); err != nil {
				return err
			}
			if joinIntent == nil || joinIntent.SpaceID != inv.SpaceID || joinIntent.Action != audit.MemberJoin || joinIntent.Detail != "role="+inv.Role {
				return ErrInvalid
			}
			if _, err := s.appendAudit(tx, Caller{v.PersonID, v.DeviceID}, joinIntent); err != nil {
				return err
			}
			res.SpaceID, res.Role = inv.SpaceID, inv.Role
			people, _ = s.activeMembers(tx, inv.SpaceID)
		}
		return tx.Delete(store.Invites, tk) // one use only
	})
	if err != nil {
		return nil, err
	}
	if res.SpaceID != "" {
		s.emit(people, Event{SpaceID: res.SpaceID, Kind: "member"})
	}
	return res, nil
}

// AddDevice registers another device of an already signed-in person (its certificate is signed by the same
// identity key; the identity key is moved between the person's own devices by the clients, never via the server).
func (s *Service) AddDevice(c Caller, cert keys.DeviceCert) error {
	v, err := cert.Verify()
	if err != nil || v.PersonID != c.Person {
		return ErrInvalid
	}
	return s.db.Update(func(tx *store.Tx) error {
		if tx.Has(store.Devices, []byte(v.DeviceID)) {
			return ErrConflict
		}
		if err := tx.Put(store.Devices, []byte(v.DeviceID), Device{Cert: cert, Added: s.now()}); err != nil {
			return err
		}
		return tx.Put(store.PersonDevices, store.Key(v.PersonID, v.DeviceID), true)
	})
}

// Challenge issues a one-time sign-in nonce for a registered device.
func (s *Service) Challenge(deviceID string) ([]byte, error) {
	nonce := pqc.NewKey()
	err := s.db.Update(func(tx *store.Tx) error {
		if _, err := deviceCert(tx, deviceID); err != nil {
			return ErrUnauthorized
		}
		return tx.Put(store.Challenges, []byte(deviceID), Challenge{Device: deviceID, Nonce: nonce, Expires: s.now().Add(s.cfg.ChallengeTTL)})
	})
	return nonce, err
}

// SignIn checks the device's signature over the challenge and returns a session token.
func (s *Service) SignIn(deviceID string, sig []byte) (string, *Caller, error) {
	token := randToken()
	var caller *Caller
	err := s.db.Update(func(tx *store.Tx) error {
		var ch Challenge
		if err := tx.Get(store.Challenges, []byte(deviceID), &ch); err != nil {
			return ErrUnauthorized
		}
		_ = tx.Delete(store.Challenges, []byte(deviceID)) // single use, whatever happens next
		if s.now().After(ch.Expires) {
			return ErrUnauthorized
		}
		dev, err := deviceCert(tx, deviceID)
		if err != nil {
			return ErrUnauthorized
		}
		if dev.Sign.Verify(PurposeAuth, AuthMessage(s.server.Public(), deviceID, ch.Nonce), sig) != nil {
			return ErrUnauthorized
		}
		var p Person
		if err := tx.Get(store.Persons, []byte(dev.PersonID), &p); err != nil || p.Status != "active" {
			return ErrForbidden
		}
		caller = &Caller{Person: dev.PersonID, Device: deviceID}
		return tx.Put(store.Sessions, s.tokenKey("session", token), Session{Person: dev.PersonID, Device: deviceID,
			Created: s.now(), Expires: s.now().Add(s.cfg.SessionTTL)})
	})
	if err != nil {
		return "", nil, err
	}
	return token, caller, nil
}

// Authenticate resolves a bearer token to its device.
func (s *Service) Authenticate(token string) (*Caller, error) {
	if len(token) < 20 || len(token) > 100 {
		return nil, ErrUnauthorized
	}
	var sess Session
	err := s.db.View(func(tx *store.Tx) error {
		if err := tx.Get(store.Sessions, s.tokenKey("session", token), &sess); err != nil {
			return ErrUnauthorized
		}
		if s.now().After(sess.Expires) {
			return ErrUnauthorized
		}
		if _, err := deviceCert(tx, sess.Device); err != nil {
			return ErrUnauthorized
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Caller{Person: sess.Person, Device: sess.Device}, nil
}

// SignOut ends a session.
func (s *Service) SignOut(token string) error {
	return s.db.Update(func(tx *store.Tx) error { return tx.Delete(store.Sessions, s.tokenKey("session", token)) })
}

// Devices returns the verified certificates of a person's active devices. Only people who share a space with
// them (or they themselves) may look.
func (s *Service) Devices(c Caller, person string) ([]keys.DeviceCert, error) {
	var out []keys.DeviceCert
	err := s.db.View(func(tx *store.Tx) error {
		if person != c.Person && !s.shareSpace(tx, c.Person, person) {
			return ErrForbidden
		}
		var ids []string
		tx.Keys(store.PersonDevices, append([]byte(person), '/'), func(k []byte) bool {
			ids = append(ids, string(k[len(person)+1:]))
			return true
		})
		for _, id := range ids {
			var d Device
			if err := tx.Get(store.Devices, []byte(id), &d); err == nil && d.Revoked == nil {
				out = append(out, d.Cert)
			}
		}
		return nil
	})
	return out, err
}

// DeviceCertByID returns one device's certificate (to verify an author or audit entry), same visibility rule.
func (s *Service) DeviceCertByID(c Caller, deviceID string) (*keys.DeviceCert, bool, error) {
	var d Device
	err := s.db.View(func(tx *store.Tx) error {
		if err := tx.Get(store.Devices, []byte(deviceID), &d); err != nil {
			return ErrNotFound
		}
		if d.Cert.PersonID != c.Person && !s.shareSpace(tx, c.Person, d.Cert.PersonID) {
			return ErrForbidden
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &d.Cert, d.Revoked != nil, nil
}

func (s *Service) shareSpace(tx *store.Tx, a, b string) bool {
	found := false
	tx.Keys(store.PersonSpaces, append([]byte(a), '/'), func(k []byte) bool {
		space := string(k[len(a)+1:])
		if tx.Has(store.Members, store.Key(space, b)) {
			found = true
			return false
		}
		return true
	})
	return found
}

// TokenKey is how other parts of the server store secret tokens (the admin portal's sessions and links): the same
// keyed hash as device sessions, under its own kind.
func (s *Service) TokenKey(kind, token string) []byte { return s.tokenKey(kind, token) }

// RandToken makes a 256-bit random token.
func RandToken() string { return randToken() }
