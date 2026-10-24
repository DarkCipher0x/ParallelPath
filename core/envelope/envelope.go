// Package envelope is the shape of every piece of family content: a message, a reaction, a calendar event, an
// expense, a file record. Content is end-to-end encrypted and signed; the server only ever sees the header.
//
// One item can have many revisions. Revision 0 is the original. Every later revision (an edit, or a retraction)
// points at the hash of the one before it and MUST carry a reason. Old revisions are never deleted, so the full
// history stays readable to everyone allowed to see the item.
//
// Layout of a revision:
//
//	header      space, item, rev, prev-hash, kind, epoch, author person/device, ref, created  (plaintext)
//	key         a fresh content key, sealed with the space epoch key            (AAD = header)
//	ciphertext  the JSON body, sealed with the content key                      (AAD = header)
//	signature   author device: ML-DSA-87 + Ed25519 over header || H(key) || H(ciphertext)
package envelope

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
)

// PurposeItem is the signature purpose for item revisions.
const PurposeItem = "item"

// Kinds of item. The kind is visible to the server (so it can route and count) but nothing inside is.
const (
	KindMessage  = "message"
	KindReaction = "reaction"
	KindEvent    = "event"
	KindExpense  = "expense"
	KindFile     = "file"
	KindRecord   = "record"
	KindJournal  = "journal"
	KindCheckIn  = "checkin"
	KindSchedule = "schedule" // a parenting-time schedule; with Ref, a parent's answer to one
	KindSwap     = "swap"     // a request to change parenting time; with Ref, the answer to one
	KindCall     = "call"     // a voice or video call: who called, when, how long (the call itself is never stored)
	KindInfo     = "info"     // an info bank entry or child (earlier test builds stored them as "record" items)
)

// Limits.
const (
	MaxBody   = 256 << 10 // 256 KiB of JSON per revision (files go in blobs, not bodies)
	MaxReason = 2000      // characters
)

// Item is one revision of one item, as stored and sent. Everything except Ciphertext is visible to the server.
type Item struct {
	SpaceID    string    `json:"space_id"`
	ItemID     string    `json:"item_id"`
	Rev        uint64    `json:"rev"`
	Prev       []byte    `json:"prev,omitempty"` // hash of revision Rev-1 (empty for rev 0)
	Kind       string    `json:"kind"`
	Ref        string    `json:"ref,omitempty"` // the item this relates to (a reaction's message, a reply's parent)
	Epoch      uint64    `json:"epoch"`
	Person     string    `json:"person"`
	Device     string    `json:"device"`
	Created    time.Time `json:"created"`
	Key        []byte    `json:"key"`
	Ciphertext []byte    `json:"ciphertext"`
	Signature  []byte    `json:"signature"`
}

// Body is the decrypted content. Which fields are used depends on the kind; unknown kinds carry Data.
type Body struct {
	Text        string          `json:"text,omitempty"`
	Emoji       string          `json:"emoji,omitempty"`  // reactions
	Remove      bool            `json:"remove,omitempty"` // reaction taken back
	Attachments []Attachment    `json:"attachments,omitempty"`
	Reason      string          `json:"reason,omitempty"`    // required on every revision after the first
	Retracted   bool            `json:"retracted,omitempty"` // a retraction: content withdrawn, history kept
	Data        json.RawMessage `json:"data,omitempty"`      // structured content of other kinds
}

