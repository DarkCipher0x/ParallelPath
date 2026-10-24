package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"pp/core/keys"
	"pp/core/pqc"
	"pp/engine/vault"
)

// Recovery kits: a way back in if every device is lost. The kit - this person's identity key, their name and the
// keys of their private journal - is sealed on this device with a key derived (Argon2id) from a 192-bit recovery
// code, and kept on the server under a hash of the code. The person writes the code down; nobody else ever sees it.
// Recovering on a new device revokes all the old ones; each family's parents then approve the new device (after
// comparing safety numbers, which stay the same because the identity does).

type recoveryKit struct {
	Identity []byte            `json:"identity"`
	Name     string            `json:"name"`
	Journal  string            `json:"journal,omitempty"`
	Epochs   map[uint64][]byte `json:"journal_epochs,omitempty"`
	Made     time.Time         `json:"made"`
}

type sealedKit struct {
	Salt   []byte             `json:"salt"`
	Params pqc.PasswordParams `json:"params"`
	Box    []byte             `json:"box"`
}

// recoveryState is what this device keeps (in its vault) to refresh the kit later without the code.
type recoveryState struct {
	ID     string             `json:"id"`
	Key    []byte             `json:"key"`
	Salt   []byte             `json:"salt"`
	Params pqc.PasswordParams `json:"params"`
	Made   time.Time          `json:"made"`
}

func recoveryCode(secret []byte) string {
	s := b32.EncodeToString(secret)
	var parts []string
	for i := 0; i < len(s); i += 4 {
		parts = append(parts, s[i:min(i+4, len(s))])
	}
	return "R-" + strings.Join(parts, "-")
}

func parseRecoveryCode(code string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(code))
	s = strings.TrimPrefix(s, "R-") // the prefix with its dash: "R" alone is also a base32 letter
	s = strings.NewReplacer("-", "", " ", "").Replace(s)
	b, err := b32.DecodeString(s)
	if err != nil || len(b) != 24 {
		return nil, errors.New("that isn't a recovery code - it starts with R- and you wrote it down when you made your kit")
	}
	return b, nil
}

func recoveryID(secret []byte) string {
	h := pqc.Hash([]byte("pp/recovery-id/v1"), secret)
	return hex.EncodeToString(h[:16])
}

// RecoveryCreate makes (or replaces) this person's recovery kit and returns the code to write down.
func (e *Engine) RecoveryCreate() (map[string]string, error) {
	secret := make([]byte, 24)
	_, _ = rand.Read(secret)
	st := recoveryState{ID: recoveryID(secret), Salt: pqc.NewSalt(), Params: pqc.DefaultPasswordParams, Made: time.Now().UTC()}
	st.Key = pqc.PasswordKey(secret, st.Salt, st.Params)
	code := recoveryCode(secret)
	pqc.Wipe(secret)
	if err := e.saveKit(st); err != nil {
		return nil, err
	}
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return nil, errLocked
	}
	if err := v.Put(vault.State, "recovery", st); err != nil {
		return nil, err
	}
	return map[string]string{"code": code}, nil
}

// saveKit seals the current kit with the stored key and uploads it.
func (e *Engine) saveKit(st recoveryState) error {
	e.mu.Lock()
	v, ident := e.v, e.ident
	e.mu.Unlock()
	if v == nil || ident == nil {
		return errLocked
	}
	kit := recoveryKit{Identity: ident.Key.Bytes(), Made: time.Now().UTC(), Epochs: map[uint64][]byte{}}
	_ = v.Get(vault.State, "my-name", &kit.Name)
	if j, _ := e.journalSpace(false); j != "" {
		kit.Journal = j
		if sv, err := e.spaceView(j); err == nil {
			for ep := uint64(1); ep <= sv.Space.Epoch; ep++ {
				if k, ok := e.epochKey(v, j, ep); ok {
					kit.Epochs[ep] = k
				}
			}
		}
	}
	plain, _ := json.Marshal(kit)
	defer pqc.Wipe(plain)
	for _, k := range kit.Epochs {
		pqc.Wipe(k)
	}
	box, err := pqc.Seal(st.Key, []byte("pp/recovery/v1|"+st.ID), plain)
	if err != nil {
		return err
	}
	sealed, _ := json.Marshal(sealedKit{Salt: st.Salt, Params: st.Params, Box: box})
	return e.call("PUT", "/api/v1/recovery", map[string]any{"id": st.ID, "sealed": sealed}, nil)
}

