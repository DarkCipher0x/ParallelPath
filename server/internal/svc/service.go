package svc

import (
	"errors"
	"sync/atomic"
	"time"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
	"pp/server/internal/store"
)

// Errors returned to clients (the API maps them to HTTP status codes; messages are safe to show).
var (
	ErrUnauthorized = errors.New("not signed in")
	ErrForbidden    = errors.New("not allowed")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict - refresh and try again")
	ErrInvalid      = errors.New("invalid request")
	ErrExpired      = errors.New("expired")
	ErrRateLimited  = errors.New("too many requests - wait a moment")
)

// Event tells connected clients that something changed (they then fetch and verify it themselves).
type Event struct {
	SpaceID string `json:"space_id,omitempty"`
	Kind    string `json:"kind"` // item | audit | grant | member | space
	Seq     uint64 `json:"seq,omitempty"`
	// Signal is set for kind "signal": an end-to-end encrypted call-setup message (the server can't read it).
	Signal *Signal `json:"signal,omitempty"`
}

// Notifier delivers events to people's connected devices.
type Notifier interface {
	Notify(people []string, ev Event)
}

// Config holds tunable limits.
type Config struct {
	SessionTTL   time.Duration
	InviteTTL    time.Duration
	MaxBlobBytes int64
	SpaceQuota   int64 // total encrypted bytes per space
	ChallengeTTL time.Duration
	// The call relay: credentials are signed with TurnSecret; TurnPort is where it listens (0 = no relay).
	TurnSecret []byte
	TurnPort   int
}

// DefaultConfig is used unless the server config overrides it.
var DefaultConfig = Config{
	SessionTTL:   30 * 24 * time.Hour,
	InviteTTL:    7 * 24 * time.Hour,
	MaxBlobBytes: 2 << 30,  // 2 GiB per file (videos)
	SpaceQuota:   50 << 30, // 50 GiB per family space
	ChallengeTTL: 2 * time.Minute,
}

// Service is the server's rule engine.
type Service struct {
	db       *store.DB
	server   *pqc.SigningKey
	sessK    []byte
	notify   Notifier
	cfg      Config
	signals  signalLimit
	turnPort atomic.Int64
	links    links
	now      func() time.Time
	tlsPin   atomic.Pointer[func() []byte]
}

// SetTLSPin tells the service how to read the TLS pin the apps should hold (nil fingerprint = publicly trusted).
func (s *Service) SetTLSPin(f func() []byte) { s.tlsPin.Store(&f) }

// TLSPin is the server-signed statement of the current TLS pin (nil when there's nothing to pin).
func (s *Service) TLSPin() *keys.TLSPin {
	f := s.tlsPin.Load()
	if f == nil {
		return nil
	}
	pin := (*f)()
	if len(pin) == 0 {
		return nil
	}
	at := wire.Ms(s.now())
	sig, err := s.server.Sign(keys.PurposeTLSPin, keys.TLSPinMessage(pin, at))
	if err != nil {
		return nil
	}
	return &keys.TLSPin{Pin: pin, At: at, Signature: sig}
}

// New creates the service.
func New(db *store.DB, server *pqc.SigningKey, sessionKey []byte, n Notifier, cfg Config) *Service {
	return &Service{db: db, server: server, sessK: sessionKey, notify: n, cfg: cfg, now: func() time.Time { return time.Now().UTC() }}
}

// ServerKey is the public key that countersigns every audit entry (clients pin it).
func (s *Service) ServerKey() pqc.VerifyKey { return s.server.Public() }

// Caller is an authenticated device.
type Caller struct {
	Person string
	Device string
}

// deviceCert loads and verifies a registered, unrevoked device.
func deviceCert(tx *store.Tx, deviceID string) (*keys.VerifiedDevice, error) {
	var d Device
	if err := tx.Get(store.Devices, []byte(deviceID), &d); err != nil {
		return nil, ErrNotFound
	}
	if d.Revoked != nil {
		return nil, ErrForbidden
	}
	v, err := d.Cert.Verify()
	if err != nil {
		return nil, ErrInvalid
	}
	return v, nil
}

