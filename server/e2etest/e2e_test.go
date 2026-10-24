// Package e2etest runs real client engines against a real server (in-process, over post-quantum TLS on
// loopback), exercising the whole system the way the apps use it.
package e2etest

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pp/core/pqc"
	"pp/engine/app"
	"pp/server/internal/api"
	"pp/server/internal/seal"
	"pp/server/internal/store"
	"pp/server/internal/svc"
	"pp/server/internal/tlsx"
)

// servers finds a test server by its URL (see serverOf).
var servers sync.Map

type testServer struct {
	url   string
	svc   *svc.Service
	dir   string
	cert  atomic.Pointer[tls.Certificate]
	mu    sync.Mutex
	conns map[net.Conn]bool
}

func startServer(t *testing.T) *testServer {
	dir := t.TempDir()
	k, err := seal.Derive(pqc.NewKey())
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "pp.db"), k.Data)
	if err != nil {
		t.Fatal(err)
	}
	hub := api.NewHub()
	s := svc.New(db, k.Signing, k.Session, hub, svc.DefaultConfig)
	cert, err := tlsx.LoadOrCreate("", "", dir, []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0") // loopback only (no firewall prompts)
	if err != nil {
		t.Fatal(err)
	}
	ts := &testServer{url: "https://" + ln.Addr().String(), svc: s, dir: dir}
	ts.cert.Store(&cert)
	servers.Store(ts.url, ts)
	cfg := tlsx.ServerConfig(cert)
	cfg.Certificates = nil
	cfg.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return ts.cert.Load(), nil }
	s.SetTLSPin(func() []byte { return tlsx.Fingerprint(*ts.cert.Load()) })
	srv := &http.Server{Handler: api.New(s, hub, dir, "Test family server", slog.Default()), TLSConfig: cfg,
		ConnState: func(c net.Conn, st http.ConnState) {
			ts.mu.Lock()
			defer ts.mu.Unlock()
			if ts.conns == nil {
				ts.conns = map[net.Conn]bool{}
			}
			if st == http.StateClosed || st == http.StateHijacked {
				delete(ts.conns, c)
			} else {
				ts.conns[c] = true
			}
		}}
	go srv.ServeTLS(ln, "", "")
	t.Cleanup(func() { srv.Close(); db.Close() })
	return ts
}

// newCertificate makes the server switch to a brand-new self-signed certificate (like an operator moving it to
// new certificate files): apps must follow it only through the server's signed statement.
func (ts *testServer) newCertificate(t *testing.T) {
	t.Helper()
	cert, err := tlsx.LoadOrCreate("", "", t.TempDir(), []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	ts.cert.Store(&cert)
	// A real change comes with a restart: every open connection drops, so the apps must do a fresh handshake.
	ts.mu.Lock()
	for c := range ts.conns {
		c.Close()
	}
	ts.mu.Unlock()
}

func call[T any](t *testing.T, e *app.Engine, method string, args string) T {
	t.Helper()
	res, err := e.Call(method, []byte(args))
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return res.(T)
}

// waitFor polls until cond is true (engines sync in the background on server events).
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func setup(t *testing.T, ts *testServer, invite, name, kind string) (*app.Engine, *app.SetupResult) {
	return setupWith(t, ts, invite, name, kind, nil)
}

func setupWith(t *testing.T, ts *testServer, invite, name, kind string, l app.Listener) (*app.Engine, *app.SetupResult) {
	e := app.New(t.TempDir(), l)
	p, err := e.Preview(ts.url)
	if err != nil {
		t.Fatal(err)
	}
	if p.TrustedByCA || len(p.CertSHA256) != 32 || !strings.Contains(p.KeyExchange, "MLKEM") {
		t.Fatalf("preview: %+v", p)
	}
	res, err := e.Setup(app.SetupRequest{Server: ts.url, CertSHA256: p.CertSHA256, Invite: invite, Passphrase: "correct horse battery",
		YourName: name, DeviceName: name + "'s device", DeviceKind: kind})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Lock)
	serverURL.Store(e, ts.url)
	return e, res
}