// Attachment points at an encrypted blob. The blob key travels only inside the encrypted body.
type Attachment struct {
	BlobID string `json:"blob_id"`
	Name   string `json:"name"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
	Key    []byte `json:"key"`
	SHA3   []byte `json:"sha3"`            // SHA3-512 of the plaintext file
	Width  int    `json:"width,omitempty"` // images and video
	Height int    `json:"height,omitempty"`
	Millis int64  `json:"millis,omitempty"` // audio and video duration
	Thumb  []byte `json:"thumb,omitempty"`  // small JPEG preview, encrypted along with the body
}

var (
	ErrNoReason    = errors.New("envelope: a reason is required to change an item")
	ErrBadItem     = errors.New("envelope: invalid item")
	ErrNotAuthor   = errors.New("envelope: only the original author can revise an item")
	ErrBrokenChain = errors.New("envelope: revision doesn't follow the previous one")
)

func (it *Item) header() []byte {
	return wire.NewWriter("pp/item/v1").
		String(it.SpaceID).String(it.ItemID).Uint(it.Rev).Raw(it.Prev).String(it.Kind).String(it.Ref).
		Uint(it.Epoch).String(it.Person).String(it.Device).Time(it.Created).Bytes()
}

func (it *Item) signedBytes() []byte {
	hk, hc := pqc.Hash(it.Key), pqc.Hash(it.Ciphertext)
	return wire.NewWriter("pp/item-sig/v1").Raw(it.header()).Raw(hk[:]).Raw(hc[:]).Bytes()
}

// Hash identifies this exact revision (header, ciphertext and signature). The next revision's Prev is this.
func (it *Item) Hash() [64]byte {
	return pqc.Hash(it.signedBytes(), it.Signature)
}

// Draft describes a revision to create.
type Draft struct {
	SpaceID string
	ItemID  string // empty for a new item (a fresh ID is assigned)
	Kind    string
	Ref     string
	Body    Body
	Prev    *Item // the current latest revision when editing or retracting; nil for a new item
}

// Seal encrypts and signs a new revision with the author's device keys and the current epoch key.
func Seal(dev *keys.Device, person string, epoch uint64, epochKey []byte, d Draft, now time.Time) (*Item, error) {
	if err := wire.CheckID(d.SpaceID); err != nil {
		return nil, err
	}
	if d.Prev != nil {
		d.Kind = d.Prev.Kind // a revision always keeps the item's kind
	}
	if !validKind(d.Kind) {
		return nil, fmt.Errorf("envelope: unknown kind %q", d.Kind)
	}
	it := &Item{SpaceID: d.SpaceID, ItemID: d.ItemID, Kind: d.Kind, Ref: d.Ref, Epoch: epoch,
		Person: person, Device: dev.DeviceID(), Created: wire.Ms(now)}
	if d.Prev == nil {
		if it.ItemID == "" {
			it.ItemID = wire.NewID()
		}
		if d.Body.Reason != "" || d.Body.Retracted {
			return nil, errors.New("envelope: a new item can't be a retraction")
		}
	} else {
		if d.Prev.ItemID != it.ItemID && it.ItemID != "" || d.Prev.SpaceID != d.SpaceID {
			return nil, ErrBrokenChain
		}
		if d.Prev.Person != person {
			return nil, ErrNotAuthor
		}
		if d.Body.Reason == "" {
			return nil, ErrNoReason
		}
		h := d.Prev.Hash()
		it.ItemID, it.Rev, it.Prev, it.Kind, it.Ref = d.Prev.ItemID, d.Prev.Rev+1, h[:], d.Prev.Kind, d.Prev.Ref
	}
	if err := checkBody(it.Rev, &d.Body); err != nil {
		return nil, err
	}
	if it.Ref != "" {
		if err := wire.CheckID(it.Ref); err != nil {
			return nil, err
		}
	}
	plain, err := json.Marshal(d.Body)
	if err != nil {
		return nil, err
	}
	defer pqc.Wipe(plain)
	if err := wire.CheckLen(len(plain), MaxBody); err != nil {
		return nil, err
	}
	ck := pqc.NewKey()
	defer pqc.Wipe(ck)
	h := it.header()
	if it.Key, err = pqc.Seal(epochKey, append([]byte("key|"), h...), ck); err != nil {
		return nil, err
	}
	if it.Ciphertext, err = pqc.Seal(ck, append([]byte("body|"), h...), plain); err != nil {
		return nil, err
	}
	if it.Signature, err = dev.Sign.Sign(PurposeItem, it.signedBytes()); err != nil {
		return nil, err
	}
	return it, nil
}

// Open verifies the author's signature and decrypts the body. author must be the verified certificate of
// it.Device, and it must belong to it.Person.
func Open(it *Item, epochKey []byte, author *keys.VerifiedDevice) (*Body, error) {
	if err := it.CheckShape(); err != nil {
		return nil, err
	}
	if author == nil || author.DeviceID != it.Device || author.PersonID != it.Person {
		return nil, ErrBadItem
	}
	if author.Sign.Verify(PurposeItem, it.signedBytes(), it.Signature) != nil {
		return nil, ErrBadItem
	}
	h := it.header()
	ck, err := pqc.Open(epochKey, append([]byte("key|"), h...), it.Key)
	if err != nil {
		return nil, err
	}
	defer pqc.Wipe(ck)
	plain, err := pqc.Open(ck, append([]byte("body|"), h...), it.Ciphertext)
	if err != nil {
		return nil, err
	}
	defer pqc.Wipe(plain)
	var b Body
	if err := json.Unmarshal(plain, &b); err != nil {
		return nil, ErrBadItem
	}
	if err := checkBody(it.Rev, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// VerifySignature checks the author's signature without decrypting (what the server does before storing).
func VerifySignature(it *Item, author *keys.VerifiedDevice) error {
	if err := it.CheckShape(); err != nil {
		return err
	}
	if author == nil || author.DeviceID != it.Device || author.PersonID != it.Person {
		return ErrBadItem
	}
	if author.Sign.Verify(PurposeItem, it.signedBytes(), it.Signature) != nil {
		return ErrBadItem
	}
	return nil
}

// CheckShape validates everything the server can check without keys: IDs, kind, sizes, chain fields.
func (it *Item) CheckShape() error {
	if wire.CheckID(it.SpaceID) != nil || wire.CheckID(it.ItemID) != nil || !validKind(it.Kind) {
		return ErrBadItem
	}
	if it.Ref != "" && wire.CheckID(it.Ref) != nil {
		return ErrBadItem
	}
	if (it.Rev == 0) != (len(it.Prev) == 0) || (it.Rev > 0 && len(it.Prev) != 64) {
		return ErrBrokenChain
	}
	if len(it.Person) != 32 || len(it.Device) != 32 || len(it.Signature) != pqc.SignatureSize {
		return ErrBadItem
	}
	if len(it.Key) != pqc.SealedSize(pqc.KeySize) || wire.CheckLen(len(it.Ciphertext), pqc.SealedSize(MaxBody)) != nil {
		return ErrBadItem
	}
	return nil
}

// Follows checks that next is a valid next revision of prev (same item, same author person, hash-linked).
func Follows(prev, next *Item) error {
	h := prev.Hash()
	if next.SpaceID != prev.SpaceID || next.ItemID != prev.ItemID || next.Rev != prev.Rev+1 ||
		!pqc.Equal(next.Prev, h[:]) || next.Kind != prev.Kind || next.Ref != prev.Ref {
		return ErrBrokenChain
	}
	if next.Person != prev.Person {
		return ErrNotAuthor
	}
	return nil
}

func checkBody(rev uint64, b *Body) error {
	if rev > 0 && len([]rune(b.Reason)) == 0 {
		return ErrNoReason
	}
	if len([]rune(b.Reason)) > MaxReason {
		return wire.ErrTooLarge
	}
	if rev == 0 && b.Retracted {
		return ErrBadItem
	}
	if b.Emoji != "" && (len(b.Emoji) > 32) {
		return ErrBadItem
	}
	for _, a := range b.Attachments {
		if wire.CheckID(a.BlobID) != nil || len(a.Key) != pqc.KeySize || len(a.SHA3) != 64 || a.Size < 0 {
			return ErrBadItem
		}
	}
	return nil
}

func validKind(k string) bool {
	switch k {
	case KindMessage, KindReaction, KindEvent, KindExpense, KindFile, KindRecord, KindJournal, KindCheckIn, KindSchedule, KindSwap,
		KindCall, KindInfo:
		return true
	}
	return false
}
