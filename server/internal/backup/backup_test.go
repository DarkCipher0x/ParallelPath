package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pp/core/pqc"
	"pp/server/internal/store"
)

func TestOnceKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "pp.db"), pqc.NewKey())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	out := filepath.Join(dir, "backups")
	start := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	for i := range 5 {
		if _, err := Once(db, out, 3, start.Add(time.Duration(i)*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(out, "notes.txt"), []byte("not a backup"), 0o600)
	if err := prune(out, 3); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(out)
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{"notes.txt", "parallelpath-20261003-030000.db", "parallelpath-20261004-030000.db", "parallelpath-20261005-030000.db"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestBlobsAreCopiedOnce(t *testing.T) {
	data, out := t.TempDir(), t.TempDir()
	if n, err := Blobs(data, out); err != nil || n != 0 {
		t.Fatalf("no files yet: %d %v", n, err)
	}
	src := filepath.Join(data, "blobs", "ab", "abcdef")
	os.MkdirAll(filepath.Dir(src), 0o700)
	os.WriteFile(src, []byte("ciphertext"), 0o600)
	os.WriteFile(src+"2.partial", []byte("half"), 0o600)
	if n, err := Blobs(data, out); err != nil || n != 1 {
		t.Fatalf("first copy: %d %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "blobs", "ab", "abcdef")); string(b) != "ciphertext" {
		t.Fatal("the copy differs")
	}
	if n, _ := Blobs(data, out); n != 0 {
		t.Fatal("an unchanged file was copied again")
	}
}
