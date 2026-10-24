package svc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"pp/core/audit"
	"pp/core/wire"
	"pp/server/internal/store"
)

// Encrypted files (photos, videos, documents). The client encrypts with core/blob and uploads ciphertext; the
// server checks size, quota and the PPB1 header, stores the file and records blob.upload. Every download is
// recorded (blob.download) BEFORE the bytes are served, so file access always leaves a trace.

// BlobDir is where encrypted files are kept.
func (s *Service) blobPath(dir, id string) string { return filepath.Join(dir, "blobs", id[:2], id) }

// StoreBlob moves an uploaded temporary file into the blob store.
func (s *Service) StoreBlob(c Caller, dir, spaceID, blobID, tmp string, size int64, intent *audit.Intent) error {
	if wire.CheckID(blobID) != nil || intent == nil || intent.Action != audit.BlobUpload || intent.SpaceID != spaceID ||
		intent.ItemID != blobID || intent.Detail != fmt.Sprintf("size=%d", size) {
		return ErrInvalid
	}
	if size > s.cfg.MaxBlobBytes {
		return fmt.Errorf("%w: file too large", ErrInvalid)
	}
	f, err := os.Open(tmp)
	if err != nil {
		return err
	}
	hdr := make([]byte, 4)
	_, rerr := io.ReadFull(f, hdr)
	f.Close()
	if rerr != nil || !bytes.Equal(hdr, []byte("PPB1")) {
		return fmt.Errorf("%w: not an encrypted ParallelPath file", ErrInvalid)
	}
	dst := s.blobPath(dir, blobID)
	var people []Member
	err = s.db.Update(func(tx *store.Tx) error {
		m, err := s.member(tx, spaceID, c.Person)
		if err != nil {
			return err
		}
		if !CanWrite(m.Role) || m.ReadOnly {
			return ErrForbidden
		}
		if tx.Has(store.Blobs, []byte(blobID)) {
			return ErrConflict
		}
		var used int64
		_ = tx.Get(store.Seqs, store.Key("quota", spaceID), &used)
		if used+size > s.cfg.SpaceQuota {
			return fmt.Errorf("%w: the family space's storage is full", ErrForbidden)
		}
		if err := tx.Put(store.Seqs, store.Key("quota", spaceID), used+size); err != nil {
			return err
		}
		if err := tx.Put(store.Blobs, []byte(blobID), Blob{ID: blobID, SpaceID: spaceID, Uploader: c.Person, Size: size, Created: s.now()}); err != nil {
			return err
		}
		if _, err := s.appendAudit(tx, c, intent); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil { // inside the transaction: no record without the file
			return err
		}
		people, _ = s.activeMembers(tx, spaceID)
		return nil
	})
	if err == nil {
		s.emit(people, Event{SpaceID: spaceID, Kind: "audit"})
	}
	return err
}

// OpenBlob records the download, then returns the encrypted file for streaming.
func (s *Service) OpenBlob(c Caller, dir, blobID string, intent *audit.Intent) (*os.File, int64, error) {
	if wire.CheckID(blobID) != nil || intent == nil || intent.Action != audit.BlobDownload || intent.ItemID != blobID {
		return nil, 0, ErrInvalid
	}
	var b Blob
	err := s.db.Update(func(tx *store.Tx) error {
		if err := tx.Get(store.Blobs, []byte(blobID), &b); err != nil {
			return ErrNotFound
		}
		if intent.SpaceID != b.SpaceID {
			return ErrInvalid
		}
		m, err := s.member(tx, b.SpaceID, c.Person)
		if err != nil {
			return err
		}
		if !m.Sees(b.Created) {
			return ErrForbidden // a professional's history window
		}
		_, err = s.appendAudit(tx, c, intent)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(s.blobPath(dir, blobID))
	if err != nil {
		return nil, 0, ErrNotFound
	}
	return f, b.Size, nil
}
