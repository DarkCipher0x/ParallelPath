package api

import (
	"encoding/pem"
	"fmt"
	"html"
	"net/http"
	"strings"
)

// Trust is what the server tells devices about its certificate.
type Trust struct {
	Mode        string // local-ca | acme | files | self
	CAPEM       []byte // local-ca: the certificate to install
	Fingerprint []byte
	Name        string
}

// WithTrust adds the certificate pages: /trust (how to make this server trusted on each kind of device) and, for
// a local CA, /ca.crt (PEM) and /ca.cer (DER, for Windows' certificate import).
func WithTrust(next http.Handler, t Trust) http.Handler {
	m := http.NewServeMux()
	m.Handle("/", next)
	m.HandleFunc("GET /trust", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(trustPage(t)))
	})
	if len(t.CAPEM) > 0 {
		serve := func(body []byte, typ, name string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", typ)
				w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
				w.Header().Set("Cache-Control", "no-cache")
				_, _ = w.Write(body)
			}
		}
		der := t.CAPEM
		if b, _ := pem.Decode(t.CAPEM); b != nil {
			der = b.Bytes
		}
		file := "ParallelPath-" + safeName(t.Name) + "-CA"
		m.HandleFunc("GET /ca.crt", serve(t.CAPEM, "application/x-x509-ca-cert", file+".crt"))
		m.HandleFunc("GET /ca.cer", serve(der, "application/pkix-cert", file+".cer"))
	}
	return m
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func groupHex(b []byte) string {
	h := fmt.Sprintf("%X", b)
	var parts []string
	for i := 0; i < len(h); i += 4 {
		parts = append(parts, h[i:min(i+4, len(h))])
	}
	return strings.Join(parts, " ")
}

func trustPage(t Trust) string {
	var body string
	switch t.Mode {
	case "local-ca":
		body = `<p>This server uses its own certificate authority, so each device needs to trust it <b>once</b>. After that
browsers open it with no warnings, and the connection uses post-quantum TLS (ML-KEM). The certificate can only vouch for
this server's own address - it can't be used to impersonate any other site.</p>
<p><a class="button" href="/ca.crt">Download the certificate</a> &nbsp; <a href="/ca.cer">(.cer for Windows)</a></p>
<p class="fp">Fingerprint (SHA-256) - check it matches what the server's owner gave you:<br><code>` + groupHex(t.Fingerprint) + `</code></p>
<h2>Windows</h2><ol><li>Open the downloaded <code>.cer</code> file and choose <b>Install Certificate…</b></li>
<li>Choose <b>Local Machine</b> (or Current User), then <b>Place all certificates in the following store</b> → <b>Trusted Root Certification Authorities</b>.</li>
<li>Finish, then restart your browser. (The ParallelPath server setup can do this for you on the server PC.)</li></ol>
<h2>Android</h2><ol><li>Download the <code>.crt</code> file.</li>
<li>Settings → Security and privacy → More security settings → Encryption and credentials → <b>Install a certificate</b> → <b>CA certificate</b>, and choose the file.</li></ol>
<h2>iPhone and iPad</h2><ol><li>Open this page in Safari and download the <code>.crt</code> file; allow the profile download.</li>
<li>Settings → General → VPN and Device Management → install the ParallelPath profile.</li>
<li>Settings → General → About → <b>Certificate Trust Settings</b> → turn on full trust for it.</li></ol>
<h2>Mac</h2><ol><li>Open the downloaded file; Keychain Access adds it to the <b>System</b> keychain.</li>
<li>Double-click it there → Trust → <b>When using this certificate: Always Trust</b>.</li></ol>
<h2>Linux (Ubuntu)</h2><p><code>sudo cp ParallelPath-*-CA.crt /usr/local/share/ca-certificates/ &amp;&amp; sudo update-ca-certificates</code>
(Firefox: Settings → Privacy &amp; Security → Certificates → View Certificates → Authorities → Import.)</p>
<p>The ParallelPath apps for Windows and Android don't need this: they check this server's certificate themselves.</p>`
	case "acme", "files":
		body = `<p>This server has a certificate your device already trusts, so there's nothing to install. The connection uses
post-quantum TLS (ML-KEM).</p>`
	default:
		body = `<p>This server uses a self-signed certificate, so browsers show a warning. The ParallelPath apps for Windows and
Android check the certificate's fingerprint themselves. To use ParallelPath in a browser without warnings, the
server's owner can switch it to its own certificate authority (<code>tls_mode: local-ca</code>) or a Let's Encrypt
certificate (<code>tls_mode: acme</code>) - see the admin portal.</p>
<p class="fp">Fingerprint (SHA-256):<br><code>` + groupHex(t.Fingerprint) + `</code></p>`
	}
	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Trust this ParallelPath server</title><link rel="stylesheet" href="/style.css"></head><body><main>
<span class="badge">Post-quantum protected</span>
<h1>Trust <span>` + html.EscapeString(t.Name) + `</span></h1>` + body + `
<p><a href="/">Back</a></p></main></body></html>`
}
