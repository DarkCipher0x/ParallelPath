package app

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/engine/remote"
	"pp/engine/vault"
)

// Linking another device to this person (a phone and a PC, say), so it reads and writes as them in every family -
// and holds their private journal.
//
//  1. This device makes a one-time link code (160 random bits; it's typed or scanned, never sent to the server).
//  2. The new device posts its public keys to a slot named by a hash of the code, sealed with a key from the code.
//  3. This device shows what's asking ("Pixel 9, Android"); when the person confirms, it certifies the new device
//     with the identity key, registers it, shares every family key it holds with it, and answers with the identity
//     key - wrapped to the new device's ML-KEM-1024 key and also sealed with the code.
//  4. The new device checks the certificate is for itself, stores everything in its own vault and signs in.

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func linkCode(secret []byte) string {
	s := b32.EncodeToString(secret)
	var parts []string
	for i := 0; i < len(s); i += 4 {
		parts = append(parts, s[i:min(i+4, len(s))])
	}
	return "L-" + strings.Join(parts, "-")
}

func parseLinkCode(code string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(code))
	s = strings.TrimPrefix(s, "L-") // the prefix with its dash: "L" alone is also a base32 letter
	s = strings.NewReplacer("-", "", " ", "").Replace(s)
	b, err := b32.DecodeString(s)
	if err != nil || len(b) != 20 {
		return nil, errors.New("that isn't a link code - it starts with L- and comes from your other device")
	}
	return b, nil
}

func linkKeys(secret []byte) (string, []byte) {
	h := pqc.Hash([]byte("pp/link-id/v1"), secret)
	return hex.EncodeToString(h[:16]), pqc.Derive(secret, nil, "link", pqc.KeySize)
}

