//go:build linux

package atrest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// check finds the mount holding path in /proc/self/mountinfo and asks the kernel whether its block device (or one
// of the devices under it) is a dm-crypt target.
func check(path string) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer f.Close()
	best, dev := "", ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		p := strings.Fields(s.Text())
		if len(p) < 5 {
			continue
		}
		mnt := strings.ReplaceAll(p[4], `\040`, " ")
		if (real == mnt || strings.HasPrefix(real, strings.TrimSuffix(mnt, "/")+"/")) && len(mnt) >= len(best) {
			best, dev = mnt, p[2]
		}
	}
	if dev == "" {
		return fmt.Errorf("atrest: can't find the volume holding %s", real)
	}
	if isCrypt(dev, 0) {
		return nil
	}
	return fmt.Errorf("atrest: %s (mounted at %s) is not on an encrypted LUKS/dm-crypt volume", real, best)
}

func isCrypt(dev string, depth int) bool {
	if depth > 4 {
		return false
	}
	if b, err := os.ReadFile("/sys/dev/block/" + dev + "/dm/uuid"); err == nil && strings.HasPrefix(string(b), "CRYPT-") {
		return true
	}
	slaves, _ := os.ReadDir("/sys/dev/block/" + dev + "/slaves")
	for _, s := range slaves {
		b, err := os.ReadFile("/sys/class/block/" + s.Name() + "/dev")
		if err == nil && isCrypt(strings.TrimSpace(string(b)), depth+1) {
			return true
		}
	}
	return false
}

// The Linux check works for any account, so there is nothing to attest.
func volumeID(string) (string, error) { return "", nil }
