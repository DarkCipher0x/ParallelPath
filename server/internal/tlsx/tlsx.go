// Package tlsx is the server's TLS policy: TLS 1.3 only, and only hybrid post-quantum key exchange.
//
//	SecP384r1MLKEM1024  ML-KEM-1024 + P-384  (category 5; used by the ParallelPath apps)
//	X25519MLKEM768      ML-KEM-768 + X25519  (what current browsers support)
//
// A client that can't do either is refused during the handshake: no session key is ever agreed with classical
// cryptography alone, so recorded traffic can't be decrypted later by a quantum computer. (The one exception is
// Let's Encrypt's TLS-ALPN-01 validation handshake, which carries no data and only proves control of the name.)
//
// Where the certificate comes from (config "tls_mode"):
//
//	local-ca  this server's own certificate authority, limited by X.509 name constraints to the server's own names,
//	          issuing short-lived server certificates. Install the CA on each device once (the home page links it)
//	          and browsers trust the server with no warnings. The apps pin the CA, so renewals need nothing.
//	acme      a free, automatically renewed certificate from Let's Encrypt, for a server with a public name.
//	files     a certificate and key the operator provides (e.g. from their own CA).
//	self      the original self-signed certificate (kept for servers set up that way; the apps pin it).
package tlsx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"pp/core/pqc"
)

// Groups is the only key exchange the server accepts, in preference order.
var Groups = []tls.CurveID{tls.SecP384r1MLKEM1024, tls.X25519MLKEM768}

// Modes.
const (
	ModeLocalCA = "local-ca"
	ModeACME    = "acme"
	ModeFiles   = "files"
	ModeSelf    = "self"
)

// Options say where the server's certificate comes from.
type Options struct {
	Mode        string
	DataDir     string
	Name        string   // the server's name (shown in the CA's name)
	Hosts       []string // names and IPs the certificate covers (local-ca, self)
	CertFile    string   // files
	KeyFile     string
	ACMEDomains []string // acme
	ACMEEmail   string
	KEK         []byte // seals the local CA's private key at rest (derived from the server's master key)
}

// Source serves the server's certificate and keeps it current.
type Source struct {
	mode  string
	opt   Options
	cert  atomic.Pointer[tls.Certificate]
	acme  *autocert.Manager
	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey
	caPEM []byte
	// Uncovered are configured names the local CA can't vouch for (added after it was made: it's limited to the
	// names it was created with). The admin portal offers to make a new CA for them.
	Uncovered []string
}

// Open loads (or creates) the certificate for the configured mode.
func Open(o Options) (*Source, error) {
	s := &Source{mode: o.Mode, opt: o}
	switch o.Mode {
	case ModeFiles:
		c, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("tls: the certificate files can't be used: %w", err)
		}
		s.cert.Store(&c)
	case ModeSelf:
		c, err := loadOrCreateSelf(o.DataDir, o.Hosts)
		if err != nil {
			return nil, err
		}
		s.cert.Store(&c)
	case ModeACME:
		if len(o.ACMEDomains) == 0 {
			return nil, errors.New("tls: Let's Encrypt needs the server's public domain name")
		}
		s.acme = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(o.ACMEDomains...),
			Cache:      autocert.DirCache(filepath.Join(o.DataDir, "acme")),
			Email:      o.ACMEEmail,
			Client:     &acme.Client{DirectoryURL: acme.LetsEncryptURL},
		}
	case ModeLocalCA, "":
		s.mode = ModeLocalCA
		if err := s.openCA(); err != nil {
			return nil, err
		}
		if err := s.Renew(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("tls: unknown tls_mode %q", o.Mode)
	}
	return s, nil
}

// Mode is the certificate mode in use.
func (s *Source) Mode() string { return s.mode }

// Config is the TLS configuration for the public listener.
func (s *Source) Config() *tls.Config {
	base := &tls.Config{
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: Groups,
		NextProtos:       []string{"h2", "http/1.1"},
		GetCertificate:   s.getCertificate,
	}
	if s.acme != nil {
		base.NextProtos = append(base.NextProtos, acme.ALPNProto)
		base.GetConfigForClient = func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			if slices.Contains(h.SupportedProtos, acme.ALPNProto) {
				// Let's Encrypt validating the name: no application data flows on this connection.
				c := base.Clone()
				c.CurvePreferences, c.NextProtos, c.GetConfigForClient = nil, []string{acme.ALPNProto}, nil
				return c, nil
			}
			return nil, nil
		}
	}
	return base
}

