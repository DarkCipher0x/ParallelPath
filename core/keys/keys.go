// Package keys defines who is who: a person's identity, the devices that act for them, and the keys a family space
// is encrypted with.
//
//   - A person has one hybrid identity signing key. Their person ID is its fingerprint.
//   - Each device has its own hybrid signing key and hybrid KEM key, certified by the identity key (DeviceCert).
//     Devices sign everything they do; the certificate ties that to the person.
//   - A family space has an epoch key per epoch. It's rotated whenever someone loses access (and on a schedule),
//     and wrapped separately to every device that may read it (EpochGrant).
package keys

import (
	"errors"
	"fmt"
	"time"

	"pp/core/pqc"
	"pp/core/wire"
)

// Signature purposes (pqc binds every signature to one of these).
const (
	PurposeDeviceCert = "device-cert"
	PurposeEpochGrant = "epoch-grant"
)

// Device kinds.
const (
	KindWindows = "windows"
	KindAndroid = "android"
	KindWeb     = "web"
	KindLinux   = "linux"
	KindMac     = "mac"
	KindIOS     = "ios"
)

// Identity is a person's private identity key (kept only on their own devices, encrypted at rest).
type Identity struct{ Key *pqc.SigningKey }

// NewIdentity creates a person's identity key.
func NewIdentity() (*Identity, error) {
	k, err := pqc.NewSigningKey()
	if err != nil {
		return nil, err
	}
	return &Identity{Key: k}, nil
}

// PersonID is the person's stable ID: the fingerprint of the identity key.
func (i *Identity) PersonID() string { return PersonID(i.Key.Public()) }

// PersonID returns the person ID for an identity verify key.
func PersonID(v pqc.VerifyKey) string { return wire.Fingerprint(v.ID()) }

// Device is a device's private keys.
type Device struct {
	Sign *pqc.SigningKey
	KEM  *pqc.KEMKey
}

// NewDevice creates a device's keys.
func NewDevice() (*Device, error) {
	s, err := pqc.NewSigningKey()
	if err != nil {
		return nil, err
	}
	k, err := pqc.NewKEMKey()
	if err != nil {
		return nil, err
	}
	return &Device{Sign: s, KEM: k}, nil
}

// DeviceID is the device's stable ID: the fingerprint of its signing key.
func (d *Device) DeviceID() string { return wire.Fingerprint(d.Sign.Public().ID()) }

// DeviceCert says "this device acts for this person". Signed by the person's identity key.
type DeviceCert struct {
	PersonID    string    `json:"person_id"`
	IdentityKey []byte    `json:"identity_key"` // the person's verify key (lets anyone check the signature)
	DeviceID    string    `json:"device_id"`
	SignKey     []byte    `json:"sign_key"` // device verify key
	KEMKey      []byte    `json:"kem_key"`  // device encryption key
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Issued      time.Time `json:"issued"`
	Signature   []byte    `json:"signature"`
}

func (c *DeviceCert) signedBytes() []byte {
	return wire.NewWriter("pp/device-cert/v1").
		String(c.PersonID).Raw(c.IdentityKey).String(c.DeviceID).Raw(c.SignKey).Raw(c.KEMKey).
		String(c.Kind).String(c.Name).Time(c.Issued).Bytes()
}

// Certify issues a certificate for a device of this person.
func (i *Identity) Certify(d *Device, kind, name string, now time.Time) (*DeviceCert, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	c := &DeviceCert{
		PersonID: i.PersonID(), IdentityKey: i.Key.Public().Bytes(),
		DeviceID: d.DeviceID(), SignKey: d.Sign.Public().Bytes(), KEMKey: d.KEM.Public().Bytes(),
		Kind: kind, Name: name, Issued: wire.Ms(now),
	}
	sig, err := i.Key.Sign(PurposeDeviceCert, c.signedBytes())
	if err != nil {
		return nil, err
	}
	c.Signature = sig
	return c, nil
}

// CertifyPublic issues a certificate for another device of this person from its public keys (linking a new device:
// the existing device certifies it, and the new device never needs to be trusted before it has a certificate).
func (i *Identity) CertifyPublic(sign pqc.VerifyKey, kem pqc.KEMPublicKey, kind, name string, now time.Time) (*DeviceCert, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	c := &DeviceCert{
		PersonID: i.PersonID(), IdentityKey: i.Key.Public().Bytes(),
		DeviceID: wire.Fingerprint(sign.ID()), SignKey: sign.Bytes(), KEMKey: kem.Bytes(),
		Kind: kind, Name: name, Issued: wire.Ms(now),
	}
	sig, err := i.Key.Sign(PurposeDeviceCert, c.signedBytes())
	if err != nil {
		return nil, err
	}
	c.Signature = sig
	return c, nil
}

// VerifiedDevice is a certificate that has been checked; only this type is used to trust a device.
type VerifiedDevice struct {
	PersonID string
	DeviceID string
	Identity pqc.VerifyKey
	Sign     pqc.VerifyKey
	KEM      pqc.KEMPublicKey
	Kind     string
	Name     string
	Issued   time.Time
}

// ErrBadCert means a certificate is malformed or its signature or IDs don't match.
var ErrBadCert = errors.New("keys: invalid device certificate")

// Verify checks the certificate: the IDs must match the keys and the identity signature must be valid.
func (c *DeviceCert) Verify() (*VerifiedDevice, error) {
	ident, err := pqc.ParseVerifyKey(c.IdentityKey)
	if err != nil || PersonID(ident) != c.PersonID {
		return nil, ErrBadCert
	}
	sk, err := pqc.ParseVerifyKey(c.SignKey)
	if err != nil || wire.Fingerprint(sk.ID()) != c.DeviceID {
		return nil, ErrBadCert
	}
	kk, err := pqc.ParseKEMPublicKey(c.KEMKey)
	if err != nil {
		return nil, ErrBadCert
	}
	if !validKind(c.Kind) || checkName(c.Name) != nil {
		return nil, ErrBadCert
	}
	if ident.Verify(PurposeDeviceCert, c.signedBytes(), c.Signature) != nil {
		return nil, ErrBadCert
	}
	return &VerifiedDevice{PersonID: c.PersonID, DeviceID: c.DeviceID, Identity: ident, Sign: sk, KEM: kk,
		Kind: c.Kind, Name: c.Name, Issued: c.Issued}, nil
}

func validKind(k string) bool {
	switch k {
	case KindWindows, KindAndroid, KindWeb, KindLinux, KindMac, KindIOS:
		return true
	}
	return false
}

func checkName(n string) error {
	if len(n) == 0 || len(n) > 64 {
		return fmt.Errorf("keys: device name must be 1-64 bytes")
	}
	return nil
}
