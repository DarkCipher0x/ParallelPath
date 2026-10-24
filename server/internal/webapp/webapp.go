// Package webapp serves the ParallelPath web app: the same app as on Windows and Android, compiled to WebAssembly
// (the UI from clients/apps/web, the encryption engine from engine/cmd/ppweb). It's embedded in ppserver by the
// release build; everything is encrypted in the browser, so this only ever hands out static files.
package webapp

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed all:dist
var dist embed.FS

// CSP for the web app: its own scripts and WebAssembly only, nothing from anywhere else.
const csp = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; img-src 'self' blob: data:; " +
	"media-src 'self' blob:; connect-src 'self'; worker-src 'self' blob:; font-src 'self' data:; manifest-src 'self'; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

var hashed = regexp.MustCompile(`^[0-9a-f]{16,}\.(wasm|js)$`) // content-hashed names never change

type file struct {
	raw, gz []byte
	etag    string
	typ     string
}

var (
	mu    sync.Mutex
	files = map[string]*file{}
)

func load(name string) (*file, bool) {
	mu.Lock()
	defer mu.Unlock()
	if f, ok := files[name]; ok {
		return f, true
	}
	raw, err := fs.ReadFile(dist, "dist/"+name)
	if err != nil {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	f := &file{raw: raw, etag: `"` + hex.EncodeToString(sum[:12]) + `"`, typ: mime.TypeByExtension(path.Ext(name))}
	switch path.Ext(name) {
	case ".wasm":
		f.typ = "application/wasm"
	case ".webmanifest":
		f.typ = "application/manifest+json"
	case ".map":
		f.typ = "application/json"
	}
	if f.typ == "" {
		f.typ = "application/octet-stream"
	}
	if len(raw) > 1024 && !strings.HasPrefix(f.typ, "image/png") {
		var b bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
		_, _ = zw.Write(raw)
		_ = zw.Close()
		f.gz = b.Bytes()
	}
	files[name] = f
	return f, true
}

// Built reports whether the release build put the web app in (the placeholder only has index.html).
func Built() bool { _, err := fs.Stat(dist, "dist/ppweb.wasm"); return err == nil }

// Handler serves the app under prefix (e.g. "/app/").
func Handler(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, prefix)
		if name == "" || strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		if strings.Contains(name, "..") || strings.HasSuffix(name, ".map") {
			http.NotFound(w, r)
			return
		}
		f, ok := load(name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		if strings.HasPrefix(name, "call/") {
			// The call page opens in a frame over the app (same site only).
			h.Set("Content-Security-Policy", strings.Replace(csp, "frame-ancestors 'none'", "frame-ancestors 'self'", 1))
			h.Set("X-Frame-Options", "SAMEORIGIN")
		} else {
			h.Set("Content-Security-Policy", csp)
		}
		h.Set("Permissions-Policy", "camera=(self), microphone=(self), geolocation=(self), display-capture=(self)")
		h.Set("Content-Type", f.typ)
		h.Set("ETag", f.etag)
		h.Set("Vary", "Accept-Encoding")
		if hashed.MatchString(name) {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache") // revalidated with the ETag on every load
		}
		body := f.raw
		if f.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.Set("Content-Encoding", "gzip")
			body = f.gz
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
	})
}
