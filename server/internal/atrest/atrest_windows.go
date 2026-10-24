//go:build windows

package atrest

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// check asks Windows whether the drive holding path is protected by BitLocker.
//
// It first uses the BitLocker PowerShell module, which is the most precise (protection on and fully encrypted) but
// needs administrator rights. Without them (the service runs as a low-privilege virtual account) it falls back to
// the shell's System.Volume.BitLockerProtection property: 1 when protection is on, 2 when off, and "unknown" inside a
// service (see ErrUnknown). Property values are numbers or enum names, so this works in any display language.
func check(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	vol := filepath.VolumeName(abs)
	if len(vol) != 2 || vol[1] != ':' {
		return fmt.Errorf("atrest: %s is not on a local drive letter; BitLocker can't be checked", abs)
	}
	ps := fmt.Sprintf(`try { $v = Get-BitLockerVolume -MountPoint '%[1]s' -ErrorAction Stop; "gbv|$($v.ProtectionStatus)|$($v.VolumeStatus)" }
catch { $p = (New-Object -ComObject Shell.Application).NameSpace('%[1]s\').Self.ExtendedProperty('System.Volume.BitLockerProtection'); "shell|$p" }`, vol)
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil {
		return fmt.Errorf("atrest: couldn't read the BitLocker status of %s: %v", vol, err)
	}
	return judge(vol, strings.TrimSpace(string(out)))
}

// judge interprets the status line check printed. The shell property is 1 when protection is on and 2 when it's
// off; anything else (0 or empty, which is what a service account gets) means it couldn't tell.
func judge(vol, st string) error {
	switch {
	case strings.HasPrefix(st, "gbv|On|") && strings.HasSuffix(st, "|FullyEncrypted"):
		return nil
	case st == "shell|1":
		return nil
	case strings.HasPrefix(st, "gbv|"):
		return fmt.Errorf("atrest: drive %s is not fully BitLocker-encrypted with protection on (%s)", vol, strings.TrimPrefix(st, "gbv|"))
	case st == "shell|2":
		return fmt.Errorf("atrest: drive %s is not protected by BitLocker", vol)
	default:
		return fmt.Errorf("%w (drive %s, %s)", ErrUnknown, vol, st)
	}
}

// volumeID returns the \\?\Volume{GUID}\ name of the volume holding path (any account may ask).
func volumeID(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_PATH+1)
	p, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return "", err
	}
	if err := windows.GetVolumePathName(p, &buf[0], uint32(len(buf))); err != nil {
		return "", err
	}
	mount := windows.UTF16ToString(buf)
	m, _ := windows.UTF16PtrFromString(mount)
	name := make([]uint16, 64)
	if err := windows.GetVolumeNameForVolumeMountPoint(m, &name[0], uint32(len(name))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(name), nil
}
