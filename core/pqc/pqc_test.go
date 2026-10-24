package pqc

import (
	"bytes"
	"crypto/ed25519"
	"crypto/mldsa"
	"testing"
)

func TestSignVerify(t *testing.T) {
	k, err := NewSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("hello family")
	sig, err := k.Sign("message", msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != SignatureSize {
		t.Fatalf("signature size %d", len(sig))
	}
	pub, err := ParseVerifyKey(k.Public().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.Verify("message", msg, sig); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if pub.Verify("audit-entry", msg, sig) == nil {
		t.Fatal("signature accepted for another purpose")
	}
	if pub.Verify("message", []byte("hello familY"), sig) == nil {
		t.Fatal("signature accepted for another message")
	}
	other, _ := NewSigningKey()
	if other.Public().Verify("message", msg, sig) == nil {
		t.Fatal("signature accepted under another key")
	}
}

// Each half is useless alone: corrupting either the ML-DSA or the Ed25519 part must fail.
func TestCompositeNeedsBothHalves(t *testing.T) {
	k, _ := NewSigningKey()
	sig, _ := k.Sign("message", []byte("x"))
	for _, at := range []int{10, mldsa.MLDSA87SignatureSize + 10} {
		bad := bytes.Clone(sig)
		bad[at] ^= 1
		if k.Public().Verify("message", []byte("x"), bad) == nil {
			t.Fatalf("signature with corrupted byte %d accepted", at)
		}
	}
}

// A valid Ed25519 half from one composite key must not verify under another composite key that shares it
// (the bound message includes the full composite key fingerprint).
func TestHalfCannotBeTransplanted(t *testing.T) {
	a, _ := NewSigningKey()
	seed := a.Bytes()
	seed[0] ^= 0xff // different ML-DSA half, same Ed25519 half
	b, _ := SigningKeyFromSeed(seed)
	sigA, _ := a.Sign("message", []byte("m"))
	sigB, _ := b.Sign("message", []byte("m"))
	mixed := append(bytes.Clone(sigB[:mldsa.MLDSA87SignatureSize]), sigA[mldsa.MLDSA87SignatureSize:]...)
	if b.Public().Verify("message", []byte("m"), mixed) == nil {
		t.Fatal("transplanted Ed25519 half accepted")
	}
	if len(sigA[mldsa.MLDSA87SignatureSize:]) != ed25519.SignatureSize {
		t.Fatal("layout")
	}
}

func TestSigningKeyRoundTrip(t *testing.T) {
	k, _ := NewSigningKey()
	k2, err := SigningKeyFromSeed(k.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k.Public().Bytes(), k2.Public().Bytes()) {
		t.Fatal("restored key differs")
	}
	if k.Public().ID() != k2.Public().ID() {
		t.Fatal("key id differs")
	}
	if _, err := ParseVerifyKey([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestWrapUnwrap(t *testing.T) {
	k, err := NewKEMKey()
	if err != nil {
		t.Fatal(err)
	}
	secret := NewKey()
	ct, err := Wrap(k.Public(), "space-epoch|s1|3|d1", secret)
	if err != nil {
		t.Fatal(err)
	}
	got, err := k.Unwrap("space-epoch|s1|3|d1", ct)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := k.Unwrap("space-epoch|s1|4|d1", ct); err == nil {
		t.Fatal("unwrap accepted the wrong context")
	}
	ct[len(ct)-1] ^= 1
	if _, err := k.Unwrap("space-epoch|s1|3|d1", ct); err == nil {
		t.Fatal("tampered wrap accepted")
	}
	other, _ := NewKEMKey()
	ct2, _ := Wrap(k.Public(), "c", secret)
	if _, err := other.Unwrap("c", ct2); err == nil {
		t.Fatal("unwrap with another key succeeded")
	}
	raw, _ := k.Bytes()
	k2, err := KEMKeyFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := k2.Unwrap("c", ct2); err != nil || !bytes.Equal(got, secret) {
		t.Fatal("restored KEM key can't unwrap")
	}
}

func TestSealOpen(t *testing.T) {
	key := NewKey()
	sealed, err := Seal(key, []byte("item|42"), []byte("photo bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) != SealedSize(len("photo bytes")) {
		t.Fatal("sealed size")
	}
	pt, err := Open(key, []byte("item|42"), sealed)
	if err != nil || string(pt) != "photo bytes" {
		t.Fatal("open failed")
	}
	if _, err := Open(key, []byte("item|43"), sealed); err != ErrDecrypt {
		t.Fatal("wrong aad accepted")
	}
	if _, err := Open(NewKey(), []byte("item|42"), sealed); err != ErrDecrypt {
		t.Fatal("wrong key accepted")
	}
	sealed[15] ^= 1
	if _, err := Open(key, []byte("item|42"), sealed); err != ErrDecrypt {
		t.Fatal("tampered data accepted")
	}
	if _, err := Open(key, nil, []byte("tiny")); err != ErrDecrypt {
		t.Fatal("short input accepted")
	}
}

func TestDeriveSeparation(t *testing.T) {
	s := NewKey()
	a := Derive(s, nil, "vault", 32)
	b := Derive(s, nil, "sync", 32)
	if bytes.Equal(a, b) {
		t.Fatal("different labels gave the same key")
	}
	if !bytes.Equal(a, Derive(s, nil, "vault", 32)) {
		t.Fatal("derive not deterministic")
	}
}

func TestPassword(t *testing.T) {
	h := HashPassword([]byte("correct horse battery staple"))
	ok, err := CheckPassword([]byte("correct horse battery staple"), h)
	if err != nil || !ok {
		t.Fatalf("right password rejected: %v", err)
	}
	if ok, _ := CheckPassword([]byte("wrong"), h); ok {
		t.Fatal("wrong password accepted")
	}
	if NeedsRehash(h) {
		t.Fatal("fresh hash flagged for rehash")
	}
	if _, err := CheckPassword([]byte("x"), "$argon2id$v=19$m=1024,t=1,p=1$AAAA$AAAA"); err == nil {
		t.Fatal("weak parameters accepted")
	}
}
