package keys

import (
	"time"

	"pp/core/wire"
)

// PurposeTLSPin signs a server's statement of which TLS certificate (or certificate authority) it uses now. The
// apps pin the server's long-term signing key at setup, so when the certificate changes (a switch to the server's
// own CA, new certificate files) they can move their TLS pin over without trusting the network: only the real
// server can sign the statement.
const PurposeTLSPin = "pp/tls-pin/v1"

// TLSPin is the signed statement, served in /api/v1/hello.
type TLSPin struct {
	Pin       []byte    `json:"pin"` // SHA-256 of the certificate authority, or of the self-signed certificate
	At        time.Time `json:"at"`
	Signature []byte    `json:"sig"`
}

// TLSPinMessage is what the server signs.
func TLSPinMessage(pin []byte, at time.Time) []byte {
	return wire.NewWriter(PurposeTLSPin).Raw(pin).Time(wire.Ms(at)).Bytes()
}
