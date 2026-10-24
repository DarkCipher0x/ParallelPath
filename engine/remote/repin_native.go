//go:build !js

package remote

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// IsPinChanged reports whether a request failed because the server's certificate no longer matches the pin.
func IsPinChanged(err error) bool { return errors.Is(err, errPin) }

// Hello is the server's public self-description, fetched while the TLS pin doesn't match: TLSPin is the server's
// signed statement of its new pin, which the caller checks against the server key it pinned at setup.
type Hello struct {
	ServerKey []byte          `json:"server_key"`
	TLSPin    json.RawMessage `json:"tls_pin"`
}

// FetchHello connects without the pin (post-quantum TLS still required) and returns the pin the server presented
// now plus its hello. Nothing is sent but a GET of a public endpoint: no session token, no data.
func FetchHello(ctx context.Context, server string) ([]byte, *Hello, error) {
	u, err := parseBase(server)
	if err != nil {
		return nil, nil, err
	}
	var seen []byte
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: Groups, ServerName: u.Hostname(), InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no server certificate")
			}
			seen = pinOf(cs.PeerCertificates)
			return nil
		}}
	hc := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg, DisableKeepAlives: true}}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String()+"/api/v1/hello", nil)
	if err != nil {
		return nil, nil, err
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	var h Hello
	if res.StatusCode != http.StatusOK || json.NewDecoder(http.MaxBytesReader(nil, res.Body, 1<<20)).Decode(&h) != nil {
		return nil, nil, errors.New("the server didn't answer")
	}
	return seen, &h, nil
}