// member returns the caller's active membership of a space.
func (s *Service) member(tx *store.Tx, space, person string) (*Member, error) {
	if wire.CheckID(space) != nil {
		return nil, ErrInvalid
	}
	var m Member
	if err := tx.Get(store.Members, store.Key(space, person), &m); err != nil {
		return nil, ErrForbidden
	}
	if !m.Active(s.now()) {
		return nil, ErrForbidden
	}
	return &m, nil
}

// activeMembers lists the people currently in a space.
func (s *Service) activeMembers(tx *store.Tx, space string) ([]Member, error) {
	var out []Member
	now := s.now()
	err := store.Scan(tx, store.Members, append([]byte(space), '/'), nil, func(_ []byte, m *Member) bool {
		if m.Active(now) {
			out = append(out, *m)
		}
		return true
	})
	return out, err
}

// appendAudit verifies a device-signed intent and appends it to the chain (inside the caller's transaction).
func (s *Service) appendAudit(tx *store.Tx, c Caller, in *audit.Intent) (*audit.Entry, error) {
	if in.Person != c.Person || in.Device != c.Device {
		return nil, ErrForbidden
	}
	dev, err := deviceCert(tx, c.Device)
	if err != nil {
		return nil, err
	}
	if in.VerifyIntent(dev.Sign) != nil {
		return nil, ErrInvalid
	}
	// The client's clock may drift, but not wildly: it's shown next to the server's time in the audit view.
	if d := s.now().Sub(in.ClientTime); d > 24*time.Hour || d < -24*time.Hour {
		return nil, ErrInvalid
	}
	var head *audit.Entry
	var headSeq uint64
	if err := tx.Get(store.AuditHeads, headKey(in.SpaceID), &headSeq); err == nil {
		var h audit.Entry
		if err := tx.Get(store.Audit, store.KeySeq(headSeq, in.SpaceID), &h); err != nil {
			return nil, err
		}
		head = &h
	}
	e, err := audit.Append(s.server, head, *in, s.now())
	if err != nil {
		return nil, err
	}
	if err := tx.Put(store.Audit, store.KeySeq(e.Seq, in.SpaceID), e); err != nil {
		return nil, err
	}
	return e, tx.Put(store.AuditHeads, headKey(in.SpaceID), e.Seq)
}

// AdminAudit records an administrator action in the server chain, signed by the server itself (admins act on
// the machine, not through a device).
func (s *Service) AdminAudit(action, detail string) error {
	return s.db.Update(func(tx *store.Tx) error {
		in := audit.Intent{Action: audit.AdminAction, Person: adminID, Device: adminID, Detail: action + ": " + detail, ClientTime: s.now()}
		if len(in.Detail) > 200 {
			in.Detail = in.Detail[:200]
		}
		var head *audit.Entry
		var headSeq uint64
		if err := tx.Get(store.AuditHeads, headKey(""), &headSeq); err == nil {
			var h audit.Entry
			if err := tx.Get(store.Audit, store.KeySeq(headSeq, ""), &h); err != nil {
				return err
			}
			head = &h
		}
		signed, err := audit.SignIntent(s.server, in)
		if err != nil {
			return err
		}
		e, err := audit.Append(s.server, head, *signed, s.now())
		if err != nil {
			return err
		}
		if err := tx.Put(store.Audit, store.KeySeq(e.Seq, ""), e); err != nil {
			return err
		}
		return tx.Put(store.AuditHeads, headKey(""), e.Seq)
	})
}

// adminID stands for "the server operator" in the server chain (32 chars like a person ID; never a real one).
const adminID = audit.Operator

func (s *Service) emit(people []Member, ev Event) {
	if s.notify == nil {
		return
	}
	ids := make([]string, 0, len(people))
	for _, m := range people {
		ids = append(ids, m.Person)
	}
	s.notify.Notify(ids, ev)
}

// headKey is where a chain's head sequence is kept ("server" for the admin chain, which has no space ID).
func headKey(space string) []byte {
	if space == "" {
		return []byte("server")
	}
	return []byte(space)
}
