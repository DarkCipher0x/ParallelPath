// Package audit is the tamper-evident record of everything that happens in a family space (and, in a separate
// chain, everything administrators do to the server).
//
// Each entry is signed twice:
//   - by the acting device ("I did this, to this exact item revision, for this reason"), so the server can't
//     invent actions, and
//   - by the server, after it assigns the sequence number, time and previous-entry hash, so the order is fixed.
//
// Entries form a hash chain: Hash_n = SHA3-512(canonical(entry_n)), and entry_n.Prev = Hash_(n-1). Any client can
// re-verify the whole chain; a dropped, reordered or altered entry shows up at the exact place it happened.
// Clients remember the last head they've seen, so a server that rewinds or forks the log is caught too.
package audit

import (
	"errors"
	"fmt"
	"time"

	"pp/core/pqc"
	"pp/core/wire"
)

// Signature purposes.
const (
	PurposeIntent = "audit-intent"
	PurposeEntry  = "audit-entry"
)

// Actions. The action names are shown to people in the audit viewer, so they stay stable.
const (
	ItemCreate    = "item.create"   // new message, event, expense...
	ItemRevise    = "item.revise"   // edit or retraction (the reason is in the encrypted revision)
	ItemView      = "item.view"     // first opened by this person (read receipt)
	BlobUpload    = "blob.upload"   // an encrypted file stored
	BlobDownload  = "blob.download" // an encrypted file fetched
	MemberInvite  = "member.invite"
	MemberJoin    = "member.join"
	MemberRemove  = "member.remove"
	MemberRole    = "member.role"
	MemberPerms   = "member.permissions" // a parent changed what someone they invited may do
	DeviceAdd     = "device.add"
	DeviceRevoke  = "device.revoke"
	EpochRotate   = "epoch.rotate"
	KeysGrant     = "keys.grant" // space keys shared with a member's device
	SpaceCreate   = "space.create"
	SpaceRename   = "space.rename"
	SpaceSettings = "space.settings" // a parent changed a family setting (e.g. children may chat)
	ReportExport  = "report.export"
	CallStart     = "call.start"
	CallEnd       = "call.end"
	SignIn        = "auth.signin"
	AdminAction   = "admin.action"
)

// Intent is what a device asks to record. It's signed by that device.
type Intent struct {
	SpaceID    string    `json:"space_id"` // empty for the server (admin) chain
	Action     string    `json:"action"`
	Person     string    `json:"person"`
	Device     string    `json:"device"`
	ItemID     string    `json:"item_id,omitempty"`
	ItemRev    uint64    `json:"item_rev,omitempty"`
	ItemHash   []byte    `json:"item_hash,omitempty"` // exact revision acted on
	Detail     string    `json:"detail,omitempty"`    // short, never sensitive (e.g. "kind=message")
	ClientTime time.Time `json:"client_time"`
	Signature  []byte    `json:"signature"`
}

// Entry is a recorded intent, sequenced and countersigned by the server.
type Entry struct {
	Intent
	Seq       uint64    `json:"seq"`
	Prev      []byte    `json:"prev"`
	Time      time.Time `json:"time"`
	ServerSig []byte    `json:"server_sig"`
}

const maxDetail = 200

func (i *Intent) signedBytes() []byte {
	return wire.NewWriter("pp/audit-intent/v1").
		String(i.SpaceID).String(i.Action).String(i.Person).String(i.Device).String(i.ItemID).Uint(i.ItemRev).
		Raw(i.ItemHash).String(i.Detail).Time(i.ClientTime).Bytes()
}

// SignIntent creates a signed intent.
func SignIntent(key *pqc.SigningKey, i Intent) (*Intent, error) {
	if err := i.check(); err != nil {
		return nil, err
	}
	i.ClientTime = wire.Ms(i.ClientTime)
	sig, err := key.Sign(PurposeIntent, i.signedBytes())
	if err != nil {
		return nil, err
	}
	i.Signature = sig
	return &i, nil
}

