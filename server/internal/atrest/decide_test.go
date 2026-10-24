package atrest

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	att := &Attestation{Volume: `\?\Volume{a}\`, Time: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}
	unknown := fmt.Errorf("%w (drive C:, shell|0)", ErrUnknown)
	off := errors.New("atrest: drive C: is not protected by BitLocker")
	cases := []struct {
		name    string
		err     error
		att     *Attestation
		volume  string
		allow   bool
		started bool
	}{
		{"verified live", nil, nil, "", false, true},
		{"unknown, attested same volume", unknown, att, `\?\Volume{a}\`, false, true},
		{"unknown, attested another volume", unknown, att, `\?\Volume{b}\`, false, false},
		{"unknown, never attested", unknown, nil, `\?\Volume{a}\`, false, false},
		{"definitely off beats an attestation", off, att, `\?\Volume{a}\`, false, false},
		{"off, testing allowed", off, nil, "", true, true},
	}
	for _, c := range cases {
		_, err := Decide(c.err, c.att, c.volume, c.allow)
		if (err == nil) != c.started {
			t.Errorf("%s: started=%v err=%v", c.name, err == nil, err)
		}
	}
}
