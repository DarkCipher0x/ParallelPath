package e2etest

import (
	"testing"
)

// TestCallAfterRestart: calls ring after the caller's and the callee's apps have been locked and unlocked again
// (an app restart), not only straight after setup.
func TestCallAfterRestart(t *testing.T) {
	var ai, bi inbox
	alice, bob, space, _, _ := familyWith(t, ai.listener(), bi.listener())
	for _, e := range []interface {
		Lock()
		Unlock(pass, secret []byte) error
	}{alice, bob} {
		e.Lock()
		if err := e.Unlock([]byte("correct horse battery"), nil); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "both online again", func() bool { return alice.Status().Online && bob.Status().Online })
	started, err := alice.CallStart(space, false)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob's phone rings after a restart", func() bool { r := bi.find("ring"); return r != nil && r["call"] == started["call"] })
}
