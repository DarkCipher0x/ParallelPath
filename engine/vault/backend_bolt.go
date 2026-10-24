//go:build !js

package vault

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

func headerPath(dir string) string { return filepath.Join(dir, "vault.json") }
func dbPath(dir string) string     { return filepath.Join(dir, "vault.db") }

func metaExists(dir string) bool { _, err := os.Stat(headerPath(dir)); return err == nil }

func readMeta(dir string) ([]byte, error) {
	b, err := os.ReadFile(headerPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoVault
	}
	return b, err
}

func writeMeta(dir string, b []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := headerPath(dir) + ".new"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, headerPath(dir))
}

type boltKV struct{ db *bolt.DB }

func openKV(dir string) (kv, error) {
	db, err := bolt.Open(dbPath(dir), 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &boltKV{db}, nil
}

func (b *boltKV) Get(bucket, key string) []byte {
	var out []byte
	_ = b.db.View(func(tx *bolt.Tx) error {
		if s := tx.Bucket([]byte(bucket)).Get([]byte(key)); s != nil {
			out = append([]byte(nil), s...)
		}
		return nil
	})
	return out
}

func (b *boltKV) Put(bucket, key string, val []byte) error {
	return b.db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte(bucket)).Put([]byte(key), val) })
}

func (b *boltKV) Delete(bucket, key string) error {
	return b.db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte(bucket)).Delete([]byte(key)) })
}

func (b *boltKV) Scan(bucket, prefix string, fn func(string, []byte) bool) {
	_ = b.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(bucket)).Cursor()
		p := []byte(prefix)
		for k, s := c.Seek(p); k != nil && len(k) >= len(p) && string(k[:len(p)]) == prefix; k, s = c.Next() {
			if !fn(string(k), s) {
				break
			}
		}
		return nil
	})
}

func (b *boltKV) Close() error { return b.db.Close() }
