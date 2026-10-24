// Package wire is the canonical byte encoding that every signature and every hash in ParallelPath covers.
//
// Signatures are never computed over JSON (field order, spacing and number formats vary between encoders).
// Instead each signed structure writes its fields, in a fixed order, through a Writer:
// type-tagged and length-prefixed, so two different structures can never encode to the same bytes.
package wire

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

const (
	tagUint   = 0x01
	tagBytes  = 0x02
	tagString = 0x03
	tagBool   = 0x04
	tagTime   = 0x05
	tagList   = 0x06
)

// Writer builds canonical bytes. It starts with a domain label (for example "pp/item/v1") so the encodings of
// different structure types never collide.
type Writer struct{ b []byte }

// NewWriter starts an encoding for the given structure type.
func NewWriter(domain string) *Writer {
	w := &Writer{b: make([]byte, 0, 256)}
	w.String(domain)
	return w
}

// Bytes returns the encoding.
func (w *Writer) Bytes() []byte { return w.b }

// Uint appends an unsigned integer.
func (w *Writer) Uint(v uint64) *Writer {
	w.b = append(w.b, tagUint)
	w.b = binary.BigEndian.AppendUint64(w.b, v)
	return w
}

// Raw appends a byte string (nil and empty encode the same).
func (w *Writer) Raw(v []byte) *Writer {
	w.b = append(w.b, tagBytes)
	w.b = binary.BigEndian.AppendUint32(w.b, uint32(len(v)))
	w.b = append(w.b, v...)
	return w
}

// String appends a UTF-8 string.
func (w *Writer) String(v string) *Writer {
	w.b = append(w.b, tagString)
	w.b = binary.BigEndian.AppendUint32(w.b, uint32(len(v)))
	w.b = append(w.b, v...)
	return w
}

// Bool appends a boolean.
func (w *Writer) Bool(v bool) *Writer {
	w.b = append(w.b, tagBool)
	if v {
		w.b = append(w.b, 1)
	} else {
		w.b = append(w.b, 0)
	}
	return w
}

// Time appends a timestamp in UTC milliseconds (the precision every ParallelPath time is stored with).
func (w *Writer) Time(t time.Time) *Writer {
	w.b = append(w.b, tagTime)
	w.b = binary.BigEndian.AppendUint64(w.b, uint64(t.UTC().UnixMilli()))
	return w
}

// Strings appends a list of strings.
func (w *Writer) Strings(v []string) *Writer {
	w.b = append(w.b, tagList)
	w.b = binary.BigEndian.AppendUint32(w.b, uint32(len(v)))
	for _, s := range v {
		w.String(s)
	}
	return w
}

// Ms converts a time to the stored precision (UTC, whole milliseconds), so values round-trip exactly.
func Ms(t time.Time) time.Time { return time.UnixMilli(t.UTC().UnixMilli()).UTC() }

// ErrTooLarge is returned when a field exceeds its size limit.
var ErrTooLarge = errors.New("wire: value too large")

// CheckLen enforces a size limit before data is signed, stored or sent.
func CheckLen(n, max int) error {
	if n < 0 || n > max || n > math.MaxUint32 {
		return ErrTooLarge
	}
	return nil
}
