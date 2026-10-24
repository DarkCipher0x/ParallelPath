//go:build !js

package remote

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
)

// ProbeServer connects once without trusting anything, to show the person what they're connecting to.
func ProbeServer(server string) (*Probe, error) {
	u, err := parseBase(server)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "http" { // a development listener on this machine (see parseBase)
		return &Probe{URL: u.String(), TrustedByCA: true, KeyExchange: "none - development listener on this machine"}, nil
	}
	host := u.Host
	if u.Port() == "" {
		host += ":443"
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: Groups, ServerName: u.Hostname(), InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", host, cfg)
	if err != nil {
		return nil, fmt.Errorf("can't reach the server with post-quantum TLS: %w", err)
	}
	defer conn.Close()
	cs := conn.ConnectionState()
	leaf := cs.PeerCertificates[0]
	p := &Probe{URL: u.String(), CertSHA256: pinOf(cs.PeerCertificates), Subject: leaf.Subject.CommonName, KeyExchange: cs.CurveID.String()}
	inter := x509.NewCertPool()
	for _, c := range cs.PeerCertificates[1:] {
		inter.AddCert(c)
	}
	_, verr := leaf.Verify(x509.VerifyOptions{DNSName: u.Hostname(), Intermediates: inter})
	p.TrustedByCA = verr == nil
	return p, nil
}
