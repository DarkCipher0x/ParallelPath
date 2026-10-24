package pqc

import (
	"crypto/ed25519"
	"crypto/mldsa"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
)

// Composite signatures: ML-DSA-87 + Ed25519. A signature is valid only if BOTH halves verify.
//
// Each half signs the same bound message
//
//	"PP-SIG-v1" || u8(len(purpose)) || purpose || SHA3-512(composite public key) || message
//
// so a half can't be lifted out and reused under another key, and a signature made for one purpose (say, an
// audit entry) can never be replayed as another (say, a device certificate).

const (
	sigSeedSize = 32 // ML-DSA seed and Ed25519 seed are both 32 bytes

	// SigningKeySize is the encoded private key: ML-DSA-87 seed || Ed25519 seed.
	SigningKeySize = 2 * sigSeedSize
	// VerifyKeySize is the encoded public key: ML-DSA-87 public key || Ed25519 public key.
	VerifyKeySize = mldsa.MLDSA87PublicKeySize + ed25519.PublicKeySize
	// SignatureSize is ML-DSA-87 signature || Ed25519 signature.
	SignatureSize = mldsa.MLDSA87SignatureSize + ed25519.SignatureSize
)

var (
	ErrBadSignature = errors.New("pqc: signature invalid")
	ErrBadKey       = errors.New("pqc: malformed key")
)

// SigningKey is a person's or device's hybrid signing key.
type SigningKey struct {
	pq   *mldsa.PrivateKey
	ed   ed25519.PrivateKey
	pub  VerifyKey
	seed [SigningKeySize]byte
}

// VerifyKey is the public half, safe to publish.
type VerifyKey struct {
	pq  *mldsa.PublicKey
	ed  ed25519.PublicKey
	raw []byte
	id  [64]byte // SHA3-512 of raw: binds signatures to this exact composite key
}

// NewSigningKey generates a fresh hybrid signing key.
func NewSigningKey() (*SigningKey, error) {
	var seed [SigningKeySize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}
	defer Wipe(seed[:])
	return SigningKeyFromSeed(seed[:])
}

// SigningKeyFromSeed rebuilds a key from its 64-byte encoding (as stored, encrypted, in a vault).
func SigningKeyFromSeed(seed []byte) (*SigningKey, error) {
	if len(seed) != SigningKeySize {
		return nil, ErrBadKey
	}
	pq, err := mldsa.NewPrivateKey(mldsa.MLDSA87(), seed[:sigSeedSize])
	if err != nil {
		return nil, fmt.Errorf("pqc: ML-DSA key: %w", err)
	}
	ed := ed25519.NewKeyFromSeed(seed[sigSeedSize:])
	k := &SigningKey{pq: pq, ed: ed}
	copy(k.seed[:], seed)
	raw := append(append(make([]byte, 0, VerifyKeySize), pq.PublicKey().Bytes()...), ed.Public().(ed25519.PublicKey)...)
	if k.pub, err = ParseVerifyKey(raw); err != nil {
		return nil, err
	}
	return k, nil
}

// Bytes returns the 64-byte private encoding. Callers must encrypt it before it touches storage.
func (k *SigningKey) Bytes() []byte { return append([]byte(nil), k.seed[:]...) }

// Public returns the verify key.
func (k *SigningKey) Public() VerifyKey { return k.pub }

// Sign signs msg for the given purpose (a short, fixed label such as "audit-entry").
func (k *SigningKey) Sign(purpose string, msg []byte) ([]byte, error) {
	m, err := boundMessage(purpose, k.pub.id[:], msg)
	if err != nil {
		return nil, err
	}
	pqSig, err := k.pq.Sign(rand.Reader, m, &mldsa.Options{Context: purpose})
	if err != nil {
		return nil, fmt.Errorf("pqc: ML-DSA sign: %w", err)
	}
	edSig := ed25519.Sign(k.ed, m)
	return append(pqSig, edSig...), nil
}

// ParseVerifyKey decodes and checks a published composite public key.
func ParseVerifyKey(raw []byte) (VerifyKey, error) {
	if len(raw) != VerifyKeySize {
		return VerifyKey{}, ErrBadKey
	}
	pq, err := mldsa.NewPublicKey(mldsa.MLDSA87(), raw[:mldsa.MLDSA87PublicKeySize])
	if err != nil {
		return VerifyKey{}, ErrBadKey
	}
	v := VerifyKey{pq: pq, ed: ed25519.PublicKey(append([]byte(nil), raw[mldsa.MLDSA87PublicKeySize:]...)), raw: append([]byte(nil), raw...)}
	v.id = Hash(v.raw)
	return v, nil
}

// Bytes returns the published encoding.
func (v VerifyKey) Bytes() []byte { return append([]byte(nil), v.raw...) }

// ID is the SHA3-512 fingerprint of the key: the stable identifier for a person or device.
func (v VerifyKey) ID() [64]byte { return v.id }

// IsZero reports whether the key is unset.
func (v VerifyKey) IsZero() bool { return v.pq == nil }

// Verify checks a composite signature. Both halves must be valid.
func (v VerifyKey) Verify(purpose string, msg, sig []byte) error {
	if v.pq == nil || len(sig) != SignatureSize {
		return ErrBadSignature
	}
	m, err := boundMessage(purpose, v.id[:], msg)
	if err != nil {
		return err
	}
	pqOK := mldsa.Verify(v.pq, m, sig[:mldsa.MLDSA87SignatureSize], &mldsa.Options{Context: purpose}) == nil
	edOK := ed25519.Verify(v.ed, m, sig[mldsa.MLDSA87SignatureSize:])
	if !pqOK || !edOK {
		return ErrBadSignature
	}
	return nil
}

func boundMessage(purpose string, keyID, msg []byte) ([]byte, error) {
	if len(purpose) == 0 || len(purpose) > 64 {
		return nil, errors.New("pqc: signature purpose must be 1-64 bytes")
	}
	b := make([]byte, 0, 9+1+len(purpose)+len(keyID)+8+len(msg))
	b = append(b, "PP-SIG-v1"...)
	b = append(b, byte(len(purpose)))
	b = append(b, purpose...)
	b = append(b, keyID...)
	b = binary.BigEndian.AppendUint64(b, uint64(len(msg)))
	return append(b, msg...), nil
}
