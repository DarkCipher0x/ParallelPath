package svc

import (
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"pp/core/audit"
	"pp/core/pqc"
	"pp/server/internal/store"
)

// What the server operator can see and do. The server holds no names, messages or files it can read - families are
// end-to-end encrypted - so the operator works with metadata only: accounts by fingerprint, device names and kinds
// (from their certificates), counts, sizes and times. Every change is recorded in the server's audit chain.

// Overview is the server at a glance.
type Overview struct {
	People         int   `json:"people"`
	PeopleBlocked  int   `json:"people_blocked"`
	Devices        int   `json:"devices"`
	DevicesRevoked int   `json:"devices_revoked"`
	SignedIn       int   `json:"signed_in"` // live device sessions
	Spaces         int   `json:"spaces"`
	Items          int64 `json:"items"`
	Files          int   `json:"files"`
	FileBytes      int64 `json:"file_bytes"`
	Invites        int   `json:"invites"` // still usable
	AuditEntries   int64 `json:"audit_entries"`
}

// AdminDevice is one device, as the operator sees it.
type AdminDevice struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	Added      time.Time  `json:"added"`
	Revoked    *time.Time `json:"revoked,omitempty"`
	LastSignIn time.Time  `json:"last_sign_in,omitempty"`
}

// AdminPerson is one account, as the operator sees it (no name: names are encrypted inside the families).
type AdminPerson struct {
	ID      string        `json:"id"`
	Created time.Time     `json:"created"`
	Blocked bool          `json:"blocked"`
	Spaces  int           `json:"spaces"`
	Devices []AdminDevice `json:"devices"`
	// FamiliesLeft is how many more families they may start (only the operator can change it).
	FamiliesLeft int `json:"families_left"`
}

// AdminSpace is one family space, as the operator sees it.
type AdminSpace struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	Epoch     uint64    `json:"epoch"`
	Members   int       `json:"members"`
	Items     uint64    `json:"items"`
	Files     int       `json:"files"`
	FileBytes int64     `json:"file_bytes"`
}

// AdminInvite is an invite code that hasn't been used yet (the code itself is never stored, only its hash).
type AdminInvite struct {
	ID        string    `json:"id"` // the stored hash, which identifies it for revoking
	Kind      string    `json:"kind"`
	Space     string    `json:"space,omitempty"`
	Role      string    `json:"role,omitempty"`
	CreatedBy string    `json:"created_by"`
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
}

// AdminOverview counts what the server holds.
func (s *Service) AdminOverview() (*Overview, error) {
	o := &Overview{}
	now := s.now()
	err := s.db.View(func(tx *store.Tx) error {
		if err := store.Scan(tx, store.Persons, nil, nil, func(_ []byte, p *Person) bool {
			o.People++
			if p.Status != "active" {
				o.PeopleBlocked++
			}
			return true
		}); err != nil {
			return err
		}
		if err := store.Scan(tx, store.Devices, nil, nil, func(_ []byte, d *Device) bool {
			o.Devices++
			if d.Revoked != nil {
				o.DevicesRevoked++
			}
			return true
		}); err != nil {
			return err
		}
		if err := store.Scan(tx, store.Sessions, nil, nil, func(_ []byte, x *Session) bool {
			if now.Before(x.Expires) {
				o.SignedIn++
			}
			return true
		}); err != nil {
			return err
		}
		tx.Keys(store.Spaces, nil, func([]byte) bool { o.Spaces++; return true })
		tx.Keys(store.Items, nil, func([]byte) bool { o.Items++; return true })
		if err := store.Scan(tx, store.Blobs, nil, nil, func(_ []byte, b *Blob) bool {
			o.Files++
			o.FileBytes += b.Size
			return true
		}); err != nil {
			return err
		}
		if err := store.Scan(tx, store.Invites, nil, nil, func(_ []byte, i *Invite) bool {
			if now.Before(i.Expires) {
				o.Invites++
			}
			return true
		}); err != nil {
			return err
		}
		var head uint64
		if tx.Get(store.AuditHeads, headKey(""), &head) == nil {
			o.AuditEntries = int64(head)
		}
		return nil
	})
	return o, err
}