func (s *Source) getCertificate(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if s.acme != nil {
		return s.acme.GetCertificate(h)
	}
	return s.cert.Load(), nil
}

// HTTPHandler answers Let's Encrypt HTTP-01 challenges (for a plain-HTTP port 80 listener) and redirects everything
// else to HTTPS. Without ACME it only redirects.
func (s *Source) HTTPHandler(httpsPort string) http.Handler {
	redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if httpsPort != "" && httpsPort != "443" {
			host = net.JoinHostPort(host, httpsPort)
		}
		http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
	if s.acme != nil {
		return s.acme.HTTPHandler(redirect)
	}
	return redirect
}

// Fingerprint is the SHA-256 the apps pin: the local CA (stable across renewals), or the self-signed certificate.
// Nil for publicly trusted certificates (nothing to pin; the apps verify them normally).
func (s *Source) Fingerprint() []byte {
	switch s.mode {
	case ModeLocalCA:
		return sum256(s.ca.Raw)
	case ModeSelf:
		return sum256(s.cert.Load().Certificate[0])
	case ModeFiles:
		c := s.cert.Load()
		return sum256(c.Certificate[len(c.Certificate)-1])
	}
	return nil
}

// CACertPEM is the local CA's certificate, for devices to install (nil in other modes).
func (s *Source) CACertPEM() []byte { return s.caPEM }

// CACert is the local CA (nil in other modes).
func (s *Source) CACert() *x509.Certificate { return s.ca }