// refreshRecovery re-seals the kit after something it holds changed (the journal was created).
func (e *Engine) refreshRecovery() {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	var st recoveryState
	if v != nil && v.Get(vault.State, "recovery", &st) == nil {
		_ = e.saveKit(st)
	}
}

// RecoveryStatus says whether this person has a kit, and when it was made.
func (e *Engine) RecoveryStatus() (map[string]any, error) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return nil, errLocked
	}
	var st recoveryState
	if v.Get(vault.State, "recovery", &st) != nil {
		return map[string]any{"has_kit": false}, nil
	}
	return map[string]any{"has_kit": true, "made": st.Made}, nil
}

// SetupRecoverRequest recovers this person's account on this (new) device.
type SetupRecoverRequest struct {
	Server       string `json:"server"`
	CertSHA256   []byte `json:"cert_sha256"`
	Code         string `json:"code"`
	Passphrase   string `json:"passphrase"`
	DeviceSecret []byte `json:"device_secret,omitempty"`
	DeviceName   string `json:"device_name"`
	DeviceKind   string `json:"device_kind"`
}

// SetupRecover fetches and opens the kit, registers this device (revoking the person's old ones) and signs in.
func (e *Engine) SetupRecover(req SetupRecoverRequest) (*SetupResult, error) {
	secret, err := parseRecoveryCode(req.Code)
	if err != nil {
		return nil, err
	}
	id := recoveryID(secret)
	if len(req.Passphrase) < 8 {
		return nil, errors.New("the passphrase must be at least 8 characters")
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var got struct {
		Sealed []byte `json:"sealed"`
	}
	if err := srv.Call(ctx, "GET", "/api/v1/recovery/"+id, nil, &got); err != nil {
		return nil, errors.New("there's no recovery kit for that code on this server")
	}
	var sk sealedKit
	if json.Unmarshal(got.Sealed, &sk) != nil {
		return nil, errors.New("the recovery kit is damaged")
	}
	key := pqc.PasswordKey(secret, sk.Salt, sk.Params)
	plain, err := pqc.Open(key, []byte("pp/recovery/v1|"+id), sk.Box)
	pqc.Wipe(key)
	if err != nil {
		return nil, errors.New("the recovery code doesn't open the kit")
	}
	defer pqc.Wipe(plain)
	var kit recoveryKit
	if json.Unmarshal(plain, &kit) != nil {
		return nil, errors.New("the recovery kit is damaged")
	}
	ik, err := pqc.SigningKeyFromSeed(kit.Identity)
	if err != nil {
		return nil, errors.New("the recovery kit is damaged")
	}
	ident := &keys.Identity{Key: ik}
	dev, err := keys.NewDevice()
	if err != nil {
		return nil, err
	}
	cert, err := ident.Certify(dev, req.DeviceKind, strings.TrimSpace(req.DeviceName), time.Now())
	if err != nil {
		return nil, err
	}
	if err := srv.Call(ctx, "POST", "/api/v1/recovery/"+id+"/device", map[string]any{"cert": cert}, nil); err != nil {
		return nil, err
	}
	v, err := vault.Create(e.vaultDir(), []byte(req.Passphrase), req.DeviceSecret)
	if err != nil {
		return nil, err
	}
	kem, _ := dev.KEM.Bytes()
	errs := []error{
		v.Put(vault.Keys, "identity", ident.Key.Bytes()),
		v.Put(vault.Keys, "device", deviceKeys{Sign: dev.Sign.Bytes(), KEM: kem}),
		v.Put(vault.Keys, "cert", cert),
		v.Put(vault.Keys, "server", prev.info),
		v.Put(vault.State, "my-name", kit.Name),
	}
	if kit.Journal != "" { // the private journal: nobody else holds its keys, so they came in the kit
		errs = append(errs, v.Put(vault.State, "journal-space", kit.Journal))
		for ep, k := range kit.Epochs {
			errs = append(errs, v.Put(vault.Epochs, epochKeyName(kit.Journal, ep), k))
			pqc.Wipe(k)
		}
	}
	pqc.Wipe(kem)
	if err := firstErr(errs...); err != nil {
		v.Lock()
		return nil, err
	}
	e.v, e.srv, e.ident, e.dev, e.person, e.server = v, srv, ident, dev, ident.PersonID(), prev.info
	if err := signIn(ctx, srv, dev, e.serverKey(), v); err != nil {
		return nil, err
	}
	e.startLocked()
	return &SetupResult{Person: e.person, Waiting: true}, nil
}
