package svc

import (
	"encoding/hex"
	"time"

	"pp/core/keys"
	"pp/server/internal/store"
)

// Recovery kits. A person can keep a sealed copy of their identity on the server, encrypted on their device with a
// key derived (Argon2id) from a recovery code only they have; the server stores it under a hash of the code and can't
// read it. With the code, a new device fetches the kit, certifies itself with the identity inside, and registers -
// which also revokes every other device of that person, in case the lost one is in someone else's hands. The
// families then see the recovered person waiting for approval, as with a new device.

// RecoveryKit is a stored, sealed kit.
type RecoveryKit struct {
	Person  string    `json:"person"`
	Sealed  []byte    `json:"sealed"`
	Updated time.Time `json:"updated"`
}

const maxRecoveryKit = 64 << 10

func recoveryID(id string) bool { b, err := hex.DecodeString(id); return err == nil && len(b) == 16 }

// RecoverySave stores (or replaces) the caller's kit.
func (s *Service) RecoverySave(c Caller, id string, sealed []byte) error {
	if !recoveryID(id) || len(sealed) == 0 || len(sealed) > maxRecoveryKit {
		return ErrInvalid
	}
	return s.db.Update(func(tx *store.Tx) error {
		var old string
		if tx.Get(store.RecoveryOf, []byte(c.Person), &old) == nil && old != id {
			_ = tx.Delete(store.Recovery, []byte(old))
		}
		var other RecoveryKit
		if tx.Get(store.Recovery, []byte(id), &other) == nil && other.Person != c.Person {
			return ErrConflict
		}
		if err := tx.Put(store.Recovery, []byte(id), RecoveryKit{Person: c.Person, Sealed: sealed, Updated: s.now()}); err != nil {
			return err
		}
		return tx.Put(store.RecoveryOf, []byte(c.Person), id)
	})
}

// RecoveryFetch returns a sealed kit to whoever has its code (rate-limited by the API).
func (s *Service) RecoveryFetch(id string) ([]byte, error) {
	if !recoveryID(id) {
		return nil, ErrInvalid
	}
	var k RecoveryKit
	err := s.db.View(func(tx *store.Tx) error { return tx.Get(store.Recovery, []byte(id), &k) })
	if err != nil {
		return nil, ErrNotFound
	}
	return k.Sealed, nil
}

// RecoveryRegister registers a new device certified by the kit's identity, and revokes the person's other devices.
func (s *Service) RecoveryRegister(id string, cert keys.DeviceCert) error {
	if !recoveryID(id) {
		return ErrInvalid
	}
	v, err := cert.Verify()
	if err != nil {
		return ErrInvalid
	}
	if time.Since(cert.Issued) > 10*time.Minute || time.Until(cert.Issued) > 5*time.Minute {
		return ErrInvalid // a fresh certificate only (made just now, by the person holding the kit's identity)
	}
	revoked := 0
	err = s.db.Update(func(tx *store.Tx) error {
		var k RecoveryKit
		if tx.Get(store.Recovery, []byte(id), &k) != nil || k.Person != v.PersonID {
			return ErrForbidden
		}
		var p Person
		if tx.Get(store.Persons, []byte(v.PersonID), &p) != nil || (p.Status != "" && p.Status != "active") {
			return ErrForbidden
		}
		if tx.Has(store.Devices, []byte(v.DeviceID)) {
			return ErrConflict
		}
		now := s.now()
		var others []string
		tx.Keys(store.PersonDevices, append([]byte(v.PersonID), '/'), func(k []byte) bool {
			others = append(others, string(k[len(v.PersonID)+1:]))
			return true
		})
		for _, dev := range others {
			var d Device
			if tx.Get(store.Devices, []byte(dev), &d) == nil && d.Revoked == nil {
				d.Revoked = &now
				if err := tx.Put(store.Devices, []byte(dev), d); err != nil {
					return err
				}
				revoked++
			}
		}
		if err := endSessions(tx, func(x *Session) bool { return x.Person == v.PersonID }); err != nil {
			return err
		}
		if err := tx.Put(store.Devices, []byte(v.DeviceID), Device{Cert: cert, Added: now}); err != nil {
			return err
		}
		return tx.Put(store.PersonDevices, store.Key(v.PersonID, v.DeviceID), true)
	})
	if err != nil {
		return err
	}
	return s.AdminAudit("person.recover", "an account was recovered with its recovery kit on a new device; "+itoa(revoked)+" old device(s) revoked")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
