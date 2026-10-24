// mktar packs the Linux server bundle as a .tar.gz with proper Unix permissions (tar on Windows can't): programs
// 0755, everything else 0644, owned by root, with a fixed timestamp so the same input always gives the same archive.
//
//	go run ./installer/linux/mktar OUT.tar.gz DIR
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var executable = map[string]bool{"ppserver": true, "install.sh": true}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mktar OUT.tar.gz DIR")
		os.Exit(2)
	}
	if err := pack(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "mktar:", err)
		os.Exit(1)
	}
}

func pack(out, dir string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	base := filepath.Base(dir)
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: base + "/", Mode: 0o755, ModTime: stamp, Format: tar.FormatPAX}); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			return fmt.Errorf("%s: only plain files belong in the bundle", e.Name())
		}
		src, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		st, _ := src.Stat()
		mode := int64(0o644)
		if executable[e.Name()] {
			mode = 0o755
		}
		hdr := &tar.Header{Typeflag: tar.TypeReg, Name: base + "/" + e.Name(), Size: st.Size(), Mode: mode, ModTime: stamp,
			Uname: "root", Gname: "root", Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			src.Close()
			return err
		}
		_, err = io.Copy(tw, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}
