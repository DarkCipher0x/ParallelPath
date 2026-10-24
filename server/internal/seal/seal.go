// Package seal protects the server's 32-byte master key at rest, using the operating system's key protection:
//
//   - Windows: DPAPI, bound to the account the service runs as (only that account on this machine can unseal it).
//   - Linux: a systemd encrypted credential ($CREDENTIALS_DIRECTORY/master_key, sealed with the TPM where one
//     exists, otherwise the host key). The installer sets this up with systemd-creds.
//
// Every server secret is derived from the master key with a distinct label (data key, signing key, ...), so only
// one value ever needs protecting.
package seal

import (
	"errors"
	"os"
	"path/filepath"

	"pp/core/pqc"
)

// Keys derived from the master key.
type Keys struct {
	Data    []byte          // seals every database value
	Signing *pqc.SigningKey // countersigns audit entries; its public key identifies this server
	Session []byte          // derives session-token hashes
	TLS     []byte          // seals the local certificate authority's private key
	TURN    []byte          // signs the call relay's short-lived credentials
}

// Derive expands the master key into the server's working keys.
func Derive(master []byte) (*Keys, error) {
	if len(master) != pqc.KeySize {
		return nil, errors.New("seal: master key must be 32 bytes")
	}
	seed := pqc.Derive(master, nil, "server-signing", pqc.SigningKeySize)
	defer pqc.Wipe(seed)
	sk, err := pqc.SigningKeyFromSeed(seed)
	if err != nil {
		return nil, err
	}
	return &Keys{
		Data:    pqc.Derive(master, nil, "server-data", pqc.KeySize),
		Signing: sk,
		Session: pqc.Derive(master, nil, "server-session", pqc.KeySize),
		TLS:     pqc.Derive(master, nil, "server-tls-ca", pqc.KeySize),
		TURN:    pqc.Derive(master, nil, "server-turn", pqc.KeySize),
	}, nil
}

// sealedFile is where a DPAPI/plain-file master key lives in the data directory.
func sealedFile(dataDir string) string { return filepath.Join(dataDir, "master.sealed") }

// Load returns the master key, creating and sealing a new one on first run (create=true).
func Load(dataDir string, create bool) ([]byte, error) {
	if k, ok, err := fromCredential(); ok || err != nil {
		return k, err
	}
	path := sealedFile(dataDir)
	blob, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return nil, errors.New("seal: no master key yet - run `ppserver init` first")
		}
		master := pqc.NewKey()
		sealed, err := protect(master)
		if err != nil {
			return nil, err
		}
		if err := writeExclusive(path, sealed); err != nil {
			return nil, err
		}
		return master, nil
	}
	if err != nil {
		return nil, err
	}
	return unprotect(blob)
}

// fromCredential reads a systemd credential if the service was started with one.
func fromCredential() ([]byte, bool, error) {
	dir := os.Getenv("CREDENTIALS_DIRECTORY")
	if dir == "" {
		return nil, false, nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "master_key"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if len(b) != pqc.KeySize {
		return nil, true, errors.New("seal: systemd credential master_key must be 32 raw bytes")
	}
	return b, true, nil
}

func writeExclusive(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
