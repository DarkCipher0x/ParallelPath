// Package serverinfo is server.json in the data folder: what a running server publishes about itself for the
// installer and the operator (fingerprints people compare when they connect). It holds nothing secret.
package serverinfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Info describes the running server.
type Info struct {
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	ServerKey  string    `json:"server_key"`      // fingerprint of the server's signing key
	CertSHA256 string    `json:"tls_cert_sha256"` // the certificate pin the apps show
	Listen     string    `json:"listen"`
	Hosts      []string  `json:"hosts"`
	AtRest     string    `json:"at_rest"` // how the data folder is protected
	PID        int       `json:"pid"`
	Started    time.Time `json:"started"`
}

// Path is server.json inside a data directory.
func Path(data string) string { return filepath.Join(data, "server.json") }

// Write replaces server.json atomically.
func Write(data string, i *Info) error {
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path(data) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path(data))
}

// Read reads server.json.
func Read(data string) (*Info, error) {
	b, err := os.ReadFile(Path(data))
	if err != nil {
		return nil, err
	}
	var i Info
	return &i, json.Unmarshal(b, &i)
}

// Remove deletes server.json (the server is stopping, or an installer wants to see a fresh start).
func Remove(data string) { _ = os.Remove(Path(data)) }
