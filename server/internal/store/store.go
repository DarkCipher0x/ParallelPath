// Package store is the server's database: an embedded, transactional key-value store (bbolt) in one file.
//
// Every value is sealed with the server's data key (AES-256-GCM, AAD = bucket + key), so the database file is
// unreadable without the sealed master key, even before considering that family content inside it is already
// end-to-end encrypted. Keys (the lookup paths) are IDs and sequence numbers only, never personal data.
package store

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"

	"pp/core/pqc"
)

// Buckets.
const (
	Meta          = "meta"
	Persons       = "persons"
	Devices       = "devices"
	PersonDevices = "person_devices" // person/device
	Sessions      = "sessions"
	Challenges    = "challenges"
	Invites       = "invites"
	Spaces        = "spaces"
	Members       = "members"        // space/person
	PersonSpaces  = "person_spaces"  // person/space
	Grants        = "grants"         // space/epoch(8)/device
	Items         = "items"          // space/seq(8)
	ItemIndex     = "item_index"     // space/item/rev(8) -> seq
	Audit         = "audit"          // space/seq(8)   (space "" = the server chain)
	AuditHeads    = "audit_heads"    // space -> seq
	Blobs         = "blobs"          // blob id -> metadata
	Seqs          = "seqs"           // per-space item sequence counters
	AdminAccounts = "admin_accounts" // server operators (admin portal)
	AdminSessions = "admin_sessions" // signed-in operators (keyed hash of the cookie)
	AdminTokens   = "admin_tokens"   // one-time setup and local sign-in links (keyed hash)
	Recovery      = "recovery"       // recovery kit id -> sealed kit (the person's own, unreadable without their code)
	RecoveryOf    = "recovery_of"    // person -> their kit id
)

var all = []string{Meta, Persons, Devices, PersonDevices, Sessions, Challenges, Invites, Spaces, Members,
	PersonSpaces, Grants, Items, ItemIndex, Audit, AuditHeads, Blobs, Seqs, AdminAccounts, AdminSessions, AdminTokens,
	Recovery, RecoveryOf}

// ErrNotFound is returned when a key doesn't exist.
var ErrNotFound = errors.New("store: not found")

// DB is the open database.
type DB struct {
	b   *bolt.DB
	key []byte
}

// Open opens (or creates) the database. key is the 32-byte data key unsealed from the master key.
func Open(path string, key []byte) (*DB, error) {
	if len(key) != pqc.KeySize {
		return nil, errors.New("store: bad data key")
	}
	b, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second, FreelistType: bolt.FreelistMapType})
	if err != nil {
		return nil, err
	}
	err = b.Update(func(tx *bolt.Tx) error {
		for _, n := range all {
			if _, err := tx.CreateBucketIfNotExists([]byte(n)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Close()
		return nil, err
	}
	return &DB{b: b, key: append([]byte(nil), key...)}, nil
}

// Close closes the database.
func (d *DB) Close() error { pqc.Wipe(d.key); return d.b.Close() }

// Tx is a transaction. All reads and writes inside one Update are atomic.
type Tx struct {
	tx  *bolt.Tx
	key []byte
}

// View runs a read-only transaction.
func (d *DB) View(fn func(*Tx) error) error {
	return d.b.View(func(tx *bolt.Tx) error { return fn(&Tx{tx, d.key}) })
}

// Update runs a read-write transaction; it commits only if fn returns nil.
func (d *DB) Update(fn func(*Tx) error) error {
	return d.b.Update(func(tx *bolt.Tx) error { return fn(&Tx{tx, d.key}) })
}

func aad(bucket string, key []byte) []byte {
	return append(append([]byte("store|"+bucket+"|"), key...), 0)
}

// Put stores v (JSON-encoded, then sealed) under key.
func (t *Tx) Put(bucket string, key []byte, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sealed, err := pqc.Seal(t.key, aad(bucket, key), raw)
	pqc.Wipe(raw)
	if err != nil {
		return err
	}
	return t.tx.Bucket([]byte(bucket)).Put(key, sealed)
}

// Get loads and unseals the value under key into v.
func (t *Tx) Get(bucket string, key []byte, v any) error {
	sealed := t.tx.Bucket([]byte(bucket)).Get(key)
	if sealed == nil {
		return ErrNotFound
	}
	return t.open(bucket, key, sealed, v)
}

func (t *Tx) open(bucket string, key, sealed []byte, v any) error {
	raw, err := pqc.Open(t.key, aad(bucket, key), sealed)
	if err != nil {
		return errors.New("store: value failed to decrypt (database tampered or wrong key)")
	}
	defer pqc.Wipe(raw)
	return json.Unmarshal(raw, v)
}

// Has reports whether key exists.
func (t *Tx) Has(bucket string, key []byte) bool {
	return t.tx.Bucket([]byte(bucket)).Get(key) != nil
}

// Delete removes key (only used for short-lived records: challenges, sessions, used invites).
func (t *Tx) Delete(bucket string, key []byte) error {
	return t.tx.Bucket([]byte(bucket)).Delete(key)
}

// Scan visits every key starting with prefix, from start (inclusive, nil = beginning of the prefix), in order.
// fn returns false to stop. Values are unsealed into a fresh value from newV.
func Scan[T any](t *Tx, bucket string, prefix, start []byte, fn func(key []byte, v *T) bool) error {
	c := t.tx.Bucket([]byte(bucket)).Cursor()
	from := prefix
	if start != nil {
		from = start
	}
	for k, sealed := c.Seek(from); k != nil && hasPrefix(k, prefix); k, sealed = c.Next() {
		var v T
		if err := t.open(bucket, k, sealed, &v); err != nil {
			return err
		}
		if !fn(append([]byte(nil), k...), &v) {
			break
		}
	}
	return nil
}

// Keys visits the keys under prefix without decrypting the values (cheap membership listing).
func (t *Tx) Keys(bucket string, prefix []byte, fn func(key []byte) bool) {
	c := t.tx.Bucket([]byte(bucket)).Cursor()
	for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
		if !fn(append([]byte(nil), k...)) {
			return
		}
	}
}

// NextSeq increments and returns the counter called name.
func (t *Tx) NextSeq(name string) (uint64, error) {
	b := t.tx.Bucket([]byte(Seqs))
	var n uint64
	if v := b.Get([]byte(name)); len(v) == 8 {
		n = binary.BigEndian.Uint64(v)
	}
	n++
	return n, b.Put([]byte(name), U64(n))
}

// Key joins path parts with "/" into a lookup key.
func Key(parts ...string) []byte {
	n := 0
	for _, p := range parts {
		n += len(p) + 1
	}
	b := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			b = append(b, '/')
		}
		b = append(b, p...)
	}
	return b
}

// KeySeq is Key(parts...) + "/" + big-endian seq, so keys sort numerically.
func KeySeq(seq uint64, parts ...string) []byte {
	return append(append(Key(parts...), '/'), U64(seq)...)
}

// U64 encodes a sequence number big-endian.
func U64(n uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, n); return b }

// SeqOf decodes the trailing 8-byte sequence from a KeySeq key.
func SeqOf(k []byte) uint64 {
	if len(k) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(k[len(k)-8:])
}

func hasPrefix(k, p []byte) bool {
	if len(k) < len(p) {
		return false
	}
	for i := range p {
		if k[i] != p[i] {
			return false
		}
	}
	return true
}

// Backup writes a consistent copy of the (still sealed) database to path.
func (d *DB) Backup(path string) error {
	return d.b.View(func(tx *bolt.Tx) error { return tx.CopyFile(path, 0o600) })
}
