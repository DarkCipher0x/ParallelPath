// Package blob encrypts files (photos, videos, documents) for upload. The server stores only the ciphertext.
//
// Format "PPB1": a 4-byte magic, then chunks. Each chunk is
//
//	u32 sealed length || AES-256-GCM( chunk plaintext )  with AAD = "blob|<id>|<index>|<last>"
//
// Every file has its own random key (it travels inside the encrypted message body). Binding the index and a
// last-chunk flag into each chunk's AAD stops chunks being reordered, dropped, duplicated or the file being cut
// short; binding the ID stops chunks being swapped between files.
package blob

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"

	"crypto/sha3"

	"pp/core/pqc"
)

// ChunkSize is the plaintext size of every chunk except the last.
const ChunkSize = 1 << 20

var magic = []byte("PPB1")

// ErrCorrupt is returned for any malformed, truncated or tampered blob.
var ErrCorrupt = errors.New("blob: corrupt or tampered file")

func aad(id string, index uint64, last bool) []byte {
	l := 0
	if last {
		l = 1
	}
	return []byte(fmt.Sprintf("blob|%s|%d|%d", id, index, l))
}

// Result describes an encrypted file.
type Result struct {
	Size   int64  // plaintext bytes
	Sealed int64  // ciphertext bytes written
	SHA3   []byte // SHA3-512 of the plaintext (goes in the message body, checked after download)
	Chunks uint64
}

// Encrypt reads plaintext from r and writes the encrypted blob to w.
func Encrypt(id string, key []byte, r io.Reader, w io.Writer) (*Result, error) {
	if len(key) != pqc.KeySize {
		return nil, errors.New("blob: key must be 32 bytes")
	}
	res := &Result{}
	h := sha3.New512()
	br := bufio.NewReaderSize(r, ChunkSize)
	if _, err := w.Write(magic); err != nil {
		return nil, err
	}
	res.Sealed = int64(len(magic))
	cur := make([]byte, ChunkSize)
	n, err := io.ReadFull(br, cur)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	cur = cur[:n]
	for index := uint64(0); ; index++ {
		// Peek one byte to know whether this chunk is the last one.
		_, perr := br.Peek(1)
		last := perr != nil
		h.Write(cur)
		sealed, err := pqc.Seal(key, aad(id, index, last), cur)
		if err != nil {
			return nil, err
		}
		var lenb [4]byte
		binary.BigEndian.PutUint32(lenb[:], uint32(len(sealed)))
		if _, err := w.Write(lenb[:]); err != nil {
			return nil, err
		}
		if _, err := w.Write(sealed); err != nil {
			return nil, err
		}
		res.Size += int64(len(cur))
		res.Sealed += int64(4 + len(sealed))
		res.Chunks++
		if last {
			break
		}
		cur = cur[:ChunkSize]
		n, err := io.ReadFull(br, cur)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return nil, err
		}
		cur = cur[:n]
	}
	res.SHA3 = h.Sum(nil)
	return res, nil
}

// Decrypt reads an encrypted blob from r and writes the plaintext to w. It fails (having possibly written a
// prefix) if anything was changed; callers write to a temporary file and only keep it on success. wantSHA3, if
// given, must equal the plaintext hash.
func Decrypt(id string, key []byte, r io.Reader, w io.Writer, wantSHA3 []byte) (int64, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var hdr [4]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil || string(hdr[:]) != string(magic) {
		return 0, ErrCorrupt
	}
	var h hash.Hash = sha3.New512()
	var total int64
	sealed := make([]byte, 0, pqc.SealedSize(ChunkSize))
	for index := uint64(0); ; index++ {
		var lenb [4]byte
		if _, err := io.ReadFull(br, lenb[:]); err != nil {
			return total, ErrCorrupt // ran out before a chunk marked last: truncated
		}
		n := binary.BigEndian.Uint32(lenb[:])
		if n < uint32(pqc.SealedSize(0)) || n > uint32(pqc.SealedSize(ChunkSize)) {
			return total, ErrCorrupt
		}
		sealed = sealed[:n]
		if _, err := io.ReadFull(br, sealed); err != nil {
			return total, ErrCorrupt
		}
		_, perr := br.Peek(1)
		last := perr != nil // nothing follows: this must be the chunk sealed as last
		pt, err := pqc.Open(key, aad(id, index, last), sealed)
		if err != nil {
			return total, ErrCorrupt
		}
		h.Write(pt)
		if _, err := w.Write(pt); err != nil {
			return total, err
		}
		total += int64(len(pt))
		if last {
			break
		}
	}
	if wantSHA3 != nil && !pqc.Equal(h.Sum(nil), wantSHA3) {
		return total, ErrCorrupt
	}
	return total, nil
}
