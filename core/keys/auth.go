package keys

import (
	"pp/core/pqc"
	"pp/core/wire"
)

// PurposeAuth is the signature purpose for signing in.
const PurposeAuth = "auth"

// AuthMessage is what a device signs to sign in: the server's own key (so a challenge can't be relayed to a
// different server), the device ID and the server's one-time nonce.
func AuthMessage(serverKey pqc.VerifyKey, deviceID string, nonce []byte) []byte {
	id := serverKey.ID()
	return wire.NewWriter("pp/auth/v1").Raw(id[:]).String(deviceID).Raw(nonce).Bytes()
}