func sum256(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

// ---- local CA

func (s *Source) caFiles() (string, string) {
	return filepath.Join(s.opt.DataDir, "tls-ca.crt"), filepath.Join(s.opt.DataDir, "tls-ca.key.sealed")
}

func (s *Source) openCA() error {
	cf, kf := s.caFiles()
	certPEM, err1 := os.ReadFile(cf)
	sealed, err2 := os.ReadFile(kf)
	if err1 == nil && err2 == nil {
		b, _ := pem.Decode(certPEM)
		if b == nil {
			return errors.New("tls: the local CA certificate is damaged")
		}
		ca, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return err
		}
		der, err := pqc.Open(s.opt.KEK, []byte("pp/tls-ca-key/v1"), sealed)
		if err != nil {
			return errors.New("tls: the local CA's key can't be unsealed (wrong master key?)")
		}
		defer pqc.Wipe(der)
		k, err := x509.ParseECPrivateKey(der)
		if err != nil {
			return err
		}
		s.ca, s.caKey, s.caPEM = ca, k, certPEM
		return nil
	}
	if len(s.opt.Hosts) == 0 {
		return errors.New("tls: the local CA needs at least one host name or IP for the server")
	}
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "ParallelPath Local CA - " + s.opt.Name, Organization: []string{"ParallelPath"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		// It can only ever vouch for this server's own names: installing it doesn't let it impersonate anyone else.
		PermittedDNSDomainsCritical: true,
	}
	for _, h := range s.opt.Hosts {
		if ip := net.ParseIP(h); ip != nil {
			bits := 128
			if ip.To4() != nil {
				ip, bits = ip.To4(), 32
			}
			tpl.PermittedIPRanges = append(tpl.PermittedIPRanges, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		} else {
			tpl.PermittedDNSDomains = append(tpl.PermittedDNSDomains, h)
		}
	}
	if len(tpl.PermittedDNSDomains) == 0 {
		tpl.ExcludedDNSDomains = []string{""} // no names at all, only the listed IPs
	}
	if len(tpl.PermittedIPRanges) == 0 {
		tpl.ExcludedIPRanges = []*net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	sealed, err = pqc.Seal(s.opt.KEK, []byte("pp/tls-ca-key/v1"), kder)
	pqc.Wipe(kder)
	if err != nil {
		return err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(kf, sealed, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(cf, certPEM, 0o644); err != nil {
		return err
	}
	ca, _ := x509.ParseCertificate(der)
	s.ca, s.caKey, s.caPEM = ca, key, certPEM
	return nil
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n
}

// leafLife is how long the local CA's server certificates last (browsers expect well under 398 days).
const leafLife = 200 * 24 * time.Hour

// Renew issues a new server certificate from the local CA when the current one is missing, expires within 30
// days or doesn't cover the configured names. The server calls it at start-up and daily.
func (s *Source) Renew() error {
	if s.mode != ModeLocalCA {
		return nil
	}
	hosts := s.permitted()
	if len(hosts) == 0 {
		return errors.New("tls: the local CA covers none of the server's names - make a new CA (admin portal)")
	}
	if c := s.cert.Load(); c != nil && c.Leaf != nil && time.Until(c.Leaf.NotAfter) > 30*24*time.Hour && covers(c.Leaf, hosts) {
		return nil
	}
	lf, kf := filepath.Join(s.opt.DataDir, "tls-leaf.crt"), filepath.Join(s.opt.DataDir, "tls-leaf.key")
	if c, err := tls.LoadX509KeyPair(lf, kf); err == nil && c.Leaf != nil && time.Until(c.Leaf.NotAfter) > 30*24*time.Hour &&
		covers(c.Leaf, hosts) && c.Leaf.CheckSignatureFrom(s.ca) == nil {
		s.cert.Store(&c)
		return nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: hosts[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(leafLife),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, s.ca, &key.PublicKey, s.caKey)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	chain := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), s.caPEM...)
	if err := os.WriteFile(kf, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(lf, chain, 0o644); err != nil {
		return err
	}
	c, err := tls.LoadX509KeyPair(lf, kf)
	if err != nil {
		return err
	}
	s.cert.Store(&c)
	return nil
}

func covers(c *x509.Certificate, hosts []string) bool {
	for _, h := range hosts {
		if c.VerifyHostname(h) != nil {
			return false
		}
	}
	return true
}

// ---- the original self-signed mode

func loadOrCreateSelf(dataDir string, hosts []string) (tls.Certificate, error) {
	cf, kf := filepath.Join(dataDir, "tls-self.crt"), filepath.Join(dataDir, "tls-self.key")
	if c, err := tls.LoadX509KeyPair(cf, kf); err == nil {
		return c, nil
	}
	if len(hosts) == 0 {
		return tls.Certificate{}, errors.New("tlsx: a self-signed certificate needs at least one host name")
	}
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: hosts[0], Organization: []string{"ParallelPath (self-signed)"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(kf, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(cf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(cf, kf)
}

// ServerConfig returns the TLS configuration for a fixed certificate (tests and tools).
func ServerConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: Groups,
		Certificates:     []tls.Certificate{cert},
		NextProtos:       []string{"h2", "http/1.1"},
	}
}

// LoadOrCreate is the original self-signed certificate (kept for tests and tools).
func LoadOrCreate(certFile, keyFile, dataDir string, hosts []string) (tls.Certificate, error) {
	if certFile != "" || keyFile != "" {
		return tls.LoadX509KeyPair(certFile, keyFile)
	}
	return loadOrCreateSelf(dataDir, hosts)
}

// Fingerprint is the SHA-256 of a certificate's DER.
func Fingerprint(c tls.Certificate) []byte {
	if len(c.Certificate) == 0 {
		return nil
	}
	return sum256(c.Certificate[0])
}

// permitted is the configured hosts the local CA's name constraints allow (and records the rest in Uncovered).
func (s *Source) permitted() []string {
	var ok []string
	s.Uncovered = nil
	for _, h := range s.opt.Hosts {
		allowed := false
		if ip := net.ParseIP(h); ip != nil {
			for _, r := range s.ca.PermittedIPRanges {
				allowed = allowed || r.Contains(ip)
			}
		} else {
			for _, d := range s.ca.PermittedDNSDomains {
				allowed = allowed || h == d || strings.HasSuffix(h, "."+d)
			}
		}
		if allowed {
			ok = append(ok, h)
		} else {
			s.Uncovered = append(s.Uncovered, h)
		}
	}
	return ok
}

// ResetCA replaces the local CA (after the server's names change): every device has to trust the new one.
func ResetCA(dataDir string) error {
	for _, f := range []string{"tls-ca.crt", "tls-ca.key.sealed", "tls-leaf.crt", "tls-leaf.key"} {
		if err := os.Remove(filepath.Join(dataDir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
