// Package app is the ParallelPath client engine: everything a client app does, behind one small API that the
// Windows app (through the ppengine process), the Android app (through gomobile) and the web client
// (through WebAssembly) all share. The UI never touches a key or a ciphertext.
package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"pp/core/keys"
	"pp/core/pqc"
	"pp/engine/remote"
	"pp/engine/vault"
)

// Listener receives events for the UI ("changed", "connection", "error"). Payload is JSON.
type Listener func(event string, payload any)

// Engine is one installation of the client on one device.
type Engine struct {
	mu       sync.Mutex
	dir      string
	v        *vault.Vault
	srv      *remote.Client
	ident    *keys.Identity
	dev      *keys.Device
	person   string
	server   serverInfo
	listen   Listener
	cancel   context.CancelFunc
	online   bool
	bodies   map[string]*decoded // decrypted bodies, memory only, cleared on lock
	syncing  map[string]bool
	calls    map[string]*callState // calls in progress (memory only)
	callSub  map[chan callEvent]struct{}
	bridge   *callBridge
	zone     *time.Location // the device's time zone, for reminders (SetTimeZone)
	remindMu sync.Mutex     // one reminder check at a time (each reminder fires once)
}

type serverInfo struct {
	URL  string `json:"url"`
	Pin  []byte `json:"pin,omitempty"`
	Key  []byte `json:"key"`
	Name string `json:"name"`
}

type deviceKeys struct {
	Sign []byte `json:"sign"`
	KEM  []byte `json:"kem"`
}

// New creates an engine keeping its data in dir (the app's private storage).
func New(dir string, l Listener) *Engine {
	if l == nil {
		l = func(string, any) {}
	}
	return &Engine{dir: dir, listen: l, bodies: map[string]*decoded{}, syncing: map[string]bool{}, calls: map[string]*callState{},
		callSub: map[chan callEvent]struct{}{}}
}

var (
	errLocked   = errors.New("ParallelPath is locked")
	errNotSetUp = errors.New("ParallelPath isn't set up on this device yet")
)

