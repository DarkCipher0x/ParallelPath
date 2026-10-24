package e2etest

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"pp/core/pqc"
	"pp/engine/app"
	"pp/engine/remote"
	"pp/server/internal/api"
	"pp/server/internal/seal"
	"pp/server/internal/store"
	"pp/server/internal/svc"
	"pp/server/internal/tlsx"
)

// TestLocalCAPinning: with tls_mode local-ca the apps pin the server's own CA, so a renewed server certificate is
// accepted without anyone doing anything, while a different CA is refused.
func TestLocalCAPinning(t *testing.T) {
	dir := t.TempDir()
	k, _ := seal.Derive(pqc.NewKey())
	db, err := store.Open(filepath.Join(dir, "pp.db"), k.Data)
	if err != nil {
		t.Fatal(err)
	}
	opt := tlsx.Options{Mode: tlsx.ModeLocalCA, DataDir: dir, Name: "CA test", Hosts: []string{"127.0.0.1"}, KEK: k.TLS}
	certs, err := tlsx.Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	hub := api.NewHub()
	s := svc.New(db, k.Signing, k.Session, hub, svc.DefaultConfig)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	var cur atomic.Pointer[tlsx.Source] // ServeTLS copies the TLS config, so swap certificates through this
	cur.Store(certs)
	cfg := certs.Config()
	cfg.GetCertificate = func(h *tls.ClientHelloInfo) (*tls.Certificate, error) { return cur.Load().Config().GetCertificate(h) }
	srv := &http.Server{Handler: api.New(s, hub, dir, "CA test", slog.Default()), TLSConfig: cfg}
	go srv.ServeTLS(ln, "", "")
	t.Cleanup(func() { srv.Close(); db.Close() })
	url := "https://" + ln.Addr().String()

	e := app.New(t.TempDir(), nil)
	p, err := e.Preview(url)
	if err != nil {
		t.Fatal(err)
	}
	if p.TrustedByCA || string(p.CertSHA256) != string(certs.Fingerprint()) {
		t.Fatal("the app should be offered the CA's fingerprint to pin")
	}
	inv, _, _ := s.CreateServerInvite()
	if _, err := e.Setup(app.SetupRequest{Server: url, CertSHA256: p.CertSHA256, Invite: inv, Passphrase: "correct horse battery",
		YourName: "Pat", DeviceName: "laptop", DeviceKind: "windows", FamilyName: "Test"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Lock)

	// The server certificate is renewed (new key, same CA): the pinned app keeps working.
	os.Remove(filepath.Join(dir, "tls-leaf.crt"))
	os.Remove(filepath.Join(dir, "tls-leaf.key"))
	renewed, err := tlsx.Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	cur.Store(renewed)
	if string(renewed.Fingerprint()) != string(certs.Fingerprint()) {
		t.Fatal("renewal changed the CA")
	}
	if _, err := e.Call("account", nil); err != nil {
		t.Fatalf("after renewal: %v", err)
	}

	// A different CA (someone else's) is refused.
	if err := tlsx.ResetCA(dir); err != nil {
		t.Fatal(err)
	}
	other, err := tlsx.Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	cur.Store(other)
	c, err := remote.New(url, p.CertSHA256)
	if err != nil {
		t.Fatal(err)
	}
	err = c.Call(context.Background(), "GET", "/api/v1/hello", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "certificate has changed") {
		t.Fatalf("a server with a different CA was accepted under the old pin: %v", err)
	}
}