// serverURL remembers which test server each engine was set up with.
var serverURL sync.Map

func serverOf(t *testing.T, e *app.Engine) string {
	u, ok := serverURL.Load(e)
	if !ok {
		t.Fatal("no server recorded for that engine")
	}
	return u.(string)
}

func samplePNG(t *testing.T, dir string) string {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 180, 255})
		}
	}
	p := filepath.Join(dir, "beach day.png")
	f, _ := os.Create(p)
	png.Encode(f, img)
	f.Close()
	return p
}

func TestTwoParentsEndToEnd(t *testing.T) {
	ts := startServer(t)
	inv, _, err := ts.svc.CreateServerInvite()
	if err != nil {
		t.Fatal(err)
	}

	// Alice starts a family from the server invite.
	alice, ares := setup(t, ts, inv, "Alice", "windows")
	space := ares.SpaceID
	if space == "" || ares.Role != "parent" {
		t.Fatalf("alice setup: %+v", ares)
	}
	waitFor(t, "family name posted", func() bool {
		s, _ := alice.Spaces()
		return len(s) == 1 && s[0].Name == "Smith-Jones family" || len(s) == 1 && s[0].Name == "Alice's family"
	})

	// She invites Bob as the other parent.
	invRes := call[map[string]any](t, alice, "invite", `{"space":"`+space+`","role":"parent"}`)
	code := invRes["code"].(string)
	if !strings.HasPrefix(code, "J.") {
		t.Fatalf("code %q", code)
	}
	bob, bres := setup(t, ts, code, "Bob", "android")
	if !bres.Waiting || bres.SpaceID != space {
		t.Fatalf("bob setup: %+v", bres)
	}

	// Both see the same safety number; Alice approves Bob.
	waitFor(t, "alice sees bob waiting", func() bool {
		alice.SyncAll()
		s, _ := alice.Spaces()
		for _, m := range s[0].Members {
			if m.Person == bres.Person && m.NeedsKeys {
				return true
			}
		}
		return false
	})
	sa := call[map[string]string](t, alice, "safety_number", `{"person":"`+bres.Person+`"}`)["number"]
	sb := call[map[string]string](t, bob, "safety_number", `{"person":"`+ares.Person+`"}`)["number"]
	if sa != sb || len(sa) != 71 {
		t.Fatalf("safety numbers differ: %q vs %q", sa, sb)
	}
	call[map[string]bool](t, alice, "approve", `{"space":"`+space+`","person":"`+bres.Person+`"}`)

	// Bob can now post: a message with a photo.
	waitFor(t, "bob has keys", func() bool {
		bob.SyncAll()
		s, _ := bob.Spaces()
		return len(s) == 1 && !s[0].Waiting
	})
	photo := samplePNG(t, t.TempDir())
	if err := bob.Send(app.SendRequest{Space: space, Text: "Here's the photo from Saturday 🏖️", Files: []app.FileInput{{Path: photo}}}); err != nil {
		t.Fatal(err)
	}

	// Alice sees it (names resolved, thumbnail present), reads it and reacts.
	var msg app.Message
	waitFor(t, "alice receives bob's message", func() bool {
		ms, _ := alice.Messages(space)
		for _, m := range ms {
			if m.AuthorName == "Bob" && len(m.Attachments) == 1 {
				msg = m
				return true
			}
		}
		return false
	})
	if msg.Text != "Here's the photo from Saturday 🏖️" || len(msg.Attachments[0].Thumb) == 0 || msg.Attachments[0].Width != 800 {
		t.Fatalf("message: %+v", msg)
	}
	call[map[string]bool](t, alice, "mark_read", `{"space":"`+space+`","item":"`+msg.ID+`"}`)
	call[map[string]bool](t, alice, "react", `{"space":"`+space+`","item":"`+msg.ID+`","emoji":"❤️"}`)

	// Alice opens the photo: downloaded, decrypted and integrity-checked.
	got := call[map[string]string](t, alice, "open_attachment", `{"space":"`+space+`","item":"`+msg.ID+`","blob":"`+msg.Attachments[0].BlobID+`"}`)["path"]
	want, _ := os.ReadFile(photo)
	have, _ := os.ReadFile(got)
	if !bytes.Equal(want, have) {
		t.Fatal("decrypted photo differs")
	}

	// Bob sees the reaction and the read receipt.
	waitFor(t, "bob sees reaction and receipt", func() bool {
		bob.SyncAll()
		ms, _ := bob.Messages(space)
		for _, m := range ms {
			if m.ID == msg.ID && len(m.Reactions) == 1 && m.Reactions[0].Emoji == "❤️" && len(m.ReadBy) == 1 && m.ReadBy[0].Name == "Alice" {
				return true
			}
		}
		return false
	})

	// Edits need a reason; history keeps every version.
	if _, err := bob.Call("edit", []byte(`{"space":"`+space+`","item":"`+msg.ID+`","text":"Photo from Sunday","reason":""}`)); err == nil {
		t.Fatal("edit without a reason accepted")
	}
	call[map[string]bool](t, bob, "edit", `{"space":"`+space+`","item":"`+msg.ID+`","text":"Here's the photo from Sunday","reason":"wrong day"}`)
	if _, err := alice.Call("edit", []byte(`{"space":"`+space+`","item":"`+msg.ID+`","text":"x","reason":"y"}`)); err == nil {
		t.Fatal("alice edited bob's message")
	}
	waitFor(t, "alice sees the edit with its reason", func() bool {
		alice.SyncAll()
		ms, _ := alice.Messages(space)
		for _, m := range ms {
			if m.ID == msg.ID && m.Edited && m.Reason == "wrong day" && len(m.History) == 2 && m.History[0].Text == "Here's the photo from Saturday 🏖️" {
				return true
			}
		}
		return false
	})

	// Retraction: content withdrawn, history kept.
	call[map[string]bool](t, bob, "retract", `{"space":"`+space+`","item":"`+msg.ID+`","reason":"sent by mistake"}`)
	waitFor(t, "alice sees the retraction", func() bool {
		alice.SyncAll()
		ms, _ := alice.Messages(space)
		for _, m := range ms {
			if m.ID == msg.ID && m.Retracted && len(m.History) == 3 && len(m.Attachments) == 0 {
				return true
			}
		}
		return false
	})

	// Both audit logs verify, and record everything.
	for name, e := range map[string]*app.Engine{"alice": alice, "bob": bob} {
		e.SyncAll()
		log, err := e.Audit(space)
		if err != nil || !log.Verified || log.Problem != "" {
			t.Fatalf("%s audit: %+v %v", name, log, err)
		}
		actions := map[string]int{}
		for _, r := range log.Rows {
			actions[r.Action]++
		}
		for _, a := range []string{"space.create", "member.invite", "member.join", "keys.grant", "item.create", "item.revise", "item.view", "blob.upload", "blob.download"} {
			if actions[a] == 0 {
				t.Errorf("%s audit lacks %s: %v", name, a, actions)
			}
		}
	}

	// Locking wipes keys; the passphrase unlocks again.
	alice.Lock()
	if _, err := alice.Messages(space); err == nil {
		t.Fatal("messages readable while locked")
	}
	if _, err := alice.Call("unlock", []byte(`{"passphrase":"wrong passphrase"}`)); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	call[map[string]bool](t, alice, "unlock", `{"passphrase":"correct horse battery"}`)
	if ms, err := alice.Messages(space); err != nil || len(ms) == 0 {
		t.Fatal("messages after unlock")
	}
	_ = rand.Reader
}
