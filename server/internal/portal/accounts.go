package portal

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"

	"pp/core/pqc"
	"pp/server/internal/store"
	"pp/server/internal/svc"
)

// Account is a server operator. The passphrase is stored as an Argon2id hash and the TOTP secret is sealed with
// the rest of the database.
type Account struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Pass        string    `json:"pass"`
	TOTP        []byte    `json:"totp"`
	Created     time.Time `json:"created"`
	CreatedBy   string    `json:"created_by"`
	LastLogin   time.Time `json:"last_login"`
	Failed      int       `json:"failed"`
	LockedUntil time.Time `json:"locked_until"`
	Disabled    bool      `json:"disabled"`
	LastStep    int64     `json:"last_step"` // last TOTP step used (no replays)
}

// Token is a one-time link: "setup" creates an account (or resets one), "local" signs in from this machine.
type Token struct {
	Kind      string    `json:"kind"`
	Account   string    `json:"account,omitempty"` // setup: the account being reset ("" = a new one)
	TOTP      []byte    `json:"totp,omitempty"`    // setup: the secret shown once setup begins
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
	CreatedBy string    `json:"created_by"`
}

// Session is a signed-in operator. Only a keyed hash of the cookie is stored.
type Session struct {
	ID      string    `json:"id"` // a short public ID, to list and end sessions
	Account string    `json:"account"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Seen    time.Time `json:"seen"`
	Expires time.Time `json:"expires"`
	IP      string    `json:"ip"`
	Agent   string    `json:"agent"`
	CSRF    string    `json:"csrf"`
}

// Lifetimes.
const (
	SetupTTL      = 24 * time.Hour
	LocalTTL      = 2 * time.Minute
	SessionIdle   = 30 * time.Minute
	SessionMax    = 12 * time.Hour
	MaxFailures   = 5
	LockoutPeriod = 15 * time.Minute
	MinPassphrase = 12
	localAccount  = "local"
)

var (
	errBadLogin = errors.New("that name, passphrase or code isn't right")
	errLocked   = errors.New("too many failed attempts: this account is locked for 15 minutes")
	errBadLink  = errors.New("this link has expired or was already used - make a new one")
)

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Store is the portal's accounts, links and sessions, in the server database.
type Store struct {
	db  *store.DB
	svc *svc.Service
	now func() time.Time
}

// NewToken makes a one-time link token (the caller turns it into a URL).
func (s *Store) NewToken(kind, account, by string) (string, error) {
	ttl := SetupTTL
	if kind == "local" {
		ttl = LocalTTL
	}
	tok := svc.RandToken()
	t := Token{Kind: kind, Account: account, Created: s.now(), Expires: s.now().Add(ttl), CreatedBy: by}
	return tok, s.db.Update(func(tx *store.Tx) error { return tx.Put(store.AdminTokens, s.svc.TokenKey("admin-"+kind, tok), t) })
}

func (s *Store) token(kind, tok string) (*Token, []byte, error) {
	if len(tok) < 20 || len(tok) > 100 {
		return nil, nil, errBadLink
	}
	k := s.svc.TokenKey("admin-"+kind, tok)
	var t Token
	err := s.db.View(func(tx *store.Tx) error { return tx.Get(store.AdminTokens, k, &t) })
	if err != nil || s.now().After(t.Expires) {
		return nil, nil, errBadLink
	}
	return &t, k, nil
}

// BeginSetup shows the TOTP secret for a setup link (the same one each time the link is opened).
func (s *Store) BeginSetup(tok string) (*Token, error) {
	t, k, err := s.token("setup", tok)
	if err != nil {
		return nil, err
	}
	if t.TOTP == nil {
		t.TOTP = newTOTPSecret()
		if err := s.db.Update(func(tx *store.Tx) error { return tx.Put(store.AdminTokens, k, t) }); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func checkName(n string) error {
	if len(n) < 2 || len(n) > 40 {
		return errors.New("the name must be 2-40 characters")
	}
	for _, r := range n {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_') {
			return errors.New("the name can use letters, digits, dots, dashes and underscores")
		}
	}
	return nil
}

func checkPassphrase(p string) error {
	if len([]rune(p)) < MinPassphrase {
		return errors.New("the passphrase must be at least 12 characters - a few random words is best")
	}
	if len(p) > 1024 {
		return errors.New("the passphrase is too long")
	}
	return nil
}

// FinishSetup creates (or resets) the account once the person has set a passphrase and proved their authenticator
// works. The link is used up.
func (s *Store) FinishSetup(tok, name, pass, code string) (*Account, error) {
	t, k, err := s.token("setup", tok)
	if err != nil {
		return nil, err
	}
	if t.TOTP == nil {
		return nil, errBadLink
	}
	if err := checkPassphrase(pass); err != nil {
		return nil, err
	}
	step, ok := checkTOTP(t.TOTP, code, s.now(), 0)
	if !ok {
		return nil, errors.New("that code isn't right - check the time on your phone and try the newest code")
	}
	hash := pqc.HashPassword([]byte(pass))
	var acc Account
	err = s.db.Update(func(tx *store.Tx) error {
		if t.Account != "" {
			if err := tx.Get(store.AdminAccounts, []byte(t.Account), &acc); err != nil {
				return errBadLink
			}
		} else {
			name = strings.TrimSpace(name)
			if err := checkName(name); err != nil {
				return err
			}
			if taken, _ := s.byName(tx, name); taken != nil {
				return errors.New("that name is taken")
			}
			acc = Account{ID: newID(), Name: name, Created: s.now(), CreatedBy: t.CreatedBy}
		}
		acc.Pass, acc.TOTP, acc.LastStep, acc.Failed, acc.LockedUntil, acc.Disabled = hash, t.TOTP, step, 0, time.Time{}, false
		if err := tx.Put(store.AdminAccounts, []byte(acc.ID), acc); err != nil {
			return err
		}
		return tx.Delete(store.AdminTokens, k)
	})
	if err != nil {
		return nil, err
	}
	verb := "created"
	if t.Account != "" {
		verb = "reset"
	}
	return &acc, s.svc.AdminAudit("admin."+verb, "admin account "+acc.Name+" "+verb)
}

func (s *Store) byName(tx *store.Tx, name string) (*Account, []byte) {
	var found *Account
	var key []byte
	_ = store.Scan(tx, store.AdminAccounts, nil, nil, func(k []byte, a *Account) bool {
		if strings.EqualFold(a.Name, name) {
			found, key = a, k
			return false
		}
		return true
	})
	return found, key
}

// Login checks name, passphrase and code. Failures count towards a lockout; the reply never says which part was wrong.
func (s *Store) Login(name, pass, code string) (*Account, error) {
	var acc *Account
	var key []byte
	var result error
	err := s.db.Update(func(tx *store.Tx) error {
		acc, key = s.byName(tx, strings.TrimSpace(name))
		if acc == nil {
			pqc.CheckPassword([]byte(pass), dummyHash) // the same work as a real check, so timing reveals nothing
			result = errBadLogin
			return nil
		}
		now := s.now()
		if now.Before(acc.LockedUntil) {
			result = errLocked
			return nil
		}
		okPass, _ := pqc.CheckPassword([]byte(pass), acc.Pass)
		step, okCode := checkTOTP(acc.TOTP, code, now, acc.LastStep)
		if !okPass || !okCode || acc.Disabled {
			acc.Failed++
			if acc.Failed >= MaxFailures {
				acc.LockedUntil, acc.Failed = now.Add(LockoutPeriod), 0
			}
			result = errBadLogin
			return tx.Put(store.AdminAccounts, key, acc)
		}
		acc.Failed, acc.LastStep, acc.LastLogin = 0, step, now
		if pqc.NeedsRehash(acc.Pass) {
			acc.Pass = pqc.HashPassword([]byte(pass))
		}
		return tx.Put(store.AdminAccounts, key, acc)
	})
	if err != nil {
		return nil, err
	}
	if result != nil {
		_ = s.svc.AdminAudit("admin.login.failed", "failed sign-in as "+clip(name, 40))
		return nil, result
	}
	return acc, s.svc.AdminAudit("admin.login", acc.Name+" signed in")
}

var dummyHash = pqc.HashPassword([]byte("not a real passphrase"))

// LocalLogin uses a one-time link made on this machine by an administrator (Windows' UAC, or root on Linux).
func (s *Store) LocalLogin(tok string) error {
	_, k, err := s.token("local", tok)
	if err != nil {
		return err
	}
	if err := s.db.Update(func(tx *store.Tx) error { return tx.Delete(store.AdminTokens, k) }); err != nil {
		return err
	}
	return s.svc.AdminAudit("admin.login", "the computer's administrator signed in on this machine")
}

// NewSession signs someone in and returns the cookie value.
func (s *Store) NewSession(account, name, ip, agent string) (string, *Session, error) {
	tok := svc.RandToken()
	now := s.now()
	sess := &Session{ID: newID()[:12], Account: account, Name: name, Created: now, Seen: now, Expires: now.Add(SessionMax), IP: ip,
		Agent: clip(agent, 160), CSRF: svc.RandToken()}
	return tok, sess, s.db.Update(func(tx *store.Tx) error {
		return tx.Put(store.AdminSessions, s.svc.TokenKey("admin-session", tok), sess)
	})
}

// Session resolves a cookie, enforcing the idle and absolute limits and that the account is still allowed.
func (s *Store) Session(tok string) (*Session, error) {
	if len(tok) < 20 || len(tok) > 100 {
		return nil, errors.New("signed out")
	}
	k := s.svc.TokenKey("admin-session", tok)
	var sess Session
	err := s.db.Update(func(tx *store.Tx) error {
		if err := tx.Get(store.AdminSessions, k, &sess); err != nil {
			return err
		}
		now := s.now()
		if now.After(sess.Expires) || now.Sub(sess.Seen) > SessionIdle {
			_ = tx.Delete(store.AdminSessions, k)
			return errors.New("expired")
		}
		if sess.Account != localAccount {
			var a Account
			if tx.Get(store.AdminAccounts, []byte(sess.Account), &a) != nil || a.Disabled {
				_ = tx.Delete(store.AdminSessions, k)
				return errors.New("disabled")
			}
		}
		sess.Seen = now
		return tx.Put(store.AdminSessions, k, sess)
	})
	if err != nil {
		return nil, errors.New("signed out")
	}
	return &sess, nil
}

// EndSession signs out by cookie.
func (s *Store) EndSession(tok string) error {
	return s.db.Update(func(tx *store.Tx) error { return tx.Delete(store.AdminSessions, s.svc.TokenKey("admin-session", tok)) })
}

// Sessions lists live sessions.
func (s *Store) Sessions() ([]Session, error) {
	out := []Session{} // never null in JSON
	now := s.now()
	err := s.db.View(func(tx *store.Tx) error {
		return store.Scan(tx, store.AdminSessions, nil, nil, func(_ []byte, x *Session) bool {
			if now.Before(x.Expires) && now.Sub(x.Seen) <= SessionIdle {
				x.CSRF = ""
				out = append(out, *x)
			}
			return true
		})
	})
	return out, err
}

// EndSessionByID signs someone out from the sessions list.
func (s *Store) EndSessionByID(id string) error {
	return s.db.Update(func(tx *store.Tx) error {
		var doomed []byte
		_ = store.Scan(tx, store.AdminSessions, nil, nil, func(k []byte, x *Session) bool {
			if x.ID == id {
				doomed = k
				return false
			}
			return true
		})
		if doomed == nil {
			return svc.ErrNotFound
		}
		return tx.Delete(store.AdminSessions, doomed)
	})
}

// Accounts lists operators (without secrets).
func (s *Store) Accounts() ([]Account, error) {
	out := []Account{}
	err := s.db.View(func(tx *store.Tx) error {
		return store.Scan(tx, store.AdminAccounts, nil, nil, func(_ []byte, a *Account) bool {
			a.Pass, a.TOTP = "", nil
			out = append(out, *a)
			return true
		})
	})
	return out, err
}

// HasAccounts reports whether any operator account exists yet.
func (s *Store) HasAccounts() bool {
	found := false
	_ = s.db.View(func(tx *store.Tx) error {
		tx.Keys(store.AdminAccounts, nil, func([]byte) bool { found = true; return false })
		return nil
	})
	return found
}

// SetDisabled turns an operator account off (signing them out) or back on.
func (s *Store) SetDisabled(id string, disabled bool, by string) error {
	var name string
	err := s.db.Update(func(tx *store.Tx) error {
		var a Account
		if err := tx.Get(store.AdminAccounts, []byte(id), &a); err != nil {
			return svc.ErrNotFound
		}
		a.Disabled, name = disabled, a.Name
		return tx.Put(store.AdminAccounts, []byte(id), a)
	})
	if err != nil {
		return err
	}
	verb := "enabled"
	if disabled {
		verb = "disabled"
	}
	return s.svc.AdminAudit("admin."+verb, by+" "+verb+" admin account "+name)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
