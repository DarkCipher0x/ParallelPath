//go:build js

package remote

// ProbeServer in the web app: the page was loaded from this server over the browser's own TLS (which verified the
// certificate and negotiated post-quantum key exchange - the server accepts nothing else), so there is nothing to
// pin; requests go through the browser's fetch.
func ProbeServer(server string) (*Probe, error) {
	u, err := parseBase(server)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "http" { // the server's development listener on this same machine
		return &Probe{URL: u.String(), TrustedByCA: true, KeyExchange: "none - development listener on this machine only"}, nil
	}
	return &Probe{URL: u.String(), TrustedByCA: true, KeyExchange: "X25519MLKEM768 (checked by your browser)"}, nil // gitleaks:allow (an algorithm name)
}
