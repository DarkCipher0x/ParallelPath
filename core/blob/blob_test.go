package blob

import (
	"bytes"
	"crypto/rand"
	"testing"

	"pp/core/pqc"
	"pp/core/wire"
)

func roundTrip(t *testing.T, size int) {
	t.Helper()
	data := make([]byte, size)
	rand.Read(data)
	id, key := wire.NewID(), pqc.NewKey()
	var enc bytes.Buffer
	res, err := Encrypt(id, key, bytes.NewReader(data), &enc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Size != int64(size) || res.Sealed != int64(enc.Len()) {
		t.Fatalf("sizes %d/%d", res.Size, res.Sealed)
	}
	var dec bytes.Buffer
	if _, err := Decrypt(id, key, bytes.NewReader(enc.Bytes()), &dec, res.SHA3); err != nil {
		t.Fatalf("size %d: %v", size, err)
	}
	if !bytes.Equal(dec.Bytes(), data) {
		t.Fatalf("size %d: mismatch", size)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, ChunkSize - 1, ChunkSize, ChunkSize + 1, 3*ChunkSize + 777} {
		roundTrip(t, n)
	}
}

func TestTamper(t *testing.T) {
	data := make([]byte, 2*ChunkSize+5)
	rand.Read(data)
	id, key := wire.NewID(), pqc.NewKey()
	var enc bytes.Buffer
	res, _ := Encrypt(id, key, bytes.NewReader(data), &enc)
	blob := enc.Bytes()
	chunk := 4 + pqc.SealedSize(ChunkSize)

	cases := map[string][]byte{
		"truncated after a full chunk": blob[:4+chunk],
		"flipped bit":                  flip(blob, 4+chunk+100),
		"chunks swapped":               swap(blob, chunk),
		"no magic":                     blob[4:],
	}
	for name, b := range cases {
		if _, err := Decrypt(id, key, bytes.NewReader(b), &bytes.Buffer{}, res.SHA3); err != ErrCorrupt {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := Decrypt(wire.NewID(), key, bytes.NewReader(blob), &bytes.Buffer{}, nil); err != ErrCorrupt {
		t.Error("decrypted under another blob id")
	}
	wrong := bytes.Clone(res.SHA3)
	wrong[0] ^= 1
	if _, err := Decrypt(id, key, bytes.NewReader(blob), &bytes.Buffer{}, wrong); err != ErrCorrupt {
		t.Error("accepted with the wrong hash")
	}
}

func flip(b []byte, at int) []byte { c := bytes.Clone(b); c[at] ^= 1; return c }

func swap(b []byte, chunk int) []byte {
	c := bytes.Clone(b)
	first := bytes.Clone(c[4 : 4+chunk])
	copy(c[4:4+chunk], c[4+chunk:4+2*chunk])
	copy(c[4+chunk:4+2*chunk], first)
	return c
}
