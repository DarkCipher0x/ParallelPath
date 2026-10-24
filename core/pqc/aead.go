package pqc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// Symmetric encryption: AES-256-GCM. Every key is random and used for one item (or one file), so the 96-bit random
// nonce can't realistically repeat. The associated data binds each ciphertext to where it belongs; moving it
// elsewhere makes it fail to open.

// KeySize is the size of every symmetric key.
const KeySize = 32

const nonceSize = 12

// NewKey returns a fresh random 256-bit key.
func NewKey() []byte {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		panic("pqc: system randomness unavailable: " + err.Error())
	}
	return k
}

// Seal encrypts plaintext with key, bound to aad. Output: nonce || ciphertext+tag.
func Seal(key, aad, plaintext []byte) ([]byte, error) {
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, nonceSize, nonceSize+len(plaintext)+g.Overhead())
	if _, err := rand.Read(out); err != nil {
		return nil, err
	}
	return g.Seal(out, out[:nonceSize], plaintext, aad), nil
}

// Open decrypts the output of Seal. Any change to the key, the data or the aad fails with ErrDecrypt.
func Open(key, aad, sealed []byte) ([]byte, error) {
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < nonceSize+g.Overhead() {
		return nil, ErrDecrypt
	}
	pt, err := g.Open(nil, sealed[:nonceSize], sealed[nonceSize:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// SealedSize is the size of Seal's output for n bytes of plaintext.
func SealedSize(n int) int { return nonceSize + n + 16 }

func gcm(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, errors.New("pqc: key must be 32 bytes")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}
