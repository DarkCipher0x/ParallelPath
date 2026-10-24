// Package api is the HTTP interface of ppserver. It only translates: decode and size-check the request, find the
// signed-in device, call internal/svc, encode the answer. All rules live in svc.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"pp/server/internal/svc"
	"pp/server/internal/webapp"
)

// Version is set at build time.
var Version = "dev"

// Server wires the HTTP routes.
type Server struct {
	svc     *svc.Service
	hub     *Hub
	dataDir string
	name    string
	limit   *limiter
	log     *slog.Logger
}

// New builds the API handler.
func New(s *svc.Service, hub *Hub, dataDir, name string, log *slog.Logger) http.Handler {
	a := &Server{svc: s, hub: hub, dataDir: dataDir, name: name, limit: newLimiter(), log: log}
	m := http.NewServeMux()

	m.HandleFunc("GET /api/v1/hello", a.hello)
	m.HandleFunc("POST /api/v1/enrol", a.rate(a.enrol))
	m.HandleFunc("POST /api/v1/auth/challenge", a.rate(a.challenge))
	m.HandleFunc("POST /api/v1/auth/signin", a.rate(a.signIn))
	m.HandleFunc("POST /api/v1/auth/signout", a.auth(a.signOut))

	m.HandleFunc("POST /api/v1/devices", a.auth(a.addDevice))
	m.HandleFunc("GET /api/v1/devices/{device}", a.auth(a.device))
	m.HandleFunc("GET /api/v1/people/{person}/devices", a.auth(a.personDevices))

	m.HandleFunc("GET /api/v1/account", a.auth(a.account))
	m.HandleFunc("GET /api/v1/spaces", a.auth(a.spaces))
	m.HandleFunc("POST /api/v1/spaces", a.auth(a.createSpace))
	m.HandleFunc("GET /api/v1/spaces/{space}", a.auth(a.space))
	m.HandleFunc("POST /api/v1/spaces/{space}/invites", a.auth(a.invite))
	m.HandleFunc("POST /api/v1/spaces/{space}/settings", a.auth(a.settings))
	m.HandleFunc("POST /api/v1/spaces/{space}/conversations", a.auth(a.conversation))
	m.HandleFunc("POST /api/v1/spaces/{space}/members/{person}/permissions", a.auth(a.permissions))
	m.HandleFunc("POST /api/v1/join", a.auth(a.join))
	m.HandleFunc("POST /api/v1/spaces/{space}/members/{person}/remove", a.auth(a.removeMember))
	m.HandleFunc("GET /api/v1/spaces/{space}/keys", a.auth(a.myGrants))
	m.HandleFunc("POST /api/v1/spaces/{space}/keys", a.auth(a.shareKeys))

	m.HandleFunc("GET /api/v1/spaces/{space}/items", a.auth(a.items))
	m.HandleFunc("POST /api/v1/spaces/{space}/items", a.auth(a.postItem))
	m.HandleFunc("POST /api/v1/spaces/{space}/record", a.auth(a.record))
	m.HandleFunc("GET /api/v1/spaces/{space}/audit", a.auth(a.audit))

	m.HandleFunc("PUT /api/v1/spaces/{space}/blobs/{blob}", a.auth(a.putBlob))
	m.HandleFunc("GET /api/v1/blobs/{blob}", a.auth(a.getBlob))

	m.HandleFunc("GET /api/v1/events", a.auth(a.events))
	m.HandleFunc("POST /api/v1/spaces/{space}/signal", a.auth(a.signal))
	m.HandleFunc("GET /api/v1/turn", a.auth(a.turn))
	m.HandleFunc("PUT /api/v1/link/{id}", a.rate(a.linkRequest)) // linking a new device (see svc/link.go)
	m.HandleFunc("POST /api/v1/link/{id}", a.auth(a.linkOpen))
	m.HandleFunc("GET /api/v1/link/{id}", a.auth(a.linkPending))
	m.HandleFunc("POST /api/v1/link/{id}/answer", a.auth(a.linkAnswer))
	m.HandleFunc("GET /api/v1/link/{id}/answer", a.rate(a.linkResult))
	m.HandleFunc("PUT /api/v1/recovery", a.auth(a.recoverySave)) // recovery kits (see svc/recovery.go)
	m.HandleFunc("GET /api/v1/recovery/{id}", a.rate(a.recoveryFetch))
	m.HandleFunc("POST /api/v1/recovery/{id}/device", a.rate(a.recoveryRegister))

	m.HandleFunc("GET /{$}", a.home)
	m.HandleFunc("GET /style.css", a.style)
	m.Handle("GET /app/", webapp.Handler("/app/")) // the web app (same app as Windows and Android)
	m.Handle("GET /app", http.RedirectHandler("/app/", http.StatusMovedPermanently))
	return secure(m)
}

// --- helpers -----------------------------------------------------------------------------------------------

const maxJSON = 1 << 20 // 1 MiB: an item body is at most 256 KiB of ciphertext plus signatures and headers

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		fail(w, svc.ErrInvalid)
		return false
	}
	return true
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) { reply(w, http.StatusOK, v) }

func fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "server error"
	switch {
	case errors.Is(err, svc.ErrUnauthorized):
		status, msg = http.StatusUnauthorized, err.Error()
	case errors.Is(err, svc.ErrForbidden):
		status, msg = http.StatusForbidden, err.Error()
	case errors.Is(err, svc.ErrNotFound):
		status, msg = http.StatusNotFound, err.Error()
	case errors.Is(err, svc.ErrConflict):
		status, msg = http.StatusConflict, err.Error()
	case errors.Is(err, svc.ErrInvalid):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, svc.ErrExpired):
		status, msg = http.StatusGone, err.Error()
	case errors.Is(err, svc.ErrRateLimited):
		status, msg = http.StatusTooManyRequests, err.Error()
	}
	reply(w, status, map[string]string{"error": msg})
}

type ctxCaller struct{}

// auth requires a valid session token.
func (a *Server) auth(h func(http.ResponseWriter, *http.Request, *svc.Caller)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found {
			fail(w, svc.ErrUnauthorized)
			return
		}
		c, err := a.svc.Authenticate(tok)
		if err != nil {
			fail(w, err)
			return
		}
		h(w, r, c)
	}
}

// rate limits unauthenticated endpoints per client address.
func (a *Server) rate(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limit.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			reply(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts - wait a minute"})
			return
		}
		h(w, r)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// secure adds the response headers every reply carries.
func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Cross-Origin-Opener-Policy", "same-origin")
		hd.Set("Cross-Origin-Resource-Policy", "same-origin")
		hd.Set("Permissions-Policy", "camera=(self), microphone=(self), geolocation=(self)")
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; img-src 'self' blob: data:; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		hd.Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (a *Server) hello(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"product":    "ParallelPath",
		"name":       a.name,
		"version":    Version,
		"server_key": a.svc.ServerKey().Bytes(),
		"tls_pin":    a.svc.TLSPin(), // signed by server_key: lets the apps follow a certificate change safely
		"crypto":     "ML-KEM-1024+P-384 HPKE, ML-DSA-87+Ed25519, AES-256-GCM, SHA3-512; TLS 1.3 SecP384r1MLKEM1024/X25519MLKEM768",
		"time":       time.Now().UTC(),
	})
}

func (a *Server) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, homePage)
}

func (a *Server) style(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = io.WriteString(w, homeCSS)
}
