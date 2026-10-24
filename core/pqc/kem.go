package pqc

import (
	"crypto/hpke"
	"errors"
	"fmt"
)

// Hybrid public-key encryption: HPKE with the MLKEM1024-P384 KEM (draft-ietf-hpke-pq), HKDF-SHA512 and
// AES-256-GCM. Used to wrap small secrets (content keys, space epoch keys) to a device. Bulk data never goes through
// HPKE directly; it's sealed with a random key (see Seal) and only that key is wrapped.

var (
	kem  = hpke.MLKEM1024P384()
	kdf  = hpke.HKDFSHA512()
	aead = hpke.AES256GCM()
)

// ErrDecrypt is returned for any failure to open: wrong key, tampered data or wrong context.
var ErrDecrypt = errors.New("pqc: decryption failed")

// KEMKey is a device's private decryption key.
type KEMKey struct {
	sk  hpke.PrivateKey
	pub KEMPublicKey
}

// KEMPublicKey is the published encryption key of a device.
type KEMPublicKey struct {
	pk  hpke.PublicKey
	raw []byte
}

// NewKEMKey generates a fresh hybrid decryption key.
func NewKEMKey() (*KEMKey, error) {
	sk, err := kem.GenerateKey()
	if err != nil {
		return nil, err
	}
	return wrapKEM(sk)
}

// KEMKeyFromBytes restores a decryption key from its private encoding.
func KEMKeyFromBytes(b []byte) (*KEMKey, error) {
	sk, err := kem.NewPrivateKey(b)
	if err != nil {
		return nil, ErrBadKey
	}
	return wrapKEM(sk)
}

func wrapKEM(sk hpke.PrivateKey) (*KEMKey, error) {
	pub, err := ParseKEMPublicKey(sk.PublicKey().Bytes())
	if err != nil {
		return nil, err
	}
	return &KEMKey{sk: sk, pub: pub}, nil
}

// Bytes returns the private encoding. Callers must encrypt it before it touches storage.
func (k *KEMKey) Bytes() ([]byte, error) { return k.sk.Bytes() }

// Public returns the encryption key to publish.
func (k *KEMKey) Public() KEMPublicKey { return k.pub }

// ParseKEMPublicKey decodes a published encryption key.
func ParseKEMPublicKey(raw []byte) (KEMPublicKey, error) {
	pk, err := kem.NewPublicKey(raw)
	if err != nil {
		return KEMPublicKey{}, ErrBadKey
	}
	return KEMPublicKey{pk: pk, raw: append([]byte(nil), raw...)}, nil
}

// Bytes returns the published encoding.
func (p KEMPublicKey) Bytes() []byte { return append([]byte(nil), p.raw...) }

// Wrap encrypts a small secret to the holder of p. context binds the ciphertext to its use (for example
// "space-epoch|<space id>|<epoch>|<device id>"); the same context must be given to Unwrap.
func Wrap(p KEMPublicKey, context string, secret []byte) ([]byte, error) {
	if p.pk == nil {
		return nil, ErrBadKey
	}
	ct, err := hpke.Seal(p.pk, kdf, aead, info("wrap", context), secret)
	if err != nil {
		return nil, fmt.Errorf("pqc: wrap: %w", err)
	}
	return ct, nil
}

// Unwrap reverses Wrap.
func (k *KEMKey) Unwrap(context string, ct []byte) ([]byte, error) {
	pt, err := hpke.Open(k.sk, kdf, aead, info("wrap", context), ct)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

func info(kind, context string) []byte { return []byte("PP-HPKE-v1|" + kind + "|" + context) }
