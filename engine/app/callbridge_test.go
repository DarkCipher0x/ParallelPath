//go:build !js

package app

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// The call bridge serves the call page only to holders of its token, and lets the page use the call methods only.
func TestCallBridge(t *testing.T) {
	e := New(t.TempDir(), nil)
	raw, err := e.CallBridge()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Lock()
	u, _ := url.Parse(raw)
	if u.Hostname() != "127.0.0.1" {
		t.Fatal("the bridge must listen on loopback only")
	}
	tok := u.Query().Get("t")
	base := u.Scheme + "://" + u.Host
	get := func(path string) (int, string) {
		res, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, body := get("/call/?t=" + tok); code != 200 || !strings.Contains(body, "call.js") {
		t.Fatalf("page: %d", code)
	}
	if code, _ := get("/call/?t=wrong"); code != 403 {
		t.Fatalf("page with a wrong token: %d", code)
	}
	if code, _ := get("/call/frames.js"); code != 200 {
		t.Fatalf("frames.js: %d", code)
	}
	post := func(token, body string) int {
		req, _ := http.NewRequest("POST", base+"/call/api/rpc", strings.NewReader(body))
		req.Header.Set("X-PP-Token", token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := post(tok, `{"method":"messages","args":{"space":"x"}}`); code != 403 {
		t.Fatalf("a non-call method through the bridge: %d", code)
	}
	if code := post("nope", `{"method":"call_turn","args":{}}`); code != 403 {
		t.Fatalf("rpc without the token: %d", code)
	}
	if code := post(tok, `{"method":"call_turn","args":{}}`); code != 200 {
		t.Fatalf("a call method: %d", code) // (it reports "locked" inside the reply - the transport itself is allowed)
	}
}