// Status is what the app shows before and after unlocking.
type Status struct {
	SetUp      bool   `json:"set_up"`
	Unlocked   bool   `json:"unlocked"`
	Person     string `json:"person,omitempty"`
	Device     string `json:"device,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	ServerURL  string `json:"server_url,omitempty"`
	Online     bool   `json:"online"`
	Crypto     string `json:"crypto"`
}

const cryptoSummary = "Post-quantum: ML-KEM-1024 + P-384, ML-DSA-87 + Ed25519, AES-256-GCM, SHA3-512, Argon2id; TLS 1.3 with ML-KEM"

// Status reports the engine state.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := Status{SetUp: vault.Exists(e.vaultDir()), Unlocked: e.v != nil, Online: e.online, Crypto: cryptoSummary}
	if e.v != nil {
		s.Person, s.ServerName, s.ServerURL = e.person, e.server.Name, e.server.URL
		s.Device = e.dev.DeviceID()
	}
	return s
}

func (e *Engine) vaultDir() string { return filepath.Join(e.dir, "vault") }
func (e *Engine) cacheDir() string { return filepath.Join(e.dir, "cache") }
func (e *Engine) tmpDir() string   { return filepath.Join(e.dir, "tmp") }

// Unlock opens the vault, restores keys and signs in to the server.
func (e *Engine) Unlock(passphrase, deviceSecret []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.v != nil {
		return nil
	}
	v, err := vault.Unlock(e.vaultDir(), passphrase, deviceSecret)
	if err != nil {
		return err
	}
	if err := e.load(v); err != nil {
		v.Lock()
		return err
	}
	e.startLocked()
	return nil
}

// load restores keys and server details from an unlocked vault.
func (e *Engine) load(v *vault.Vault) error {
	var seed []byte
	if err := v.Get(vault.Keys, "identity", &seed); err != nil {
		return err
	}
	ik, err := pqc.SigningKeyFromSeed(seed)
	pqc.Wipe(seed)
	if err != nil {
		return err
	}
	var dk deviceKeys
	if err := v.Get(vault.Keys, "device", &dk); err != nil {
		return err
	}
	sk, err := pqc.SigningKeyFromSeed(dk.Sign)
	if err != nil {
		return err
	}
	kk, err := pqc.KEMKeyFromBytes(dk.KEM)
	pqc.Wipe(dk.Sign)
	pqc.Wipe(dk.KEM)
	if err != nil {
		return err
	}
	if err := v.Get(vault.Keys, "server", &e.server); err != nil {
		return err
	}
	srv, err := remote.New(e.server.URL, e.server.Pin)
	if err != nil {
		return err
	}
	e.v, e.srv = v, srv
	e.ident, e.dev = &keys.Identity{Key: ik}, &keys.Device{Sign: sk, KEM: kk}
	e.person = e.ident.PersonID()
	var tok string
	if v.Get(vault.Keys, "session", &tok) == nil {
		srv.SetToken(tok)
	}
	return nil
}

// Lock wipes keys and decrypted content from memory, deletes decrypted files and closes the vault.
func (e *Engine) Lock() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lockLocked()
}

func (e *Engine) lockLocked() {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.bodies = map[string]*decoded{}
	if e.bridge != nil {
		e.bridge.close()
		e.bridge = nil
	}
	for _, c := range e.calls {
		pqc.Wipe(c.Key)
	}
	e.calls = map[string]*callState{}
	e.ident, e.dev, e.srv = nil, nil, nil
	if e.v != nil {
		e.v.Lock()
		e.v = nil
	}
	_ = os.RemoveAll(e.cacheDir())
	_ = os.RemoveAll(e.tmpDir())
	e.online = false
}

// startLocked begins the live connection: sign in if needed, sync everything, then listen for changes.
func (e *Engine) startLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	srv := e.srv
	go e.reminders(ctx.Done())
	go func() {
		if err := e.ensureSession(ctx); err != nil {
			if remote.IsPinChanged(err) {
				if ferr := e.followCertificate(); ferr == nil {
					return // followCertificate started a fresh connection
				} else {
					err = ferr
				}
			}
			e.listen("error", map[string]string{"message": err.Error()})
		}
		e.SyncAll()
		srv.Listen(ctx, func(ev remote.Event) {
			if ev.Kind == "signal" {
				if ev.Signal != nil {
					e.onSignal(ev.Signal)
				}
				return
			}
			if ev.SpaceID != "" {
				e.syncSpace(ev.SpaceID)
			} else {
				e.SyncAll()
			}
		}, func(up bool) {
			e.mu.Lock()
			if ctx.Err() != nil { // a connection being replaced (lock, new certificate) doesn't speak for the new one
				e.mu.Unlock()
				return
			}
			changed := e.online != up
			e.online = up
			e.mu.Unlock()
			if changed {
				e.listen("connection", map[string]bool{"online": up})
			}
			if up {
				e.SyncAll() // catch up on anything missed while offline
			}
		})
	}()
}

// ensureSession signs in with the device key if there's no valid session.
func (e *Engine) ensureSession(ctx context.Context) error {
	e.mu.Lock()
	srv, dev, v := e.srv, e.dev, e.v
	e.mu.Unlock()
	if srv == nil {
		return errLocked
	}
	var probe struct{}
	if err := srv.Call(ctx, "GET", "/api/v1/spaces", nil, &probe); err == nil {
		return nil
	} else if !remote.IsStatus(err, 401) {
		return err
	}
	return signIn(ctx, srv, dev, e.serverKey(), v)
}

func signIn(ctx context.Context, srv *remote.Client, dev *keys.Device, serverKey pqc.VerifyKey, v *vault.Vault) error {
	var ch struct {
		Nonce []byte `json:"nonce"`
	}
	if err := srv.Call(ctx, "POST", "/api/v1/auth/challenge", map[string]string{"device": dev.DeviceID()}, &ch); err != nil {
		return err
	}
	sig, err := dev.Sign.Sign(keys.PurposeAuth, keys.AuthMessage(serverKey, dev.DeviceID(), ch.Nonce))
	if err != nil {
		return err
	}
	var res struct {
		Token string `json:"token"`
	}
	if err := srv.Call(ctx, "POST", "/api/v1/auth/signin", map[string]any{"device": dev.DeviceID(), "signature": sig}, &res); err != nil {
		return err
	}
	srv.SetToken(res.Token)
	return v.Put(vault.Keys, "session", res.Token)
}

func (e *Engine) serverKey() pqc.VerifyKey {
	k, _ := pqc.ParseVerifyKey(e.server.Key)
	return k
}

// call runs a signed-in request, signing in again once if the session has expired.
func (e *Engine) call(method, path string, in, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	e.mu.Lock()
	srv := e.srv
	e.mu.Unlock()
	if srv == nil {
		return errLocked
	}
	err := srv.Call(ctx, method, path, in, out)
	if remote.IsPinChanged(err) && e.followCertificate() == nil {
		e.mu.Lock()
		srv = e.srv
		e.mu.Unlock()
		err = srv.Call(ctx, method, path, in, out)
	}
	if remote.IsStatus(err, 401) {
		if err := e.ensureSession(ctx); err != nil {
			return err
		}
		err = srv.Call(ctx, method, path, in, out)
	}
	return err
}
