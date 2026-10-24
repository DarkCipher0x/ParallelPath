// Package backup keeps daily copies of the server's database, and a copy of the families' encrypted files. The copies stay sealed with the server's keys, so a
// backup folder on another (even unencrypted) drive reveals nothing; restoring needs this server's master key.
package backup

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"pp/server/internal/store"
)

const prefix, suffix = "parallelpath-", ".db"

// Run makes a backup after first, then every interval, keeping the newest keep copies of the database (and a copy of
// every encrypted file from dataDir), until ctx ends.
func Run(ctx context.Context, db *store.DB, dataDir, dir string, keep int, first, interval time.Duration, log *slog.Logger) {
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if path, err := Once(db, dir, keep, time.Now()); err != nil {
			log.Error("backup failed", "dir", dir, "err", err)
		} else {
			log.Info("backup written", "file", path)
		}
		if n, err := Blobs(dataDir, dir); err != nil {
			log.Error("backing up files failed", "dir", dir, "err", err)
		} else if n > 0 {
			log.Info("files backed up", "new", n)
		}
		t.Reset(interval)
	}
}

// Once writes one backup now and prunes old ones.
func Once(db *store.DB, dir string, keep int, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, prefix+now.UTC().Format("20060102-150405")+suffix)
	tmp := path + ".partial"
	if err := db.Backup(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, prune(dir, keep)
}

// Blobs keeps a copy of the families' encrypted files (photos, receipts, documents) next to the database backups:
// dataDir/blobs mirrored to dir/blobs. Files never change once stored, so only new ones are copied. Like the
// database copies they're ciphertext - end-to-end encrypted by the families' devices - and reveal nothing.
func Blobs(dataDir, dir string) (int, error) {
	src, dst := filepath.Join(dataDir, "blobs"), filepath.Join(dir, "blobs")
	copied := 0
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == src {
				return filepath.SkipDir // no files stored yet
			}
			return err
		}
		if !d.Type().IsRegular() || strings.HasSuffix(d.Name(), ".partial") {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		to := filepath.Join(dst, rel)
		if _, err := os.Stat(to); err == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := copyFile(path, to+".partial"); err != nil {
			_ = os.Remove(to + ".partial")
			return err
		}
		copied++
		return os.Rename(to+".partial", to)
	})
	return copied, err
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// prune deletes all but the newest keep backups (names sort by time).
func prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if n := e.Name(); e.Type().IsRegular() && strings.HasPrefix(n, prefix) && strings.HasSuffix(n, suffix) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}
