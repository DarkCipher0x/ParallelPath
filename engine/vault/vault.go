// Package vault is the encrypted local store on each device: the person's identity key, the device keys, space
// keys, the session, and a cache of (still encrypted) items.
//
// Unlocking: passphrase -> Argon2id -> key that unwraps a random 256-bit vault key. Every value in the vault file
// is sealed with the vault key (AES-256-GCM, AAD = bucket + key). Changing the passphrase only re-wraps the vault
// key. On Android the wrapped vault key is additionally sealed by the hardware keystore (the app layer does that
// and passes the result in as an extra secret).
package vault

import (
	"encoding/json"
	"errors"
	"time"

	"pp/core/pqc"
)

// Buckets.
const (
	Keys   = "keys"   // identity, device, session, server
	Epochs = "epochs" // space/epoch -> epoch key
	Certs  = "certs"  // device id -> verified certificate
	Items  = "items"  // space/seq -> encrypted item revision
	State  = "state"  // cursors, audit heads, names
)

var buckets = []string{Keys, Epochs, Certs, Items, State}

// ErrLocked / ErrWrongPassphrase / ErrNoVault are returned by Unlock.
var (
	ErrWrongPassphrase = errors.New("vault: wrong passphrase")
	ErrNoVault         = errors.New("vault: not set up yet")
	ErrExists          = errors.New("vault: already set up")
	ErrNotFound        = errors.New("vault: not found")
)

type header struct {
	Version int                `json:"version"`
	Salt    []byte             `json:"salt"`
	Params  pqc.PasswordParams `json:"params"`
	Wrapped []byte             `json:"wrapped"` // vault key sealed with the passphrase key
	Created time.Time          `json:"created"`
}

// Vault is an unlocked vault.
type Vault struct {
	db  kv
	key []byte
	dir string
}

// Exists reports whether a vault has been set up in dir.
func Exists(dir string) bool { return metaExists(dir) }

// Create sets up a new vault protected by passphrase (+ an optional device secret, e.g. from Android Keystore).
func Create(dir string, passphrase, deviceSecret []byte) (*Vault, error) {
	if Exists(dir) {
		return nil, ErrExists
	}
	if len(passphrase) < 8 {
		return nil, errors.New("vault: the passphrase must be at least 8 characters")
	}
	vk := pqc.NewKey()
	h := header{Version: 1, Salt: pqc.NewSalt(), Params: pqc.DefaultPasswordParams, Created: time.Now().UTC()}
	pk := passKey(passphrase, deviceSecret, h)
	defer pqc.Wipe(pk)
	var err error
	if h.Wrapped, err = pqc.Seal(pk, []byte("pp/vault-key/v1"), vk); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(h, "", "  ")
	if err := writeMeta(dir, b); err != nil {
		return nil, err
	}
	return open(dir, vk)
}

// Unlock opens an existing vault.
func Unlock(dir string, passphrase, deviceSecret []byte) (*Vault, error) {
	h, err := readHeader(dir)
	if err != nil {
		return nil, err
	}
	pk := passKey(passphrase, deviceSecret, *h)
	defer pqc.Wipe(pk)
	vk, err := pqc.Open(pk, []byte("pp/vault-key/v1"), h.Wrapped)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return open(dir, vk)
}

// ChangePassphrase re-wraps the vault key under a new passphrase.
func (v *Vault) ChangePassphrase(newPass, deviceSecret []byte) error {
	if len(newPass) < 8 {
		return errors.New("vault: the passphrase must be at least 8 characters")
	}
	h, err := readHeader(v.dir)
	if err != nil {
		return err
	}
	h.Salt, h.Params = pqc.NewSalt(), pqc.DefaultPasswordParams
	pk := passKey(newPass, deviceSecret, *h)
	defer pqc.Wipe(pk)
	if h.Wrapped, err = pqc.Seal(pk, []byte("pp/vault-key/v1"), v.key); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(h, "", "  ")
	return writeMeta(v.dir, b)
}

func readHeader(dir string) (*header, error) {
	b, err := readMeta(dir)
	if err != nil {
		return nil, err
	}
	var h header
	if err := json.Unmarshal(b, &h); err != nil || h.Version != 1 {
		return nil, errors.New("vault: unreadable header")
	}
	return &h, nil
}

func passKey(pass, deviceSecret []byte, h header) []byte {
	k := pqc.PasswordKey(pass, h.Salt, h.Params)
	if len(deviceSecret) == 0 {
		return k
	}
	defer pqc.Wipe(k)
	return pqc.Derive(append(k, deviceSecret...), h.Salt, "vault-device", pqc.KeySize)
}

func open(dir string, vk []byte) (*Vault, error) {
	db, err := openKV(dir)
	if err != nil {
		return nil, err
	}
	return &Vault{db: db, key: vk, dir: dir}, nil
}

// Lock closes the vault and wipes its key from memory.
func (v *Vault) Lock() error {
	pqc.Wipe(v.key)
	return v.db.Close()
}

func aad(bucket, key string) []byte { return []byte("vault|" + bucket + "|" + key) }

// Put stores v (JSON) under key.
func (v *Vault) Put(bucket, key string, val any) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return err
	}
	sealed, err := pqc.Seal(v.key, aad(bucket, key), raw)
	pqc.Wipe(raw)
	if err != nil {
		return err
	}
	return v.db.Put(bucket, key, sealed)
}

// Get loads key into out.
func (v *Vault) Get(bucket, key string, out any) error {
	sealed := v.db.Get(bucket, key)
	if sealed == nil {
		return ErrNotFound
	}
	raw, err := pqc.Open(v.key, aad(bucket, key), sealed)
	if err != nil {
		return errors.New("vault: a value failed to decrypt (file damaged or tampered)")
	}
	defer pqc.Wipe(raw)
	return json.Unmarshal(raw, out)
}

// Delete removes key.
func (v *Vault) Delete(bucket, key string) error {
	return v.db.Delete(bucket, key)
}

// Each visits all values under a key prefix, in key order. fn gets the raw JSON (already decrypted).
func (v *Vault) Each(bucket, prefix string, fn func(key string, raw []byte) bool) error {
	type entry struct{ k, s []byte }
	var all []entry
	v.db.Scan(bucket, prefix, func(k string, s []byte) bool {
		all = append(all, entry{[]byte(k), append([]byte(nil), s...)})
		return true
	})
	for _, e := range all {
		raw, err := pqc.Open(v.key, aad(bucket, string(e.k)), e.s)
		if err != nil {
			return errors.New("vault: a value failed to decrypt (file damaged or tampered)")
		}
		cont := fn(string(e.k), raw)
		pqc.Wipe(raw)
		if !cont {
			break
		}
	}
	return nil
}

// Dir is the vault's folder (the app keeps its decrypted-file cache next to it).
func (v *Vault) Dir() string { return v.dir }
