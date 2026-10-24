//go:build js

package vault

import (
	"sort"
	"strings"
	"sync"
)

// In the browser the vault is a map in memory, loaded at start-up from IndexedDB (Preload) and written back through
// Persist on every change. Everything stored is sealed with the vault key, so IndexedDB only ever holds ciphertext;
// the passphrase-wrapped header is stored the same way under the "meta" bucket.

// Persist is set by the web app's host: it mirrors each change to IndexedDB (val nil = delete).
var Persist func(dir, bucket, key string, val []byte)

var (
	memMu sync.Mutex
	mem   = map[string]map[string][]byte{} // "dir|bucket" -> key -> value
)

const metaBucket = "meta"

// Preload loads one stored value (called for every IndexedDB record before the engine starts).
func Preload(dir, bucket, key string, val []byte) {
	memMu.Lock()
	defer memMu.Unlock()
	m := mem[dir+"|"+bucket]
	if m == nil {
		m = map[string][]byte{}
		mem[dir+"|"+bucket] = m
	}
	m[key] = val
}

func memGet(dir, bucket, key string) []byte {
	memMu.Lock()
	defer memMu.Unlock()
	if v, ok := mem[dir+"|"+bucket][key]; ok {
		return append([]byte(nil), v...)
	}
	return nil
}

func memPut(dir, bucket, key string, val []byte) {
	Preload(dir, bucket, key, append([]byte(nil), val...))
	if Persist != nil {
		Persist(dir, bucket, key, val)
	}
}

func metaExists(dir string) bool { return memGet(dir, metaBucket, "header") != nil }

func readMeta(dir string) ([]byte, error) {
	if b := memGet(dir, metaBucket, "header"); b != nil {
		return b, nil
	}
	return nil, ErrNoVault
}

func writeMeta(dir string, b []byte) error { memPut(dir, metaBucket, "header", b); return nil }

type memKV struct{ dir string }

func openKV(dir string) (kv, error) { return &memKV{dir}, nil }

func (m *memKV) Get(bucket, key string) []byte { return memGet(m.dir, bucket, key) }

func (m *memKV) Put(bucket, key string, val []byte) error {
	memPut(m.dir, bucket, key, val)
	return nil
}

func (m *memKV) Delete(bucket, key string) error {
	memMu.Lock()
	delete(mem[m.dir+"|"+bucket], key)
	memMu.Unlock()
	if Persist != nil {
		Persist(m.dir, bucket, key, nil)
	}
	return nil
}

func (m *memKV) Scan(bucket, prefix string, fn func(string, []byte) bool) {
	memMu.Lock()
	src := mem[m.dir+"|"+bucket]
	keys := make([]string, 0, len(src))
	for k := range src {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	vals := make([][]byte, len(keys))
	for i, k := range keys {
		vals[i] = src[k]
	}
	memMu.Unlock()
	for i, k := range keys {
		if !fn(k, vals[i]) {
			return
		}
	}
}

func (m *memKV) Close() error { return nil }
