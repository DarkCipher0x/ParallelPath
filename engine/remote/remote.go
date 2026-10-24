// Package remote talks to a ParallelPath server: JSON calls, encrypted file transfer and the live event stream.
//
// Transport policy is the same as the server's: TLS 1.3 with hybrid post-quantum key exchange only. A server with
// a CA-issued certificate is verified normally; a self-hosted server's own certificate is pinned (its SHA-256 is
// shown to the person on first connect and must match on every connection after).
package remote

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Groups is the post-quantum TLS policy (identical to the server's).
var Groups = []tls.CurveID{tls.SecP384r1MLKEM1024, tls.X25519MLKEM768}

// Client is a connection to one server.
type Client struct {
	base  *url.URL
	hc    *http.Client
	token string
}

// Error is an error the server returned (its message is safe to show).
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// IsStatus reports whether err is a server error with the given HTTP status.
func IsStatus(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

func tlsConfig(host string, pin []byte) *tls.Config {
	c := &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: Groups, ServerName: host}
	if len(pin) > 0 {
		c.InsecureSkipVerify = true // replaced by the pin check below
		c.VerifyConnection = func(cs tls.ConnectionState) error { return checkPin(cs, host, pin) }
	}
	return c
}

func parseBase(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname()))) {
		return nil, errors.New("the server address must be an https:// address")
	}
	u.Path, u.RawQuery, u.Fragment = "", "", ""
	return u, nil
}

// New connects to a server. pin is the certificate SHA-256 for self-hosted certificates (nil = CA-verified).
func New(server string, pin []byte) (*Client, error) {
	u, err := parseBase(server)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		TLSClientConfig:       tlsConfig(u.Hostname(), pin),
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	return &Client{base: u, hc: &http.Client{Transport: tr}}, nil
}

// Probe is what a first connection learns: whether the certificate is CA-trusted, and its fingerprint.
type Probe struct {
	URL         string `json:"url"`
	CertSHA256  []byte `json:"cert_sha256"`
	TrustedByCA bool   `json:"trusted_by_ca"`
	Subject     string `json:"subject"`
	KeyExchange string `json:"key_exchange"`
}

// SetToken sets the session token used for signed-in calls.
func (c *Client) SetToken(t string) { c.token = t }

// Call does a JSON request. in may be nil; out may be nil.
func (c *Client) Call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base.String()+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach the server: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return &Error{Status: res.StatusCode, Message: e.Error}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(out)
}

func intentHeader(intent any) (string, error) {
	b, err := json.Marshal(intent)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// PutBlob uploads an encrypted file.
func (c *Client) PutBlob(ctx context.Context, space, blob string, body io.Reader, size int64, intent any) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", c.base.String()+"/api/v1/spaces/"+space+"/blobs/"+blob, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	h, err := intentHeader(intent)
	if err != nil {
		return err
	}
	req.Header.Set("X-PP-Intent", h)
	req.Header.Set("Content-Type", "application/octet-stream")
	return c.do(req, nil)
}

// GetBlob downloads an encrypted file into w.
func (c *Client) GetBlob(ctx context.Context, blob string, intent any, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base.String()+"/api/v1/blobs/"+blob, nil)
	if err != nil {
		return err
	}
	h, err := intentHeader(intent)
	if err != nil {
		return err
	}
	req.Header.Set("X-PP-Intent", h)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach the server: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &Error{Status: res.StatusCode, Message: res.Status}
	}
	_, err = io.Copy(w, res.Body)
	return err
}

// Event is a change notification from the server.
type Event struct {
	SpaceID string  `json:"space_id"`
	Kind    string  `json:"kind"`
	Seq     uint64  `json:"seq"`
	Signal  *Signal `json:"signal,omitempty"` // kind "signal": an end-to-end encrypted call-setup message
}

// Signal is a relayed call-setup message (the server added who sent it; it can't read the payload).
type Signal struct {
	SpaceID    string `json:"space_id"`
	FromPerson string `json:"from_person"`
	FromDevice string `json:"from_device"`
	ToDevice   string `json:"to_device,omitempty"`
	Payload    []byte `json:"payload"`
}

// Listen streams events until ctx ends, reconnecting with backoff. connected(true/false) reports link state.
func (c *Client) Listen(ctx context.Context, onEvent func(Event), connected func(bool)) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := c.listenOnce(ctx, onEvent, connected)
		connected(false)
		if ctx.Err() != nil {
			return
		}
		if IsStatus(err, http.StatusUnauthorized) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (c *Client) listenOnce(ctx context.Context, onEvent func(Event), connected func(bool)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base.String()+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")
	// The stream stays open: a client without a response timeout, with a liveness watchdog instead.
	hc := &http.Client{Transport: c.hc.Transport}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &Error{Status: res.StatusCode, Message: res.Status}
	}
	connected(true)
	alive := make(chan struct{}, 1)
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { // the server pings every 25 s; 70 s of silence means the link is dead
		t := time.NewTimer(70 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-wctx.Done():
				return
			case <-alive:
				t.Reset(70 * time.Second)
			case <-t.C:
				res.Body.Close()
				return
			}
		}
	}()
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 4096), 64<<10)
	var event string
	for sc.Scan() {
		select {
		case alive <- struct{}{}:
		default:
		}
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = line[7:]
		case strings.HasPrefix(line, "data: ") && event == "change":
			var ev Event
			if json.Unmarshal([]byte(line[6:]), &ev) == nil {
				onEvent(ev)
			}
		case line == "":
			event = ""
		}
	}
	return sc.Err()
}

// loopback: plain http is only ever allowed to this same machine (a server's development listener, PP_DEV_HTTP).
func loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
