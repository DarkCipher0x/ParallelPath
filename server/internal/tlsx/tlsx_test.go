package tlsx

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"

	"pp/core/pqc"
)

func TestLocalCA(t *testing.T) {
	dir := t.TempDir()
	kek := pqc.NewKey()
	opt := Options{Mode: ModeLocalCA, DataDir: dir, Name: "Test family", Hosts: []string{"family.home.arpa", "192.168.1.20"}, KEK: kek}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	leaf := s.cert.Load().Leaf
	roots := x509.NewCertPool()
	roots.AddCert(s.CACert())
	for _, h := range opt.Hosts {
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: h, Roots: roots}); err != nil {
			t.Fatalf("leaf for %s: %v", h, err)
		}
	}
	if !s.CACert().IsCA || len(s.CACert().PermittedDNSDomains) != 1 || len(s.CACert().PermittedIPRanges) != 1 {
		t.Fatalf("CA constraints: %+v %+v", s.CACert().PermittedDNSDomains, s.CACert().PermittedIPRanges)
	}
	if len(s.cert.Load().Certificate) != 2 {
		t.Fatal("the server must send its CA with the certificate (the apps pin the CA)")
	}

	// The CA can't vouch for anyone else, even if its key were stolen.
	other := *leaf
	other.DNSNames, other.IPAddresses = []string{"bank.example.com"}, []net.IP{net.ParseIP("10.0.0.1")}
	der, err := x509.CreateCertificate(nil, &other, s.ca, leaf.PublicKey, s.caKey)
	if err != nil {
		t.Fatal(err)
	}
	evil, _ := x509.ParseCertificate(der)
	if _, err := evil.Verify(x509.VerifyOptions{DNSName: "bank.example.com", Roots: roots}); err == nil {
		t.Fatal("the local CA vouched for another site")
	}

	// Re-opening uses the same CA (its key is sealed with the KEK) and keeps the current server certificate.
	s2, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	if string(s2.Fingerprint()) != string(s.Fingerprint()) || s2.cert.Load().Leaf.SerialNumber.Cmp(leaf.SerialNumber) != 0 {
		t.Fatal("reopen changed the CA or the certificate")
	}
	opt.KEK = pqc.NewKey()
	if _, err := Open(opt); err == nil {
		t.Fatal("the CA key opened with the wrong key")
	}

	// A new host name gets a new server certificate from the same CA.
	opt.KEK, opt.Hosts = kek, append(opt.Hosts, "pp.home.arpa")
	s3, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	if s3.cert.Load().Leaf.VerifyHostname("pp.home.arpa") == nil || len(s3.Uncovered) != 1 || s3.Uncovered[0] != "pp.home.arpa" {
		t.Fatal("a name outside the CA's constraints must be reported, not put in a certificate browsers would reject")
	}
	if err := ResetCA(dir); err != nil {
		t.Fatal(err)
	}
	s4, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	if s4.cert.Load().Leaf.VerifyHostname("pp.home.arpa") != nil || len(s4.Uncovered) != 0 || string(s4.Fingerprint()) == string(s.Fingerprint()) {
		t.Fatal("a new CA should cover every name")
	}
}

func TestPostQuantumOnly(t *testing.T) {
	s, err := Open(Options{Mode: ModeLocalCA, DataDir: t.TempDir(), Name: "x", Hosts: []string{"127.0.0.1"}, KEK: pqc.NewKey()})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", s.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.(*tls.Conn).Handshake()
			c.Close()
		}
	}()
	roots := x509.NewCertPool()
	roots.AddCert(s.CACert())
	classical := &tls.Config{RootCAs: roots, CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256}}
	if c, err := tls.Dial("tcp", ln.Addr().String(), classical); err == nil {
		c.Close()
		t.Fatal("a classical-only client connected")
	}
	pq := &tls.Config{RootCAs: roots, CurvePreferences: []tls.CurveID{tls.X25519MLKEM768}}
	c, err := tls.Dial("tcp", ln.Addr().String(), pq)
	if err != nil {
		t.Fatalf("a browser-style ML-KEM client (trusting the local CA) couldn't connect: %v", err)
	}
	if c.ConnectionState().CurveID != tls.X25519MLKEM768 {
		t.Fatal("not post-quantum")
	}
	c.Close()
}
