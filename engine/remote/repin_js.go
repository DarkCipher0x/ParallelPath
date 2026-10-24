//go:build js

package remote

import (
	"context"
	"encoding/json"
	"errors"
)

// IsPinChanged is always false in the web app: the browser verifies the certificate itself (nothing is pinned).
func IsPinChanged(error) bool { return false }

// Hello: see repin_native.go.
type Hello struct {
	ServerKey []byte          `json:"server_key"`
	TLSPin    json.RawMessage `json:"tls_pin"`
}

// FetchHello isn't needed in the web app.
func FetchHello(context.Context, string) ([]byte, *Hello, error) {
	return nil, nil, errors.New("not used in the web app")
}
