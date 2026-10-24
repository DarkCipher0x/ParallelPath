package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
	"pp/engine/remote"
	"pp/engine/vault"
)

// ServerPreview is shown before setup: what the person is about to trust.
type ServerPreview struct {
	URL            string `json:"url"`
	Name           string `json:"name"`
	Version        string `json:"version"`
	CertSHA256     []byte `json:"cert_sha256"`
	TrustedByCA    bool   `json:"trusted_by_ca"`
	KeyExchange    string `json:"key_exchange"`
	ServerKeyPrint string `json:"server_key_fingerprint"`
}

type hello struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	ServerKey []byte `json:"server_key"`
}

// Preview connects to a server once and reports its identity, so the person can confirm the fingerprint
// (for a self-hosted certificate) before anything is stored.
func (e *Engine) Preview(server string) (*ServerPreview, error) {
	p, err := remote.ProbeServer(server)
	if err != nil {
		return nil, err
	}
	var pin []byte
	if !p.TrustedByCA {
		pin = p.CertSHA256
	}
	c, err := remote.New(p.URL, pin)
	if err != nil {
		return nil, err
	}
	var h hello
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.Call(ctx, "GET", "/api/v1/hello", nil, &h); err != nil {
		return nil, err
	}
	sk, err := pqc.ParseVerifyKey(h.ServerKey)
	if err != nil {
		return nil, errors.New("this server didn't present a valid ParallelPath key")
	}
	return &ServerPreview{URL: p.URL, Name: h.Name, Version: h.Version, CertSHA256: p.CertSHA256, TrustedByCA: p.TrustedByCA,
		KeyExchange: p.KeyExchange, ServerKeyPrint: groups(wire.Fingerprint(sk.ID()))}, nil
}

// SetupRequest is everything first-time setup needs.
type SetupRequest struct {
	Server       string `json:"server"`
	CertSHA256   []byte `json:"cert_sha256"` // from Preview; required unless the certificate is CA-trusted
	Invite       string `json:"invite"`
	Passphrase   string `json:"passphrase"`
	DeviceSecret []byte `json:"device_secret,omitempty"` // Android: from the hardware keystore
	YourName     string `json:"your_name"`
	DeviceName   string `json:"device_name"`
	DeviceKind   string `json:"device_kind"`
	FamilyName   string `json:"family_name,omitempty"` // when starting a new family space (server invite)
}

// SetupResult tells the app where the person landed.
type SetupResult struct {
	Person  string `json:"person"`
	SpaceID string `json:"space_id"`
	Role    string `json:"role"`
	Waiting bool   `json:"waiting_for_approval"` // joined a space; a parent still needs to share keys
}

