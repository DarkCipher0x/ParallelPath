package pqc

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password hashing and password-derived keys: Argon2id. Memory-hard, so guessing is expensive on GPUs, and
// Grover's algorithm gives an attacker no meaningful speed-up against it.

// PasswordParams are the Argon2id costs. Stored with each hash so they can be raised later.
type PasswordParams struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// DefaultPasswordParams: 64 MiB, 3 passes, 4 lanes (OWASP's strongest Argon2id profile).
var DefaultPasswordParams = PasswordParams{MemoryKiB: 64 * 1024, Time: 3, Threads: 4}

// PasswordKey derives a 256-bit key from a password (for unlocking a vault or a recovery kit).
func PasswordKey(password []byte, salt []byte, p PasswordParams) []byte {
	return argon2.IDKey(password, salt, p.Time, p.MemoryKiB, p.Threads, KeySize)
}

// NewSalt returns 32 random bytes.
func NewSalt() []byte {
	s := make([]byte, 32)
	if _, err := rand.Read(s); err != nil {
		panic("pqc: system randomness unavailable: " + err.Error())
	}
	return s
}

// HashPassword returns a self-describing hash: $argon2id$v=19$m=..,t=..,p=..$salt$hash
func HashPassword(password []byte) string {
	p := DefaultPasswordParams
	salt := NewSalt()
	h := PasswordKey(password, salt, p)
	b := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", p.MemoryKiB, p.Time, p.Threads, b.EncodeToString(salt), b.EncodeToString(h))
}

// CheckPassword verifies a password against HashPassword's output in constant time.
func CheckPassword(password []byte, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("pqc: unknown password hash format")
	}
	var p PasswordParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.MemoryKiB, &p.Time, &p.Threads); err != nil {
		return false, errors.New("pqc: bad password hash parameters")
	}
	if p.MemoryKiB < 19*1024 || p.Time < 2 || p.Threads < 1 {
		return false, errors.New("pqc: password hash parameters below the minimum")
	}
	b := base64.RawStdEncoding
	salt, err1 := b.DecodeString(parts[4])
	want, err2 := b.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) != KeySize {
		return false, errors.New("pqc: bad password hash encoding")
	}
	return Equal(PasswordKey(password, salt, p), want), nil
}

// NeedsRehash reports whether a stored hash uses weaker settings than the current defaults.
func NeedsRehash(encoded string) bool {
	var p PasswordParams
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return true
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.MemoryKiB, &p.Time, &p.Threads); err != nil {
		return true
	}
	d := DefaultPasswordParams
	return p.MemoryKiB < d.MemoryKiB || p.Time < d.Time
}
