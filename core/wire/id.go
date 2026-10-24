package wire

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"
)

// IDs are 16 bytes, shown as 32 lowercase hex characters. The first 6 bytes are the creation time in
// milliseconds (so IDs sort by time and indexes stay compact); the other 10 are random (80 bits).

// NewID returns a fresh time-ordered ID.
func NewID() string {
	var b [16]byte
	ms := uint64(time.Now().UTC().UnixMilli())
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], ms)
	copy(b[:6], t[2:])
	if _, err := rand.Read(b[6:]); err != nil {
		panic("wire: system randomness unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// ErrBadID is returned for anything that isn't a well-formed ID.
var ErrBadID = errors.New("wire: malformed id")

// CheckID validates an ID received from outside.
func CheckID(id string) error {
	if len(id) != 32 {
		return ErrBadID
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return ErrBadID
		}
	}
	return nil
}

// Fingerprint renders a key fingerprint (first 16 bytes of its SHA3-512) as hex: the ID of a person or device.
func Fingerprint(id [64]byte) string { return hex.EncodeToString(id[:16]) }
