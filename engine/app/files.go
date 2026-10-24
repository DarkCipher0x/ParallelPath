package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pp/core/audit"
	"pp/core/blob"
	"pp/core/envelope"
	"pp/core/pqc"
	"pp/core/wire"
)

// FileInput is a file the person picked to send. Apps may pass a thumbnail and dimensions they already have
// (Android and Windows can render video thumbnails); for photos the engine makes the thumbnail itself.
type FileInput struct {
	Path   string `json:"path"`
	Name   string `json:"name,omitempty"`
	Mime   string `json:"mime,omitempty"`
	Thumb  []byte `json:"thumb,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Millis int64  `json:"millis,omitempty"`
}

const maxFile = 2 << 30 // 2 GiB, matches the server default

// uploadFile encrypts a file locally (fresh key, 1 MiB AES-256-GCM chunks) and uploads only the ciphertext.
func (e *Engine) uploadFile(space string, f FileInput) (*envelope.Attachment, error) {
	src, err := os.Open(f.Path)
	if err != nil {
		return nil, fmt.Errorf("can't open %s: %w", filepath.Base(f.Path), err)
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > maxFile {
		return nil, fmt.Errorf("%s is larger than 2 GB", filepath.Base(f.Path))
	}
	name := f.Name
	if name == "" {
		name = filepath.Base(f.Path)
	}
	name = cleanName(name)
	typ := f.Mime
	if typ == "" {
		typ = mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
		if typ == "" {
			typ = "application/octet-stream"
		}
	}
	a := &envelope.Attachment{BlobID: wire.NewID(), Name: name, Mime: typ, Size: st.Size(), Key: pqc.NewKey(),
		Width: f.Width, Height: f.Height, Millis: f.Millis, Thumb: f.Thumb}
	if strings.HasPrefix(typ, "image/") && len(a.Thumb) == 0 {
		if th, w, h, err := thumbnail(f.Path); err == nil {
			a.Thumb, a.Width, a.Height = th, w, h
		}
	}
	if len(a.Thumb) > 64<<10 {
		a.Thumb = nil // keep message bodies small; the app will show a placeholder
	}

	if err := os.MkdirAll(e.tmpDir(), 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(e.tmpDir(), "enc-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	res, err := blob.Encrypt(a.BlobID, a.Key, src, tmp)
	if err != nil {
		return nil, err
	}
	a.SHA3 = res.SHA3
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.BlobUpload, ItemID: a.BlobID, Detail: fmt.Sprintf("size=%d", res.Sealed)})
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	srv := e.srv
	e.mu.Unlock()
	if srv == nil {
		return nil, errLocked
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	if err := srv.PutBlob(ctx, space, a.BlobID, tmp, res.Sealed, in); err != nil {
		return nil, fmt.Errorf("upload of %s failed: %w", name, err)
	}
	return a, nil
}

// OpenAttachment downloads and decrypts a file into this device's private cache and returns its path.
// The download is recorded in the family's audit log. The cache is wiped when the app locks.
func (e *Engine) OpenAttachment(space, messageID, blobID string) (string, error) {
	cur, err := e.latest(space, messageID)
	if err != nil {
		return "", err
	}
	var a *envelope.Attachment
	if cur.body != nil {
		for i := range cur.body.Attachments {
			if cur.body.Attachments[i].BlobID == blobID {
				a = &cur.body.Attachments[i]
			}
		}
	}
	if a == nil {
		// The file may belong to an earlier revision (edits keep files; retractions hide them).
		all, _ := e.loadDecoded(space)
		for _, d := range all {
			if d.rec.Item.ItemID == messageID && d.body != nil {
				for i := range d.body.Attachments {
					if d.body.Attachments[i].BlobID == blobID {
						a = &d.body.Attachments[i]
					}
				}
			}
		}
	}
	if a == nil {
		return "", errors.New("that file isn't attached to this item")
	}
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.BlobDownload, ItemID: blobID})
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	srv := e.srv
	e.mu.Unlock()
	if srv == nil {
		return "", errLocked
	}
	if err := os.MkdirAll(e.tmpDir(), 0o700); err != nil {
		return "", err
	}
	enc, err := os.CreateTemp(e.tmpDir(), "dl-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(enc.Name())
	defer enc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	if err := srv.GetBlob(ctx, blobID, in, enc); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	if _, err := enc.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	dir := filepath.Join(e.cacheDir(), randHex(8))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	out := filepath.Join(dir, a.Name)
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, derr := blob.Decrypt(blobID, a.Key, enc, f, a.SHA3)
	cerr := f.Close()
	if derr != nil || cerr != nil {
		os.Remove(out)
		return "", errors.New("the file failed its integrity check and was discarded")
	}
	return out, nil
}

// cleanName keeps a file name safe to write on any OS.
func cleanName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	var b strings.Builder
	for _, r := range n {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	s := strings.Trim(b.String(), ". ")
	if s == "" {
		s = "file"
	}
	if r := []rune(s); len(r) > 120 {
		s = string(r[len(r)-120:])
	}
	return s
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// thumbnail makes a JPEG preview up to 320 px on its longer side (area-averaged, so it stays sharp).
func thumbnail(path string) ([]byte, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, 0, 0, err
	}
	if cfg.Width*cfg.Height > 80_000_000 { // refuse decompression bombs
		return nil, cfg.Width, cfg.Height, errors.New("image too large to preview")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, 0, 0, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := w, h
	if w >= h && w > 320 {
		tw, th = 320, max(1, h*320/w)
	} else if h > w && h > 320 {
		tw, th = max(1, w*320/h), 320
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		y0, y1 := b.Min.Y+y*h/th, b.Min.Y+max((y+1)*h/th, y*h/th+1)
		for x := 0; x < tw; x++ {
			x0, x1 := b.Min.X+x*w/tw, b.Min.X+max((x+1)*w/tw, x*w/tw+1)
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := img.At(xx, yy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			dst.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 75}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), w, h, nil
}
