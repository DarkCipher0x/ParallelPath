//go:build windows

package seal

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var entropy = []byte("ParallelPath/master-key/v1")

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// protect seals with DPAPI for the current account (the service account), with fixed entropy.
func protect(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	err := windows.CryptProtectData(blob(plain), windows.StringToUTF16Ptr("ParallelPath master key"), blob(entropy), 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func unprotect(sealed []byte) ([]byte, error) {
	var out windows.DataBlob
	err := windows.CryptUnprotectData(blob(sealed), nil, blob(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, errors.New("seal: can't unseal the master key (wrong Windows account or machine?)")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