type linkRequest struct {
	SignKey []byte `json:"sign_key"`
	KEMKey  []byte `json:"kem_key"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
}

type linkAnswer struct {
	Wrapped []byte `json:"wrapped"` // a fresh key, wrapped to the new device's KEM key
	Box     []byte `json:"box"`     // the bundle, sealed with that key and the code's key
}

type linkBundle struct {
	Identity []byte          `json:"identity"`
	Cert     keys.DeviceCert `json:"cert"`
	Name     string          `json:"name"`
}

// LinkStart makes a one-time code for linking another device (valid ten minutes).
func (e *Engine) LinkStart() (map[string]string, error) {
	e.mu.Lock()
	ok := e.ident != nil
	e.mu.Unlock()
	if !ok {
		return nil, errLocked
	}
	secret := make([]byte, 20)
	_, _ = rand.Read(secret)
	id, _ := linkKeys(secret)
	if err := e.call("POST", "/api/v1/link/"+id, map[string]any{}, nil); err != nil { // only this person may answer it
		return nil, err
	}
	return map[string]string{"code": linkCode(secret)}, nil
}

// LinkCheck says whether a device has asked to link with the code yet, and what it is.
func (e *Engine) LinkCheck(code string) (map[string]any, error) {
	req, _, err := e.linkFetch(code)
	if remote.IsStatus(err, 404) {
		return map[string]any{"waiting": true}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"waiting": false, "name": req.Name, "kind": req.Kind}, nil
}

func (e *Engine) linkFetch(code string) (*linkRequest, []byte, error) {
	secret, err := parseLinkCode(code)
	if err != nil {
		return nil, nil, err
	}
	id, k := linkKeys(secret)
	var res struct {
		Payload []byte `json:"payload"`
	}
	if err := e.call("GET", "/api/v1/link/"+id, nil, &res); err != nil {
		return nil, nil, err
	}
	plain, err := pqc.Open(k, []byte("pp/link/request|"+id), res.Payload)
	if err != nil {
		return nil, nil, errors.New("the request didn't come from a device with this code")
	}
	var req linkRequest
	if json.Unmarshal(plain, &req) != nil {
		return nil, nil, errors.New("unreadable link request")
	}
	return &req, k, nil
}

// LinkConfirm links the device that asked: certifies and registers it, shares every family key with it, and sends
// it this person's identity. Only call it once the person has confirmed the device is theirs.
func (e *Engine) LinkConfirm(code string) (map[string]any, error) {
	req, k, err := e.linkFetch(code)
	if err != nil {
		return nil, err
	}
	defer pqc.Wipe(k)
	secret, _ := parseLinkCode(code)
	id, _ := linkKeys(secret)
	sk, err := pqc.ParseVerifyKey(req.SignKey)
	if err != nil {
		return nil, errors.New("the new device's keys are invalid")
	}
	kk, err := pqc.ParseKEMPublicKey(req.KEMKey)
	if err != nil {
		return nil, errors.New("the new device's keys are invalid")
	}
	switch req.Kind {
	case keys.KindWindows, keys.KindAndroid, keys.KindWeb, keys.KindLinux, keys.KindMac, keys.KindIOS:
	default:
		return nil, errors.New("unknown kind of device")
	}
	e.mu.Lock()
	ident, dev, v := e.ident, e.dev, e.v
	e.mu.Unlock()
	if ident == nil || v == nil {
		return nil, errLocked
	}
	cert, err := ident.CertifyPublic(sk, kk, req.Kind, req.Name, time.Now())
	if err != nil {
		return nil, err
	}
	if err := e.call("POST", "/api/v1/devices", map[string]any{"cert": cert}, nil); err != nil {
		return nil, err
	}
	vd, err := cert.Verify()
	if err != nil {
		return nil, err
	}

	// Every family (and the private journal): every epoch key this device holds, wrapped to the new device.
	e.SyncAll() // the current list of spaces (the journal may be new)
	var list []spaceView
	_ = v.Get(vault.State, "spaces", &list)
	shared := 0
	for _, s := range list {
		sv, err := e.spaceView(s.Space.ID)
		if err != nil || sv.Me.ReadOnly {
			continue // read-only access can't share keys; another member can approve the new device
		}
		for epoch := uint64(1); epoch <= sv.Space.Epoch; epoch++ {
			key, ok := e.epochKey(v, s.Space.ID, epoch)
			if !ok {
				continue
			}
			g, err := keys.Grant(dev, vd, s.Space.ID, epoch, key, sv.Me.ReadOnly, sv.Me.NotAfter, time.Now())
			pqc.Wipe(key)
			if err != nil {
				return nil, err
			}
			in, err := e.intent(audit.Intent{SpaceID: s.Space.ID, Action: audit.KeysGrant, Detail: "person=" + e.person})
			if err != nil {
				return nil, err
			}
			if err := e.call("POST", "/api/v1/spaces/"+s.Space.ID+"/keys", map[string]any{"epoch": epoch, "grants": []keys.EpochGrant{*g}, "intent": in}, nil); err != nil {
				return nil, err
			}
		}
		shared++
	}

	var name string
	_ = v.Get(vault.State, "my-name", &name)
	plain, _ := json.Marshal(linkBundle{Identity: ident.Key.Bytes(), Cert: *cert, Name: name})
	defer pqc.Wipe(plain)
	b := pqc.NewKey()
	defer pqc.Wipe(b)
	wrapped, err := pqc.Wrap(kk, "pp-link", b)
	if err != nil {
		return nil, err
	}
	bk := pqc.Derive(append(append([]byte{}, b...), k...), nil, "link-bundle", pqc.KeySize)
	defer pqc.Wipe(bk)
	box, err := pqc.Seal(bk, []byte("pp/link/answer|"+id), plain)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(linkAnswer{Wrapped: wrapped, Box: box})
	if err := e.call("POST", "/api/v1/link/"+id+"/answer", map[string]any{"payload": payload}, nil); err != nil {
		return nil, err
	}
	return map[string]any{"linked": req.Name, "families": shared}, nil
}

// SetupLinkedRequest sets this device up as another device of an existing person.
type SetupLinkedRequest struct {
	Server       string `json:"server"`
	CertSHA256   []byte `json:"cert_sha256"`
	Code         string `json:"code"`
	Passphrase   string `json:"passphrase"`
	DeviceSecret []byte `json:"device_secret,omitempty"`
	DeviceName   string `json:"device_name"`
	DeviceKind   string `json:"device_kind"`
}

// SetupLinked asks to be linked with the code, waits (up to ten minutes) for the person's other device to confirm,
// then stores what it sent and signs in.
func (e *Engine) SetupLinked(req SetupLinkedRequest) (*SetupResult, error) {
	secret, err := parseLinkCode(req.Code)
	if err != nil {
		return nil, err
	}
	id, k := linkKeys(secret)
	defer pqc.Wipe(k)
	if len(req.Passphrase) < 8 {
		return nil, errors.New("the passphrase must be at least 8 characters")
	}
	e.mu.Lock()
	if vault.Exists(e.vaultDir()) {
		e.mu.Unlock()
		return nil, errors.New("this device is already set up")
	}
	prev, err := e.previewLocked(req.Server, req.CertSHA256)
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	srv := prev.client
	dev, err := keys.NewDevice()
	if err != nil {
		return nil, err
	}
	plain, _ := json.Marshal(linkRequest{SignKey: dev.Sign.Public().Bytes(), KEMKey: dev.KEM.Public().Bytes(),
		Name: strings.TrimSpace(req.DeviceName), Kind: req.DeviceKind})
	sealed, err := pqc.Seal(k, []byte("pp/link/request|"+id), plain)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Minute)
	defer cancel()
	if err := srv.Call(ctx, "PUT", "/api/v1/link/"+id, map[string]any{"payload": sealed}, nil); err != nil {
		if remote.IsStatus(err, 409) || remote.IsStatus(err, 404) {
			return nil, errors.New("that link code has expired or already been used - make a new one on your other device")
		}
		return nil, err
	}
	var res struct {
		Payload []byte `json:"payload"`
	}
	for {
		err := srv.Call(ctx, "GET", "/api/v1/link/"+id+"/answer", nil, &res)
		if err == nil {
			break
		}
		if !remote.IsStatus(err, 409) || ctx.Err() != nil {
			if remote.IsStatus(err, 404) || ctx.Err() != nil {
				return nil, errors.New("your other device didn't confirm in time - make a new link code and try again")
			}
			return nil, err
		}
		select {
		case <-ctx.Done():
		case <-time.After(4 * time.Second): // within the server's limit for requests that aren't signed in
		}
	}
	var ans linkAnswer
	if json.Unmarshal(res.Payload, &ans) != nil {
		return nil, errors.New("unreadable answer from your other device")
	}
	b, err := dev.KEM.Unwrap("pp-link", ans.Wrapped)
	if err != nil {
		return nil, errors.New("the answer wasn't for this device")
	}
	bk := pqc.Derive(append(b, k...), nil, "link-bundle", pqc.KeySize)
	pqc.Wipe(b)
	bundleRaw, err := pqc.Open(bk, []byte("pp/link/answer|"+id), ans.Box)
	pqc.Wipe(bk)
	if err != nil {
		return nil, errors.New("the answer didn't come from a device with this code")
	}
	defer pqc.Wipe(bundleRaw)
	var bundle linkBundle
	if json.Unmarshal(bundleRaw, &bundle) != nil {
		return nil, errors.New("unreadable answer from your other device")
	}
	ik, err := pqc.SigningKeyFromSeed(bundle.Identity)
	if err != nil {
		return nil, errors.New("the identity from your other device is invalid")
	}
	ident := &keys.Identity{Key: ik}
	vd, err := bundle.Cert.Verify()
	if err != nil || vd.DeviceID != dev.DeviceID() || vd.PersonID != ident.PersonID() {
		return nil, errors.New("the certificate from your other device isn't for this device")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	v, err := vault.Create(e.vaultDir(), []byte(req.Passphrase), req.DeviceSecret)
	if err != nil {
		return nil, err
	}
	kem, _ := dev.KEM.Bytes()
	if err := firstErr(
		v.Put(vault.Keys, "identity", ident.Key.Bytes()),
		v.Put(vault.Keys, "device", deviceKeys{Sign: dev.Sign.Bytes(), KEM: kem}),
		v.Put(vault.Keys, "cert", bundle.Cert),
		v.Put(vault.Keys, "server", prev.info),
		v.Put(vault.State, "my-name", bundle.Name),
	); err != nil {
		v.Lock()
		return nil, err
	}
	pqc.Wipe(kem)
	e.v, e.srv, e.ident, e.dev, e.person, e.server = v, srv, ident, dev, ident.PersonID(), prev.info
	sctx, scancel := context.WithTimeout(context.Background(), time.Minute)
	defer scancel()
	if err := signIn(sctx, srv, dev, e.serverKey(), v); err != nil {
		return nil, err
	}
	e.startLocked()
	return &SetupResult{Person: e.person}, nil
}