// AdminPeople lists every account with its devices.
func (s *Service) AdminPeople() ([]AdminPerson, error) {
	out := []AdminPerson{} // never null in JSON
	err := s.db.View(func(tx *store.Tx) error {
		lastSignIn := map[string]time.Time{}
		if err := store.Scan(tx, store.Sessions, nil, nil, func(_ []byte, x *Session) bool {
			if x.Created.After(lastSignIn[x.Device]) {
				lastSignIn[x.Device] = x.Created
			}
			return true
		}); err != nil {
			return err
		}
		return store.Scan(tx, store.Persons, nil, nil, func(_ []byte, p *Person) bool {
			ap := AdminPerson{ID: p.ID, Created: p.Created, Blocked: p.Status != "active", Devices: []AdminDevice{},
				FamiliesLeft: s.familiesLeft(tx, p)}
			tx.Keys(store.PersonSpaces, store.Key(p.ID, ""), func([]byte) bool { ap.Spaces++; return true })
			tx.Keys(store.PersonDevices, store.Key(p.ID, ""), func(k []byte) bool {
				id := string(k[len(p.ID)+1:])
				var d Device
				if tx.Get(store.Devices, []byte(id), &d) == nil {
					ap.Devices = append(ap.Devices, AdminDevice{ID: id, Kind: d.Cert.Kind, Name: d.Cert.Name, Added: d.Added,
						Revoked: d.Revoked, LastSignIn: lastSignIn[id]})
				}
				return true
			})
			out = append(out, ap)
			return true
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, err
}

// AdminSpaces lists the family spaces.
func (s *Service) AdminSpaces() ([]AdminSpace, error) {
	out := []AdminSpace{}
	err := s.db.View(func(tx *store.Tx) error {
		files := map[string][2]int64{}
		if err := store.Scan(tx, store.Blobs, nil, nil, func(_ []byte, b *Blob) bool {
			f := files[b.SpaceID]
			files[b.SpaceID] = [2]int64{f[0] + 1, f[1] + b.Size}
			return true
		}); err != nil {
			return err
		}
		now := s.now()
		return store.Scan(tx, store.Spaces, nil, nil, func(_ []byte, sp *Space) bool {
			as := AdminSpace{ID: sp.ID, Created: sp.Created, Epoch: sp.Epoch}
			_ = store.Scan(tx, store.Members, store.Key(sp.ID, ""), nil, func(_ []byte, m *Member) bool {
				if m.Active(now) {
					as.Members++
				}
				return true
			})
			tx.Keys(store.Items, store.Key(sp.ID, ""), func([]byte) bool { as.Items++; return true })
			f := files[sp.ID]
			as.Files, as.FileBytes = int(f[0]), f[1]
			out = append(out, as)
			return true
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, err
}

// AdminInvites lists the invite codes that can still be used.
func (s *Service) AdminInvites() ([]AdminInvite, error) {
	out := []AdminInvite{}
	now := s.now()
	err := s.db.View(func(tx *store.Tx) error {
		return store.Scan(tx, store.Invites, nil, nil, func(k []byte, i *Invite) bool {
			if now.After(i.Expires) {
				return true
			}
			ai := AdminInvite{ID: string(k), Kind: "server", Space: i.SpaceID, Role: i.Role, CreatedBy: i.CreatedBy, Created: i.Created,
				Expires: i.Expires}
			if i.SpaceID != "" {
				ai.Kind = "family"
			}
			out = append(out, ai)
			return true
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, err
}

// AdminRevokeInvite cancels an unused invite.
func (s *Service) AdminRevokeInvite(id, by string) error {
	if _, err := hex.DecodeString(id); err != nil || len(id) != 64 {
		return ErrInvalid
	}
	err := s.db.Update(func(tx *store.Tx) error {
		if !tx.Has(store.Invites, []byte(id)) {
			return ErrNotFound
		}
		return tx.Delete(store.Invites, []byte(id))
	})
	if err != nil {
		return err
	}
	return s.AdminAudit("invite.revoke", by+" revoked invite "+id[:12])
}

// AdminSetBlocked blocks (or unblocks) an account. A blocked account can't sign in and its devices are signed out
// at once; its family data is untouched.
func (s *Service) AdminSetBlocked(person string, blocked bool, by string) error {
	err := s.db.Update(func(tx *store.Tx) error {
		var p Person
		if err := tx.Get(store.Persons, []byte(person), &p); err != nil {
			return ErrNotFound
		}
		p.Status = "active"
		if blocked {
			p.Status = "disabled"
		}
		if err := tx.Put(store.Persons, []byte(person), p); err != nil {
			return err
		}
		if blocked {
			return endSessions(tx, func(x *Session) bool { return x.Person == person })
		}
		return nil
	})
	if err != nil {
		return err
	}
	verb := "unblocked"
	if blocked {
		verb = "blocked"
	}
	return s.AdminAudit("person."+verb, by+" "+verb+" account "+short(person))
}

// AdminSignOutDevice ends a device's sessions (it must sign in again with its keys).
func (s *Service) AdminSignOutDevice(device, by string) error {
	err := s.db.Update(func(tx *store.Tx) error {
		if !tx.Has(store.Devices, []byte(device)) {
			return ErrNotFound
		}
		return endSessions(tx, func(x *Session) bool { return x.Device == device })
	})
	if err != nil {
		return err
	}
	return s.AdminAudit("device.signout", by+" signed out device "+short(device))
}

// AdminRevokeDevice permanently stops a device (lost or stolen): it can never sign in again. The family should then
// rotate their keys so the device can't read anything new, which the apps prompt parents to do.
func (s *Service) AdminRevokeDevice(device, by string) error {
	err := s.db.Update(func(tx *store.Tx) error {
		var d Device
		if err := tx.Get(store.Devices, []byte(device), &d); err != nil {
			return ErrNotFound
		}
		if d.Revoked != nil {
			return errors.New("already revoked")
		}
		now := s.now()
		d.Revoked = &now
		if err := tx.Put(store.Devices, []byte(device), d); err != nil {
			return err
		}
		return endSessions(tx, func(x *Session) bool { return x.Device == device })
	})
	if err != nil {
		return err
	}
	return s.AdminAudit("device.revoke", by+" revoked device "+short(device))
}

func endSessions(tx *store.Tx, match func(*Session) bool) error {
	var doomed [][]byte
	if err := store.Scan(tx, store.Sessions, nil, nil, func(k []byte, x *Session) bool {
		if match(x) {
			doomed = append(doomed, k)
		}
		return true
	}); err != nil {
		return err
	}
	for _, k := range doomed {
		if err := tx.Delete(store.Sessions, k); err != nil {
			return err
		}
	}
	return nil
}

// ServerAuditEntry is one entry of the server's own audit chain.
type ServerAuditEntry struct {
	Seq    uint64    `json:"seq"`
	Time   time.Time `json:"time"`
	Detail string    `json:"detail"`
}

// ServerAudit returns the newest entries of the server chain (newest first) and, if the chain doesn't verify, why.
func (s *Service) ServerAudit(limit int) ([]ServerAuditEntry, string, error) {
	var all []audit.Entry
	err := s.db.View(func(tx *store.Tx) error {
		return store.Scan(tx, store.Audit, store.Key("", ""), nil, func(_ []byte, e *audit.Entry) bool {
			all = append(all, *e)
			return true
		})
	})
	if err != nil {
		return nil, "", err
	}
	problem := verifyServerChain(all, s)
	out := []ServerAuditEntry{}
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, ServerAuditEntry{Seq: all[i].Seq, Time: all[i].Time, Detail: all[i].Detail})
	}
	return out, problem, nil
}

func short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// verifyServerChain checks the server chain: links, order, times and signatures (operator entries are signed by
// the server's own key).
func verifyServerChain(entries []audit.Entry, s *Service) string {
	server := s.server.Public()
	// Operator entries are checked against the server key by audit.Verify; people's entries (enrolling, sign-ins)
	// against their devices - including devices revoked since, whose certificates are kept for exactly this.
	keyOf := func(device string) (pqc.VerifyKey, bool) {
		var d Device
		if s.db.View(func(tx *store.Tx) error { return tx.Get(store.Devices, []byte(device), &d) }) != nil {
			return pqc.VerifyKey{}, false
		}
		v, err := d.Cert.Verify()
		if err != nil {
			return pqc.VerifyKey{}, false
		}
		return v.Sign, true
	}
	_, err := audit.Verify("", entries, server, keyOf, nil)
	if err != nil {
		return err.Error()
	}
	return ""
}
