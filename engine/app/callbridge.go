//go:build !js

package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pp/engine/callui"
)

// callBridge serves the call page to a browser window on this device (an Edge app window on Windows, a WebView on
// Android): the page's files, a JSON call interface limited to the call methods, and a stream of call events. It
// listens on the loopback address only, and every request needs the random token in the page's address.
type callBridge struct {
	url   string
	token string
	srv   *http.Server
}

// CallBridge starts (once) the local call bridge and returns the call page's address (add #space=...&call=...).
func (e *Engine) CallBridge() (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.bridge != nil {
		return e.bridge.url, nil
	}
	// The same port each time when it's free, so the call window's browser remembers the person's camera and
	// microphone choice for this address (permissions are kept per origin, which includes the port).
	portFile := filepath.Join(e.dir, "call-port")
	var ln net.Listener
	var err error
	if b, rerr := os.ReadFile(portFile); rerr == nil {
		if p, perr := strconv.Atoi(strings.TrimSpace(string(b))); perr == nil && p > 1024 && p < 65536 {
			ln, _ = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		}
	}
	if ln == nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return "", err
		}
		_ = os.WriteFile(portFile, []byte(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)), 0o600)
	}
	t := make([]byte, 32)
	_, _ = rand.Read(t)
	b := &callBridge{token: hex.EncodeToString(t)}
	b.url = fmt.Sprintf("http://%s/call/?t=%s", ln.Addr().String(), b.token)
	m := http.NewServeMux()
	m.Handle("GET /call/", b.guard(http.StripPrefix("/call/", http.FileServerFS(callui.Files())), true))
	m.Handle("POST /call/api/rpc", b.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Args   json.RawMessage `json:"args"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&req) != nil || !callMethods[req.Method] {
			http.Error(w, "not allowed", http.StatusForbidden)
			return
		}
		res, err := callDispatch(e, req.Method, req.Args)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": res})
	}), false))
	m.Handle("GET /call/api/events", b.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		ch := e.subscribeCalls()
		defer e.unsubscribeCalls(ch)
		_, _ = fmt.Fprint(w, ": ready\n\n")
		fl.Flush()
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case ev := <-ch:
				j, _ := json.Marshal(ev)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", j)
				fl.Flush()
			case <-ping.C:
				_, _ = fmt.Fprint(w, ": ping\n\n")
				fl.Flush()
			}
		}
	}), true))
	b.srv = &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = b.srv.Serve(ln) }()
	e.bridge = b
	return b.url, nil
}

// guard checks the token (header, or query for the page and its event stream) and the page's security headers.
func (b *callBridge) guard(h http.Handler, queryOK bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-PP-Token")
		if tok == "" && queryOK {
			tok = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(b.token)) != 1 {
			// The page's own files (call.js...) are fetched without the query; they hold nothing secret.
			if !(queryOK && r.Method == http.MethodGet && r.URL.Path != "/call/" && r.URL.Path != "/call/api/events") {
				http.Error(w, "not allowed", http.StatusForbidden)
				return
			}
		}
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; "+
			"media-src 'self' blob:; connect-src 'self'; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Cache-Control", "no-store")
		hd.Set("X-Content-Type-Options", "nosniff")
		h.ServeHTTP(w, r)
	})
}

func (b *callBridge) close() { _ = b.srv.Close() }

// callDispatch is Engine.Call (set in init: the RPC table itself refers to CallBridge).
var callDispatch func(*Engine, string, json.RawMessage) (any, error)

func init() { callDispatch = (*Engine).Call }
