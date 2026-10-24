// Package portal is the server's admin web portal at /admin/: status, invites, accounts and devices, families,
// backups, settings, logs, the server audit log and the operators themselves. The same portal serves Windows and
// Linux; ParallelPath Server Manager shows it inside its window on Windows.
//
// Security: it is reached over the server's post-quantum TLS like everything else. Operators sign in with a
// passphrase (Argon2id) and a TOTP code; five failures lock the account for 15 minutes and each address may only
// try so often. The session cookie is __Host-, Secure, HttpOnly and SameSite=Strict, every change also needs the
// session's CSRF token, and sessions end after 30 minutes idle or 12 hours. The portal can be limited to given
// networks. The first account is made with a one-time link created on the server itself (`ppserver admin
// setup-link`, or the Manager on Windows), and every operator action goes into the server's signed audit chain.
// No part of it can read family content: that is end-to-end encrypted.
package portal

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"pp/server/internal/config"
	"pp/server/internal/logring"
	"pp/server/internal/store"
	"pp/server/internal/svc"
)

//go:embed web/*
var web embed.FS

// Status is what the portal shows about the running server itself.
type Status struct {
	Version    string    `json:"version"`
	Started    time.Time `json:"started"`
	AtRest     string    `json:"at_rest"`
	ServerKey  string    `json:"server_key"`
	CertSHA256 string    `json:"tls_cert_sha256"`
	OS         string    `json:"os"`
	TLSMode    string    `json:"tls_mode"`            // local-ca | acme | files | self
	Uncovered  []string  `json:"uncovered,omitempty"` // names the local CA can't vouch for (added after it was made)
}

// Options wires the portal to the server.
type Options struct {
	DB      *store.DB
	Svc     *svc.Service
	DataDir string
	Status  Status
	Logs    *logring.Ring
	Restart func() // ends the process so the service manager starts it again
	Log     *slog.Logger
	Now     func() time.Time
}

// Portal serves /admin/.
type Portal struct {
	o     Options
	st    *Store
	mu    sync.Mutex
	cfg   *config.Config
	tries map[netip.Addr][]time.Time // failed sign-ins per address
}

// New makes the portal.
func New(o Options) (*Portal, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	c, err := config.Load(o.DataDir)
	if err != nil {
		return nil, err
	}
	return &Portal{o: o, st: &Store{db: o.DB, svc: o.Svc, now: o.Now}, cfg: c, tries: map[netip.Addr][]time.Time{}}, nil
}

// Accounts gives the account store (for the server's local admin endpoint).
func (p *Portal) Accounts() *Store { return p.st }

func (p *Portal) config() config.Config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return *p.cfg
}

// Handler serves everything under /admin/.
func (p *Portal) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /admin/{$}", p.file("index.html", "text/html; charset=utf-8"))
	m.HandleFunc("GET /admin/app.css", p.file("app.css", "text/css; charset=utf-8"))
	m.HandleFunc("GET /admin/app.js", p.file("app.js", "text/javascript; charset=utf-8"))
	m.HandleFunc("GET /admin/logo.svg", p.file("logo.svg", "image/svg+xml"))
	p.routes(m)
	return p.guard(m)
}

func (p *Portal) file(name, ctype string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := web.ReadFile("web/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		_, _ = w.Write(b)
	}
}

// guard applies the network allow-list and the portal's security headers to every request.
func (p *Portal) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.allowed(clientAddr(r)) {
			http.NotFound(w, r) // to anyone outside the allowed networks the portal doesn't exist
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; "+
			"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func clientAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

// allowed: this machine always; others when no networks are listed or they're in one.
func (p *Portal) allowed(a netip.Addr) bool {
	if a.IsLoopback() {
		return true
	}
	allow := p.config().AdminAllow
	if len(allow) == 0 {
		return a.IsValid()
	}
	for _, s := range allow {
		if pf, err := netip.ParsePrefix(s); err == nil && pf.Contains(a) {
			return true
		}
	}
	return false
}

// tooManyTries limits failed sign-ins per address: 10 in 15 minutes.
func (p *Portal) tooManyTries(a netip.Addr) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	cut := p.o.Now().Add(-15 * time.Minute)
	recent := p.tries[a][:0]
	for _, t := range p.tries[a] {
		if t.After(cut) {
			recent = append(recent, t)
		}
	}
	p.tries[a] = recent
	return len(recent) >= 10
}

func (p *Portal) failedTry(a netip.Addr) {
	p.mu.Lock()
	p.tries[a] = append(p.tries[a], p.o.Now())
	p.mu.Unlock()
}

const cookieName = "__Host-pp-admin"

func (p *Portal) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/", Secure: true, HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}

// signIn starts a session and sets its cookie.
func (p *Portal) signIn(w http.ResponseWriter, r *http.Request, account, name string) error {
	tok, _, err := p.st.NewSession(account, name, clientAddr(r).String(), r.UserAgent())
	if err != nil {
		return err
	}
	p.setCookie(w, tok, int(SessionMax/time.Second))
	return nil
}

// authed runs h for a signed-in operator; changes (anything but GET) also need the session's CSRF token.
func (p *Portal) authed(h func(http.ResponseWriter, *http.Request, *Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			fail(w, http.StatusUnauthorized, errors.New("signed out"))
			return
		}
		sess, err := p.st.Session(c.Value)
		if err != nil {
			p.setCookie(w, "", -1)
			fail(w, http.StatusUnauthorized, err)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-PP-CSRF") != sess.CSRF {
			fail(w, http.StatusForbidden, errors.New("this page is out of date - reload it"))
			return
		}
		h(w, r, sess)
	}
}

func send(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// read decodes a small JSON body strictly.
func read(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid request")
	}
	return nil
}

// errStatus maps service errors to HTTP codes.
func errStatus(err error) int {
	switch {
	case errors.Is(err, svc.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, svc.ErrInvalid):
		return http.StatusBadRequest
	}
	return http.StatusBadRequest
}

// isLocal reports a request from this machine (local sign-in links only work there).
func isLocal(r *http.Request) bool { return clientAddr(r).IsLoopback() }

// first host for links people open from elsewhere.
func (p *Portal) publicBase() string {
	c := p.config()
	host := "127.0.0.1"
	if len(c.Hosts) > 0 {
		host = c.Hosts[0]
	}
	_, port, _ := net.SplitHostPort(c.Listen)
	return "https://" + net.JoinHostPort(host, port)
}

// SetupURL is the link that creates (or resets) an operator account.
func (p *Portal) SetupURL(tok string) string { return p.publicBase() + "/admin/#setup=" + tok }

// LocalURL is the link the Manager (or `ppserver admin login-link`) opens on this machine.
func (p *Portal) LocalURL(tok string) string {
	_, port, _ := net.SplitHostPort(p.config().Listen)
	return "https://" + net.JoinHostPort("127.0.0.1", port) + "/admin/#local=" + tok
}
