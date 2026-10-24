//go:build !linux && !windows

package atrest

import "errors"

func check(string) error {
	return errors.New("atrest: encrypted-volume check not implemented on this OS")
}

func volumeID(string) (string, error) { return "", nil }
