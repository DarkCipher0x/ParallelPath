package e2etest

import (
	"crypto/rand"
	"strings"
	"testing"

	"pp/engine/app"
)

func testServerOf(t *testing.T, e *app.Engine) *testServer {
	t.Helper()
	ts, ok := servers.Load(serverOf(t, e))
	if !ok {
		t.Fatal("no test server")
	}
	return ts.(*testServer)
}

// TestCertificateChange: when the server moves to a new certificate, the apps follow it on their own - but only
// because the server signs the change with the key they pinned at setup. A certificate without that signed
// notice (what an attacker in the middle would present) is refused.
func TestCertificateChange(t *testing.T) {
	alice, bob, space, _, _ := family(t)
	ts := testServerOf(t, alice)

	ts.newCertificate(t)
	if err := alice.Send(app.SendRequest{Space: space, Text: "After the new certificate"}); err != nil {
		t.Fatalf("alice didn't follow a signed certificate change: %v", err)
	}
	waitFor(t, "bob follows it too", func() bool { return texts(t, bob, space)["After the new certificate"] })

	// Now the "server" presents a new certificate but signs a different pin (it can't sign for a certificate it
	// isn't presenting): the apps must keep refusing.
	bogus := make([]byte, 32)
	_, _ = rand.Read(bogus)
	ts.svc.SetTLSPin(func() []byte { return bogus })
	ts.newCertificate(t)
	err := alice.Send(app.SendRequest{Space: space, Text: "should not go"})
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("an unproven certificate change was accepted: %v", err)
	}
	// And with no signed notice at all.
	ts.svc.SetTLSPin(func() []byte { return nil })
	if err := alice.Send(app.SendRequest{Space: space, Text: "should not go either"}); err == nil {
		t.Fatal("a certificate change without a notice was accepted")
	}
}