// VerifyIntent checks a device's signature on its intent.
func (i *Intent) VerifyIntent(device pqc.VerifyKey) error {
	if err := i.check(); err != nil {
		return err
	}
	if wire.Fingerprint(device.ID()) != i.Device {
		return ErrBadEntry
	}
	if device.Verify(PurposeIntent, i.signedBytes(), i.Signature) != nil {
		return ErrBadEntry
	}
	return nil
}

func (i *Intent) check() error {
	if i.SpaceID != "" && wire.CheckID(i.SpaceID) != nil {
		return ErrBadEntry
	}
	if i.ItemID != "" && wire.CheckID(i.ItemID) != nil {
		return ErrBadEntry
	}
	if i.Action == "" || len(i.Action) > 32 || len(i.Detail) > maxDetail || len(i.Person) != 32 || len(i.Device) != 32 {
		return ErrBadEntry
	}
	if len(i.ItemHash) != 0 && len(i.ItemHash) != 64 {
		return ErrBadEntry
	}
	return nil
}

func (e *Entry) entryBytes() []byte {
	return wire.NewWriter("pp/audit-entry/v1").
		Raw(e.Intent.signedBytes()).Raw(e.Intent.Signature).Uint(e.Seq).Raw(e.Prev).Time(e.Time).Bytes()
}

// Hash is the chain hash of this entry (what the next entry's Prev must be).
func (e *Entry) Hash() [64]byte { return pqc.Hash(e.entryBytes()) }

// Genesis is the Prev of the first entry in a chain: bound to the chain's identity.
func Genesis(spaceID string) []byte {
	h := pqc.Hash([]byte("pp/audit-genesis/v1|" + spaceID))
	return h[:]
}

// Append sequences an intent after the given head (nil = empty chain) and countersigns it with the server key.
func Append(server *pqc.SigningKey, head *Entry, in Intent, now time.Time) (*Entry, error) {
	e := &Entry{Intent: in, Time: wire.Ms(now)}
	if head == nil {
		e.Seq, e.Prev = 1, Genesis(in.SpaceID)
	} else {
		if head.SpaceID != in.SpaceID {
			return nil, errors.New("audit: entry belongs to another chain")
		}
		h := head.Hash()
		e.Seq, e.Prev = head.Seq+1, h[:]
		if e.Time.Before(head.Time) {
			e.Time = head.Time // never go backwards, even if the clock does
		}
	}
	h := e.Hash()
	sig, err := server.Sign(PurposeEntry, h[:])
	if err != nil {
		return nil, err
	}
	e.ServerSig = sig
	return e, nil
}

// ErrBadEntry means an entry or intent is malformed or a signature is wrong.
var ErrBadEntry = errors.New("audit: invalid entry")

// DeviceKeys resolves a device ID to its verified signing key (from verified device certificates).
type DeviceKeys func(deviceID string) (pqc.VerifyKey, bool)

// Problem describes where a chain fails verification.
type Problem struct {
	Seq    uint64
	Reason string
}

func (p *Problem) Error() string { return fmt.Sprintf("audit: entry %d: %s", p.Seq, p.Reason) }

