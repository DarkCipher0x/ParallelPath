package portal

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pp/core/pqc"
	"pp/server/internal/config"
	"pp/server/internal/logring"
	"pp/server/internal/seal"
	"pp/server/internal/store"
	"pp/server/internal/svc"
)

type rig struct {
	t      *testing.T
	p      *Portal
	srv    *httptest.Server
	client *http.Client
	csrf   string
	clock  time.Time
}

func newRig(t *testing.T) *rig {
	dir := t.TempDir()
	if err := config.Default("Test server", []string{"pp.example.org"}).Save(dir); err != nil {
		t.Fatal(err)
	}
	k, _ := seal.Derive(pqc.NewKey())
	db, err := store.Open(filepath.Join(dir, "pp.db"), k.Data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &rig{t: t, clock: time.Now()}
	logs := logring.New(10)
	logs.Write([]byte("hello from the server\n"))
	p, err := New(Options{DB: db, Svc: svc.New(db, k.Signing, k.Session, nil, svc.DefaultConfig), DataDir: dir, Logs: logs,
		Now: func() time.Time { return r.clock }})
	if err != nil {
		t.Fatal(err)
	}
	r.p = p
	r.srv = httptest.NewTLSServer(p.Handler())
	t.Cleanup(r.srv.Close)
	r.client = r.newClient()
	return r
}

func (r *rig) newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	// The test server's own certificate (the real server uses post-quantum TLS; the portal doesn't depend on it).
	return &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
}

func (r *rig) do(c *http.Client, method, path string, body any, csrf string) (int, map[string]any) {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, r.srv.URL+"/admin/api/"+path, &buf)
	if csrf != "" {
		req.Header.Set("X-PP-CSRF", csrf)
	}
	res, err := c.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// signIn reads the session's CSRF token after signing in.
func (r *rig) me(c *http.Client) map[string]any {
	_, st := r.do(c, "GET", "state", nil, "")
	m, _ := st["me"].(map[string]any)
	return m
}

