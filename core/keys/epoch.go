package keys

import (
	"errors"
	"fmt"
	"time"

	"pp/core/pqc"
	"pp/core/wire"
)

// EpochGrant hands one space epoch key to one device: the key is HPKE-wrapped (ML-KEM-1024 + P-384) to that
// device's KEM key, and the grant is signed by the device that created or re-shared it.
type EpochGrant struct {
	SpaceID    string    `json:"space_id"`
	Epoch      uint64    `json:"epoch"`
	ToDevice   string    `json:"to_device"`
	FromDevice string    `json:"from_device"`
	Wrapped    []byte    `json:"wrapped"`
	ReadOnly   bool      `json:"read_only"`           // professionals get read-only grants
	NotAfter   time.Time `json:"not_after,omitempty"` // time-limited access (zero = until rotated away)
	Created    time.Time `json:"created"`
	Signature  []byte    `json:"signature"`
}

func (g *EpochGrant) signedBytes() []byte {
	return wire.NewWriter("pp/epoch-grant/v1").
		String(g.SpaceID).Uint(g.Epoch).String(g.ToDevice).String(g.FromDevice).Raw(g.Wrapped).
		Bool(g.ReadOnly).Time(g.NotAfter).Time(g.Created).Bytes()
}

func wrapContext(space string, epoch uint64, device string) string {
	return fmt.Sprintf("space-epoch|%s|%d|%s", space, epoch, device)
}

// NewEpochKey returns a fresh 256-bit epoch key.
func NewEpochKey() []byte { return pqc.NewKey() }

// Grant wraps an epoch key to a verified recipient device and signs the grant with the sending device.
func Grant(from *Device, to *VerifiedDevice, space string, epoch uint64, key []byte, readOnly bool, notAfter, now time.Time) (*EpochGrant, error) {
	if err := wire.CheckID(space); err != nil {
		return nil, err
	}
	w, err := pqc.Wrap(to.KEM, wrapContext(space, epoch, to.DeviceID), key)
	if err != nil {
		return nil, err
	}
	g := &EpochGrant{SpaceID: space, Epoch: epoch, ToDevice: to.DeviceID, FromDevice: from.DeviceID(), Wrapped: w,
		ReadOnly: readOnly, Created: wire.Ms(now)}
	if !notAfter.IsZero() {
		g.NotAfter = wire.Ms(notAfter)
	}
	if g.Signature, err = from.Sign.Sign(PurposeEpochGrant, g.signedBytes()); err != nil {
		return nil, err
	}
	return g, nil
}

// ErrBadGrant means a grant is not for this device, isn't signed by the stated sender, or doesn't open.
var ErrBadGrant = errors.New("keys: invalid epoch grant")

// VerifySignature checks the sender's signature without unwrapping (what the server does before storing a grant).
func (g *EpochGrant) VerifySignature(from *VerifiedDevice) error {
	if g.FromDevice != from.DeviceID || wire.CheckID(g.SpaceID) != nil || len(g.ToDevice) != 32 {
		return ErrBadGrant
	}
	if from.Sign.Verify(PurposeEpochGrant, g.signedBytes(), g.Signature) != nil {
		return ErrBadGrant
	}
	return nil
}

// Open verifies the grant against the sending device's verified certificate and unwraps the epoch key.
func (g *EpochGrant) Open(me *Device, from *VerifiedDevice) ([]byte, error) {
	if g.ToDevice != me.DeviceID() || g.FromDevice != from.DeviceID {
		return nil, ErrBadGrant
	}
	if from.Sign.Verify(PurposeEpochGrant, g.signedBytes(), g.Signature) != nil {
		return nil, ErrBadGrant
	}
	k, err := me.KEM.Unwrap(wrapContext(g.SpaceID, g.Epoch, g.ToDevice), g.Wrapped)
	if err != nil || len(k) != pqc.KeySize {
		return nil, ErrBadGrant
	}
	return k, nil
}