// Verify checks a chain from its first entry: sequence, links, times, both signatures. If knownHead is given (the
// last head this client saw), the chain must contain it at the same position, or the log has been rewritten.
func Verify(space string, entries []Entry, server pqc.VerifyKey, devices DeviceKeys, knownHead *Head) (*Head, error) {
	prev := Genesis(space)
	var last time.Time
	var head *Head
	for i := range entries {
		e := &entries[i]
		want := uint64(i + 1)
		switch {
		case e.SpaceID != space:
			return head, &Problem{e.Seq, "belongs to another chain"}
		case e.Seq != want:
			return head, &Problem{want, "missing or out of order"}
		case !pqc.Equal(e.Prev, prev):
			return head, &Problem{e.Seq, "doesn't link to the previous entry"}
		case e.Time.Before(last):
			return head, &Problem{e.Seq, "time goes backwards"}
		}
		if why := signerProblem(space, e, server, devices); why != "" {
			return head, &Problem{e.Seq, why}
		}
		h := e.Hash()
		if server.Verify(PurposeEntry, h[:], e.ServerSig) != nil {
			return head, &Problem{e.Seq, "server signature invalid"}
		}
		if knownHead != nil && e.Seq == knownHead.Seq && !pqc.Equal(h[:], knownHead.Hash) {
			return head, &Problem{e.Seq, "differs from the copy seen before: the log was rewritten"}
		}
		prev, last = h[:], e.Time
		head = &Head{Seq: e.Seq, Hash: h[:], Time: e.Time}
	}
	if knownHead != nil && (head == nil || head.Seq < knownHead.Seq) {
		return head, &Problem{knownHead.Seq, "entries seen before are missing: the log was cut short"}
	}
	return head, nil
}

// Head is a chain position a client remembers (and that reports quote, so they can be checked later).
type Head struct {
	Seq  uint64    `json:"seq"`
	Hash []byte    `json:"hash"`
	Time time.Time `json:"time,omitempty"`
}

// VerifyFrom continues verification from a head this client already verified. entries must start AT that head
// (the server is asked from head.Seq inclusive), so the first entry must hash to exactly what was seen before:
// that proves nothing up to there was rewritten, and the rest of the chain is checked as in Verify.
// A nil head verifies from the start of the chain.
func VerifyFrom(space string, head *Head, entries []Entry, server pqc.VerifyKey, devices DeviceKeys) (*Head, error) {
	if head == nil {
		return Verify(space, entries, server, devices, nil)
	}
	if len(entries) == 0 || entries[0].Seq != head.Seq {
		return head, &Problem{head.Seq, "entries seen before are missing: the log was cut short"}
	}
	first := entries[0].Hash()
	if !pqc.Equal(first[:], head.Hash) || entries[0].SpaceID != space {
		return head, &Problem{head.Seq, "differs from the copy seen before: the log was rewritten"}
	}
	prev, last, cur := first[:], entries[0].Time, head
	for i := 1; i < len(entries); i++ {
		e := &entries[i]
		switch {
		case e.SpaceID != space:
			return cur, &Problem{e.Seq, "belongs to another chain"}
		case e.Seq != head.Seq+uint64(i):
			return cur, &Problem{head.Seq + uint64(i), "missing or out of order"}
		case !pqc.Equal(e.Prev, prev):
			return cur, &Problem{e.Seq, "doesn't link to the previous entry"}
		case e.Time.Before(last):
			return cur, &Problem{e.Seq, "time goes backwards"}
		}
		if why := signerProblem(space, e, server, devices); why != "" {
			return cur, &Problem{e.Seq, why}
		}
		h := e.Hash()
		if server.Verify(PurposeEntry, h[:], e.ServerSig) != nil {
			return cur, &Problem{e.Seq, "server signature invalid"}
		}
		prev, last = h[:], e.Time
		cur = &Head{Seq: e.Seq, Hash: h[:], Time: e.Time}
	}
	return cur, nil
}

// Operator stands for the server's operator in the server chain (space ""). Operator entries are signed by the
// server's own key - administrators act on the machine, not through a device - and are only valid in that chain.
const Operator = "00000000000000000000000000000000"

// signerProblem checks who signed an intent: a person's device, or (server chain only) the operator via the
// server key. It returns why it fails, or "".
func signerProblem(space string, e *Entry, server pqc.VerifyKey, devices DeviceKeys) string {
	if e.Device == Operator {
		if space != "" {
			return "operator entry outside the server chain"
		}
		if e.check() != nil || server.Verify(PurposeIntent, e.signedBytes(), e.Signature) != nil {
			return "operator signature invalid"
		}
		return ""
	}
	dk, ok := devices(e.Device)
	if !ok {
		return "unknown device"
	}
	if e.VerifyIntent(dk) != nil {
		return "device signature invalid"
	}
	return ""
}
