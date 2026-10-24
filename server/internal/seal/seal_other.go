//go:build !windows

package seal

import "errors"

// On Linux the master key must come from a systemd encrypted credential (see Load). A plain key file is only
// accepted for development, when PP_DEV_PLAIN_KEY=1 is set, because it isn't protected by anything but file
// permissions.

func protect(plain []byte) ([]byte, error) {
	if !devPlain() {
		return nil, errNoCredential
	}
	return append([]byte(nil), plain...), nil
}

func unprotect(sealed []byte) ([]byte, error) {
	if !devPlain() {
		return nil, errNoCredential
	}
	if len(sealed) != 32 {
		return nil, errors.New("seal: bad development key file")
	}
	return append([]byte(nil), sealed...), nil
}

var errNoCredential = errors.New("seal: no systemd credential master_key - install with the ParallelPath installer " +
	"(it seals the key with systemd-creds), or set PP_DEV_PLAIN_KEY=1 for development only")
