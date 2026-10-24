package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"pp/core/keys"
	"pp/engine/remote"
	"pp/engine/vault"
)

// followCertificate moves this device's TLS pin to the server's new certificate (or certificate authority), but
// only when the server proves the change with a statement signed by the server key this device pinned at setup -
// something a network attacker can't produce. Anything else leaves the old pin in place and the connection
// refused.
func (e *Engine) followCertificate() error {
	e.mu.Lock()
	server, v := e.server, e.v
	e.mu.Unlock()
	if v == nil {
		return errLocked
	}
	if len(server.Pin) == 0 {
		return errors.New("the server's certificate isn't trusted any more")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	seen, hello, err := remote.FetchHello(ctx, server.URL)
	if err != nil {
		return err
	}
	if bytes.Equal(seen, server.Pin) {
		return nil // it was something else (a passing network problem)
	}
	if !bytes.Equal(hello.ServerKey, server.Key) {
		return errors.New("the server's identity has changed - this could be an attack; ask the server's owner")
	}
	var st keys.TLSPin
	if len(hello.TLSPin) == 0 || json.Unmarshal(hello.TLSPin, &st) != nil {
		return errors.New("the server's certificate has changed without a signed notice - ask the server's owner")
	}
	if !bytes.Equal(st.Pin, seen) || e.serverKey().Verify(keys.PurposeTLSPin, keys.TLSPinMessage(st.Pin, st.At), st.Signature) != nil {
		return errors.New("the server's certificate has changed and its signed notice doesn't match - this could be an attack")
	}
	if d := time.Since(st.At); d > time.Hour || d < -time.Hour {
		return errors.New("the server's certificate notice is out of date - check this device's clock")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.v != v {
		return errLocked
	}
	srv, err := remote.New(server.URL, seen)
	if err != nil {
		return err
	}
	var tok string
	if v.Get(vault.Keys, "session", &tok) == nil {
		srv.SetToken(tok)
	}
	e.server.Pin = seen
	if err := v.Put(vault.Keys, "server", e.server); err != nil {
		return err
	}
	if e.cancel != nil {
		e.cancel()
	}
	e.srv = srv
	e.startLocked()
	e.listen("changed", map[string]string{"what": "certificate"})
	return nil
}
