package pqc

import (
	"crypto/hkdf"
	"crypto/sha3"
	"crypto/sha512"
	"crypto/subtle"
	"runtime"
)

// Hash is SHA3-512: audit chains, content IDs, key fingerprints.
func Hash(parts ...[]byte) [64]byte {
	h := sha3.New512()
	for _, p := range parts {
		h.Write(p)
	}
	var out [64]byte
	h.Sum(out[:0])
	return out
}

// Derive turns secret material into a key for one purpose (HKDF-SHA512). Different labels always give unrelated
// keys, so one secret can safely feed several uses.
func Derive(secret, salt []byte, label string, size int) []byte {
	k, err := hkdf.Key(sha512.New, secret, salt, "PP-KDF-v1|"+label, size)
	if err != nil {
		panic("pqc: hkdf: " + err.Error()) // only possible for an absurd size
	}
	return k
}

// Equal compares secrets in constant time.
func Equal(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }

// Wipe overwrites a secret in memory once it's no longer needed (best effort: Go may have copied it).
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}
