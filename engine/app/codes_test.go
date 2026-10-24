package app

import (
	"bytes"
	"testing"
)

// Codes round-trip whatever their first letter (L and R are base32 letters as well as the codes' prefixes).
func TestCodesRoundTrip(t *testing.T) {
	for i := 0; i < 2000; i++ {
		s := make([]byte, 20)
		r := make([]byte, 24)
		s[0], r[0] = byte(i), byte(i*7)
		s[19], r[23] = byte(i>>3), byte(i>>2)
		if got, err := parseLinkCode(linkCode(s)); err != nil || !bytes.Equal(got, s) {
			t.Fatalf("link code %x: %v", s, err)
		}
		if got, err := parseRecoveryCode(recoveryCode(r)); err != nil || !bytes.Equal(got, r) {
			t.Fatalf("recovery code %x: %v", r, err)
		}
	}
	if got, err := parseLinkCode(" l-" + linkCode(make([]byte, 20))[2:] + " "); err != nil || len(got) != 20 {
		t.Fatalf("lower case with spaces: %v", err)
	}
}
