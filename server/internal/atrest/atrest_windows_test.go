//go:build windows

package atrest

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestJudge(t *testing.T) {
	ok := []string{"gbv|On|FullyEncrypted", "shell|1"}
	bad := []string{"gbv|Off|FullyDecrypted", "gbv|On|EncryptionInProgress", "shell|2", "shell|0", "shell|", "shell|5", ""}
	for _, s := range ok {
		if err := judge("C:", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range bad {
		if judge("C:", s) == nil {
			t.Errorf("%q accepted", s)
		}
	}
	// What a service account sees is "can't tell", never "encrypted"; a clear "off" is never "can't tell".
	for _, s := range []string{"shell|0", "shell|", "shell|5", ""} {
		if !errors.Is(judge("C:", s), ErrUnknown) {
			t.Errorf("%q should be unknown", s)
		}
	}
	for _, s := range []string{"shell|2", "gbv|Off|FullyDecrypted"} {
		if errors.Is(judge("C:", s), ErrUnknown) {
			t.Errorf("%q should be a definite no", s)
		}
	}
}

func TestVolumeID(t *testing.T) {
	id, err := volumeID(os.TempDir())
	if err != nil || !strings.HasPrefix(id, `\\?\Volume{`) {
		t.Fatalf("%q %v", id, err)
	}
}

// TestCheckRuns runs the real check on this machine's temp drive; whatever the answer, it must not fail to run.
func TestCheckRuns(t *testing.T) {
	err := check(os.TempDir())
	t.Logf("temp drive: %v", err)
	if err != nil && strings.Contains(err.Error(), "couldn't read") {
		t.Fatal(err)
	}
}
