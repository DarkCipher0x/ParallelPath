// Package atrest checks that the server's data lives on an encrypted volume (BitLocker on Windows, LUKS/dm-crypt
// on Linux). ParallelPath refuses to start otherwise: encryption at rest is required, not optional.
//
// On Windows a low-privilege account (the server's own service account) can't always find out: BitLocker only
// answers administrators, and the fallback the shell offers says "unknown" inside a service. So setup - which runs
// as administrator - verifies the volume and records an Attestation, bound to that exact volume. The service uses
// it only when it can't get an answer itself; a definite "not encrypted" always wins.
package atrest

import (
	"errors"
	"fmt"
	"time"
)

// ErrUnknown means this account can't find out whether the volume is encrypted.
var ErrUnknown = errors.New("atrest: this account can't query the volume's encryption")

// Check returns nil if path is on an encrypted volume, an error wrapping ErrUnknown if that can't be determined from
// this account, or another error explaining what's missing.
func Check(path string) error { return check(path) }

// VolumeID identifies the volume holding path (on Windows its \\?\Volume{GUID}\ name), or "" where there's no need.
func VolumeID(path string) (string, error) { return volumeID(path) }

// Attestation records that an administrator verified a volume's encryption.
type Attestation struct {
	Volume string    `json:"volume"`
	Time   time.Time `json:"time"`
}

// Attest verifies path's volume now (this must run as administrator) and returns the record to keep.
func Attest(path string, now time.Time) (*Attestation, error) {
	if err := Check(path); err != nil {
		return nil, err
	}
	id, err := VolumeID(path)
	if err != nil || id == "" {
		return nil, err
	}
	return &Attestation{Volume: id, Time: now.UTC()}, nil
}

// Decide is the server's start-up rule. It returns how the data is protected, or why the server must not start.
//   - verified encrypted: fine;
//   - can't tell from here: fine only with an administrator's attestation for this same volume;
//   - anything else: refused, unless allowUnencrypted (testing only).
func Decide(checkErr error, att *Attestation, volume string, allowUnencrypted bool) (string, error) {
	switch {
	case checkErr == nil:
		return "encrypted volume", nil
	case errors.Is(checkErr, ErrUnknown) && att != nil && volume != "" && att.Volume == volume:
		return "encrypted volume (verified by setup " + att.Time.Format("2 Jan 2006") + ")", nil
	case allowUnencrypted:
		return "NOT encrypted (allowed for testing)", nil
	case errors.Is(checkErr, ErrUnknown):
		return "", fmt.Errorf("%w; run ParallelPath Server Setup again (as administrator) to verify the drive", checkErr)
	default:
		return "", checkErr
	}
}
