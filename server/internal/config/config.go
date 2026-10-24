// Package config is ppserver's settings file (config.json in the data directory). It holds no secrets.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"pp/server/internal/atrest"
)

// Config is the server's settings.
type Config struct {
	Name     string   `json:"name"`      // shown to people in the apps ("Smith family server")
	Listen   string   `json:"listen"`    // public HTTPS address, e.g. ":8443"
	Hosts    []string `json:"hosts"`     // names/IPs the self-signed certificate covers
	CertFile string   `json:"cert_file"` // tls_mode "files": a certificate and key the operator provides
	KeyFile  string   `json:"key_file"`
	// TLSMode is where the HTTPS certificate comes from: local-ca (this server's own CA, installed once per device),
	// acme (Let's Encrypt, for a public name), files (cert_file/key_file) or self (the original self-signed one).
	// Empty means: files if cert_file is set, self for servers that already have a self-signed certificate, else local-ca.
	TLSMode     string   `json:"tls_mode,omitempty"`
	ACMEDomains []string `json:"acme_domains,omitempty"` // acme: the public names (they must point at this server)
	ACMEEmail   string   `json:"acme_email,omitempty"`   // acme: optional contact for expiry notices
	// HTTPListen, when set (e.g. ":80"), redirects plain HTTP to HTTPS and answers Let's Encrypt HTTP-01 challenges.
	HTTPListen string `json:"http_listen,omitempty"`
	// The call relay (TURN). TURNListen defaults to ":3478" (UDP and TCP); "off" disables it. TURNRelayIP is the
	// address devices reach the relay on (default: guessed from hosts or the network). TURNPorts is the relay port
	// range (default "49160-49200"); open these and 3478 in the firewall for calls across the internet.
	TURNListen  string `json:"turn_listen,omitempty"`
	TURNRelayIP string `json:"turn_relay_ip,omitempty"`
	TURNPorts   string `json:"turn_ports,omitempty"`
	AdminListen string `json:"admin_listen"` // loopback only, e.g. "127.0.0.1:8444"
	MaxBlobMB   int64  `json:"max_blob_mb"`
	QuotaGB     int64  `json:"space_quota_gb"`
	// AllowUnencryptedVolume is for testing only: production refuses to start off BitLocker/LUKS.
	AllowUnencryptedVolume bool `json:"allow_unencrypted_volume,omitempty"`
	// BackupDir, when set, gets a daily copy of the (sealed) database; the newest BackupKeep copies are kept.
	BackupDir  string `json:"backup_dir,omitempty"`
	BackupKeep int    `json:"backup_keep,omitempty"`
	// AtRestVerified is setup's record (made as administrator) that the data volume is encrypted, for when the
	// server's own low-privilege account can't query that (see package atrest).
	AtRestVerified *atrest.Attestation `json:"at_rest_verified,omitempty"`
	// AdminAllow limits the admin portal to these networks (CIDR); empty = anywhere. This machine is always allowed.
	AdminAllow []string `json:"admin_allow,omitempty"`
}

// Default returns a fresh configuration.
func Default(name string, hosts []string) *Config {
	return &Config{Name: name, Listen: ":8443", Hosts: hosts, AdminListen: "127.0.0.1:8444", MaxBlobMB: 2048, QuotaGB: 50,
		BackupKeep: 14, TLSMode: "local-ca"}
}

// Path is the config file inside a data directory.
func Path(dataDir string) string { return filepath.Join(dataDir, "config.json") }

// Load reads the config file.
func Load(dataDir string) (*Config, error) {
	b, err := os.ReadFile(Path(dataDir))
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, c.Validate()
}

// Save writes the config file.
func (c *Config) Save(dataDir string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(Path(dataDir), append(b, '\n'), 0o600)
}

// Validate checks the settings make sense.
func (c *Config) Validate() error {
	switch {
	case c.Listen == "":
		return errors.New("config: listen is empty")
	case len(c.Hosts) == 0 && c.CertFile == "" && c.TLSMode != "acme":
		return errors.New("config: give hosts (the server's names or IPs) or cert_file/key_file")
	case c.TLSMode != "" && c.TLSMode != "local-ca" && c.TLSMode != "acme" && c.TLSMode != "files" && c.TLSMode != "self":
		return errors.New("config: tls_mode must be local-ca, acme, files or self")
	case c.TLSMode == "acme" && len(c.ACMEDomains) == 0:
		return errors.New("config: tls_mode acme needs acme_domains")
	case c.TLSMode == "files" && c.CertFile == "":
		return errors.New("config: tls_mode files needs cert_file and key_file")
	case (c.CertFile == "") != (c.KeyFile == ""):
		return errors.New("config: cert_file and key_file go together")
	case c.MaxBlobMB < 1 || c.QuotaGB < 1:
		return errors.New("config: max_blob_mb and space_quota_gb must be positive")
	case c.BackupDir != "" && !filepath.IsAbs(c.BackupDir):
		return errors.New("config: backup_dir must be an absolute path")
	case c.BackupDir != "" && (c.BackupKeep < 1 || c.BackupKeep > 365):
		return errors.New("config: backup_keep must be between 1 and 365")
	}
	return nil
}

// CertMode is the TLS certificate mode in effect (see TLSMode).
func (c *Config) CertMode(dataDir string) string {
	switch {
	case c.TLSMode != "":
		return c.TLSMode
	case c.CertFile != "":
		return "files"
	}
	if _, err := os.Stat(filepath.Join(dataDir, "tls-self.crt")); err == nil {
		return "self"
	}
	return "local-ca"
}