func secretOf(t *testing.T, key string) []byte {
	s, err := b32.DecodeString(strings.ReplaceAll(key, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFirstAdminAndSignIn(t *testing.T) {
	r := newRig(t)
	if _, st := r.do(r.client, "GET", "state", nil, ""); st["has_admins"] != false {
		t.Fatal("a fresh server has admins")
	}
	// Nothing works signed out.
	if code, _ := r.do(r.client, "GET", "overview", nil, ""); code != 401 {
		t.Fatalf("overview signed out: %d", code)
	}

	tok, err := r.p.Accounts().NewToken("setup", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	_, begin := r.do(r.client, "POST", "setup/begin", map[string]string{"token": tok}, "")
	secret := secretOf(t, begin["setup_key"].(string))
	if !strings.HasPrefix(begin["uri"].(string), "otpauth://totp/") {
		t.Fatalf("uri: %v", begin["uri"])
	}
	step := r.clock.Unix() / totpStep
	if code, out := r.do(r.client, "POST", "setup/finish", map[string]string{"token": tok, "name": "sam", "password": "short",
		"code": totpAt(secret, step)}, ""); code != 400 || !strings.Contains(out["error"].(string), "12 characters") {
		t.Fatalf("a short passphrase: %d %v", code, out)
	}
	if code, _ := r.do(r.client, "POST", "setup/finish", map[string]string{"token": tok, "name": "sam", "password": "correct horse battery staple",
		"code": "000000"}, ""); code != 400 {
		t.Fatal("a wrong code was accepted")
	}
	if code, out := r.do(r.client, "POST", "setup/finish", map[string]string{"token": tok, "name": "sam", "password": "correct horse battery staple",
		"code": totpAt(secret, step)}, ""); code != 200 {
		t.Fatalf("setup: %d %v", code, out)
	}
	// The link is used up.
	if code, _ := r.do(r.newClient(), "POST", "setup/begin", map[string]string{"token": tok}, ""); code != 400 {
		t.Fatal("a setup link worked twice")
	}

	me := r.me(r.client)
	if me == nil || me["name"] != "sam" {
		t.Fatalf("signed in as %v", me)
	}
	csrf := me["csrf"].(string)
	if code, out := r.do(r.client, "GET", "overview", nil, ""); code != 200 || out["name"] != "Test server" {
		t.Fatalf("overview: %d %v", code, out)
	}
	// Changes need the CSRF token.
	if code, _ := r.do(r.client, "POST", "invites", nil, ""); code != 403 {
		t.Fatal("a change without the CSRF token was accepted")
	}
	if code, out := r.do(r.client, "POST", "invites", nil, csrf); code != 200 || !strings.HasPrefix(out["code"].(string), "S.") {
		t.Fatalf("invite: %d %v", code, out)
	}
	r.do(r.client, "POST", "logout", nil, csrf)
	if r.me(r.client) != nil {
		t.Fatal("still signed in after signing out")
	}

	// Signing in: the code from setup can't be replayed; the next one works.
	c := r.newClient()
	login := func(pass, code string) int {
		n, _ := r.do(c, "POST", "login", map[string]string{"name": "SAM", "password": pass, "code": code}, "")
		return n
	}
	if login("correct horse battery staple", totpAt(secret, step)) != 401 {
		t.Fatal("a used code was accepted again")
	}
	if login("correct horse battery staple", totpAt(secret, step+1)) != 200 {
		t.Fatal("sign-in with the next code failed")
	}

	// Five failures lock the account, even with the right details afterwards.
	c = r.newClient()
	r.clock = r.clock.Add(2 * time.Minute)
	for i := 0; i < MaxFailures; i++ {
		login("wrong passphrase!!", totpAt(secret, r.clock.Unix()/totpStep))
	}
	if login("correct horse battery staple", totpAt(secret, r.clock.Unix()/totpStep)) != 401 {
		t.Fatal("a locked account signed in")
	}
	r.clock = r.clock.Add(LockoutPeriod + time.Minute)
	if login("correct horse battery staple", totpAt(secret, r.clock.Unix()/totpStep)) != 200 {
		t.Fatal("the lockout didn't end")
	}

	// The server chain records it all and still verifies.
	entries, problem, _ := r.p.o.Svc.ServerAudit(100)
	if problem != "" || len(entries) < 5 {
		t.Fatalf("server audit: %d entries, %s", len(entries), problem)
	}
}

func TestLocalLinkAndNetworks(t *testing.T) {
	r := newRig(t)
	tok, _ := r.p.Accounts().NewToken("local", "", "test")
	// Local links work from this machine only.
	req := httptest.NewRequest("POST", "/admin/api/local-login", strings.NewReader(`{"token":"`+tok+`"}`))
	req.RemoteAddr = "192.0.2.10:4000"
	w := httptest.NewRecorder()
	r.p.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("a local link from elsewhere: %d", w.Code)
	}
	if code, _ := r.do(r.client, "POST", "local-login", map[string]string{"token": tok}, ""); code != 200 {
		t.Fatal("local sign-in failed")
	}
	if code, _ := r.do(r.newClient(), "POST", "local-login", map[string]string{"token": tok}, ""); code != 400 {
		t.Fatal("a local link worked twice")
	}
	me := r.me(r.client)
	if me == nil || me["local"] != true {
		t.Fatalf("local session: %v", me)
	}
	csrf := me["csrf"].(string)

	// How many families someone may start: only through here (the service tests cover the rule itself).
	if code, _ := r.do(r.client, "POST", "people/nobody/families", map[string]int{"left": 1}, csrf); code != 404 {
		t.Fatalf("families for an unknown account: %d", code)
	}
	if code, _ := r.do(r.client, "POST", "people/nobody/families", map[string]string{"left": "lots"}, csrf); code != 400 {
		t.Fatalf("a bad families request: %d", code)
	}
	if code, _ := r.do(r.client, "POST", "people/nobody/families", map[string]int{"left": 1}, ""); code != 403 {
		t.Fatal("a families change without the CSRF token")
	}

	// Empty lists are [] (the page reads .length), never null.
	for _, path := range []string{"admins", "people", "spaces", "invites"} {
		res, _ := r.client.Get(r.srv.URL + "/admin/api/" + path)
		var raw json.RawMessage
		json.NewDecoder(res.Body).Decode(&raw)
		res.Body.Close()
		if string(raw) != "[]" {
			t.Errorf("%s: %s", path, raw)
		}
	}

	// Limit the portal to a network: others don't even see it; this machine still does.
	if code, out := r.do(r.client, "PUT", "settings", map[string]any{"name": "Test server", "max_blob_mb": 2048, "space_quota_gb": 50,
		"backup_keep": 14, "admin_allow": []string{"10.1.2.0/24", "203.0.113.7"}}, csrf); code != 200 {
		t.Fatalf("settings: %d %v", code, out)
	}
	for addr, want := range map[string]int{"10.1.2.99:5000": 200, "203.0.113.7:5000": 200, "192.0.2.10:5000": 404} {
		req := httptest.NewRequest("GET", "/admin/api/state", nil)
		req.RemoteAddr = addr
		w := httptest.NewRecorder()
		r.p.Handler().ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("%s: %d, want %d", addr, w.Code, want)
		}
	}
	if code, _ := r.do(r.client, "GET", "state", nil, ""); code != 200 {
		t.Fatal("this machine was locked out")
	}
	c, _ := config.Load(r.p.o.DataDir)
	if len(c.AdminAllow) != 2 || c.AdminAllow[1] != "203.0.113.7/32" {
		t.Fatalf("saved: %v", c.AdminAllow)
	}
	if code, _ := r.do(r.client, "PUT", "settings", map[string]any{"name": "Test server", "max_blob_mb": 2048, "space_quota_gb": 50,
		"backup_keep": 14, "admin_allow": []string{"not a network"}}, csrf); code != 400 {
		t.Fatal("a bad network was accepted")
	}

	// The portal's pages carry the strict headers, and the logs page shows the server's lines.
	res, _ := r.client.Get(r.srv.URL + "/admin/")
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP: %q", csp)
	}
	res.Body.Close()
	req2, _ := http.NewRequest("GET", r.srv.URL+"/admin/api/logs", nil)
	res, _ = r.client.Do(req2)
	var lines []string
	json.NewDecoder(res.Body).Decode(&lines)
	res.Body.Close()
	if len(lines) != 1 || lines[0] != "hello from the server" {
		t.Fatalf("logs: %v", lines)
	}
}

func TestTOTP(t *testing.T) {
	// RFC 6238 test vector (SHA-1, 8 digits there; the last six digits are what a 6-digit app shows).
	secret := []byte("12345678901234567890")
	if got := totpAt(secret, 59/30); got != "287082" {
		t.Fatalf("got %s", got)
	}
	if got := totpAt(secret, 1111111109/30); got != "081804" {
		t.Fatalf("got %s", got)
	}
	now := time.Unix(1111111109, 0)
	if _, ok := checkTOTP(secret, "081 804", now, 0); !ok {
		t.Fatal("a code with a space")
	}
	if _, ok := checkTOTP(secret, "081804", now, 1111111109/30); ok {
		t.Fatal("a replayed step")
	}
	if _, ok := checkTOTP(secret, "000000", now, 0); ok {
		t.Fatal("a wrong code")
	}
}