// Setup creates this device's identity, enrols with the invite and signs in.
func (e *Engine) Setup(req SetupRequest) (*SetupResult, error) {
	code, err := keys.ParseInvite(req.Invite)
	if err != nil {
		return nil, err
	}
	req.YourName = strings.TrimSpace(req.YourName)
	if req.YourName == "" || len([]rune(req.YourName)) > 60 {
		return nil, errors.New("enter your name (up to 60 characters)")
	}
	if code.Kind == 'S' && strings.TrimSpace(req.FamilyName) == "" {
		req.FamilyName = req.YourName + "'s family"
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if vault.Exists(e.vaultDir()) {
		return nil, errors.New("this device is already set up")
	}
	prev, err := e.previewLocked(req.Server, req.CertSHA256)
	if err != nil {
		return nil, err
	}
	srv := prev.client

	ident, err := keys.NewIdentity()
	if err != nil {
		return nil, err
	}
	dev, err := keys.NewDevice()
	if err != nil {
		return nil, err
	}
	cert, err := ident.Certify(dev, req.DeviceKind, strings.TrimSpace(req.DeviceName), time.Now())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	body := map[string]any{"invite": code.String(), "cert": cert}
	if code.Kind == 'J' {
		in, err := audit.SignIntent(dev.Sign, audit.Intent{SpaceID: code.Space, Action: audit.MemberJoin, Person: ident.PersonID(),
			Device: dev.DeviceID(), Detail: "role=" + code.Role, ClientTime: time.Now()})
		if err != nil {
			return nil, err
		}
		body["join_intent"] = in
	}
	var enrolled struct {
		PersonID string `json:"person_id"`
		SpaceID  string `json:"space_id"`
		Role     string `json:"role"`
	}
	if err := srv.Call(ctx, "POST", "/api/v1/enrol", body, &enrolled); err != nil {
		return nil, err
	}

	// Only now, with the account created, is anything written to this device.
	v, err := vault.Create(e.vaultDir(), []byte(req.Passphrase), req.DeviceSecret)
	if err != nil {
		return nil, err
	}
	kem, _ := dev.KEM.Bytes()
	if err := firstErr(
		v.Put(vault.Keys, "identity", ident.Key.Bytes()),
		v.Put(vault.Keys, "device", deviceKeys{Sign: dev.Sign.Bytes(), KEM: kem}),
		v.Put(vault.Keys, "cert", cert),
		v.Put(vault.Keys, "server", prev.info),
		v.Put(vault.State, "my-name", req.YourName),
	); err != nil {
		v.Lock()
		return nil, err
	}
	pqc.Wipe(kem)
	e.v, e.srv, e.ident, e.dev, e.person, e.server = v, srv, ident, dev, ident.PersonID(), prev.info
	if err := signIn(ctx, srv, dev, e.serverKey(), v); err != nil {
		return nil, err
	}
	res := &SetupResult{Person: e.person}
	if code.Kind == 'S' {
		space, err := e.createSpaceLocked(ctx, req.FamilyName, false)
		if err != nil {
			return nil, err
		}
		res.SpaceID, res.Role = space, "parent"
	} else {
		res.SpaceID, res.Role, res.Waiting = enrolled.SpaceID, enrolled.Role, true
		_ = v.Put(vault.State, "pending-profile/"+enrolled.SpaceID, req.YourName)
	}
	e.startLocked()
	return res, nil
}

type preview struct {
	client *remote.Client
	info   serverInfo
}

// previewLocked connects with the pin the person confirmed and checks the server's identity key.
func (e *Engine) previewLocked(server string, pin []byte) (*preview, error) {
	p, err := remote.ProbeServer(server)
	if err != nil {
		return nil, err
	}
	if !p.TrustedByCA {
		if len(pin) == 0 || !pqc.Equal(pin, p.CertSHA256) {
			return nil, errors.New("the server's certificate doesn't match the fingerprint you confirmed")
		}
	} else {
		pin = nil
	}
	c, err := remote.New(p.URL, pin)
	if err != nil {
		return nil, err
	}
	var h hello
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.Call(ctx, "GET", "/api/v1/hello", nil, &h); err != nil {
		return nil, err
	}
	if _, err := pqc.ParseVerifyKey(h.ServerKey); err != nil {
		return nil, errors.New("this server didn't present a valid ParallelPath key")
	}
	return &preview{client: c, info: serverInfo{URL: p.URL, Pin: pin, Key: h.ServerKey, Name: h.Name}}, nil
}

// JoinFamily accepts another space invite with this existing account.
func (e *Engine) JoinFamily(invite string) (*SetupResult, error) {
	code, err := keys.ParseInvite(invite)
	if err != nil || code.Kind != 'J' {
		return nil, keys.ErrBadInvite
	}
	e.mu.Lock()
	if e.v == nil {
		e.mu.Unlock()
		return nil, errLocked
	}
	in, err := audit.SignIntent(e.dev.Sign, audit.Intent{SpaceID: code.Space, Action: audit.MemberJoin, Person: e.person,
		Device: e.dev.DeviceID(), Detail: "role=" + code.Role, ClientTime: time.Now()})
	var name string
	_ = e.v.Get(vault.State, "my-name", &name)
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	var res SetupResult
	if err := e.call("POST", "/api/v1/join", map[string]any{"invite": code.String(), "intent": in}, &res); err != nil {
		return nil, err
	}
	res.Waiting = true
	e.mu.Lock()
	_ = e.v.Put(vault.State, "pending-profile/"+res.SpaceID, name)
	e.mu.Unlock()
	go e.syncSpace(res.SpaceID)
	return &res, nil
}

// groups formats a hex fingerprint as "abcd 1234 ..." for reading aloud.
func groups(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i += 4 {
		if i > 0 {
			b.WriteByte(' ')
		}
		end := min(i+4, len(s))
		b.WriteString(s[i:end])
	}
	return b.String()
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// jsonData builds a Body.Data value.
func jsonData(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
