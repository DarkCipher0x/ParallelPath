package vault

// kv is where the vault's (already sealed) values live: a bbolt file on Windows and Android, an in-memory map
// mirrored to the browser's IndexedDB in the web app. Values are ciphertext either way.
type kv interface {
	Get(bucket, key string) []byte // nil when missing
	Put(bucket, key string, val []byte) error
	Delete(bucket, key string) error
	Scan(bucket, prefix string, fn func(key string, val []byte) bool) // in key order
	Close() error
}
