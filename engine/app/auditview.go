package app

import (
	"strings"
	"time"

	"pp/core/audit"
	"pp/engine/vault"
)

// AuditRow is one line of the audit log, in words.
type AuditRow struct {
	Seq        uint64    `json:"seq"`
	Time       time.Time `json:"time"`        // server time (what the chain orders by)
	ClientTime time.Time `json:"client_time"` // the device's own clock at the time
	Person     string    `json:"person"`
	Name       string    `json:"name"`
	Device     string    `json:"device"`
	Action     string    `json:"action"`
	What       string    `json:"what"` // human description
	ItemID     string    `json:"item_id,omitempty"`
	ItemRev    uint64    `json:"item_rev,omitempty"`
}

// AuditLog is a space's verified audit log.
type AuditLog struct {
	Rows     []AuditRow `json:"rows"`
	Verified bool       `json:"verified"`
	HeadSeq  uint64     `json:"head_seq"`
	HeadHash string     `json:"head_hash"` // quoted in reports so they can be checked against the chain later
	Problem  string     `json:"problem,omitempty"`
}

// Audit returns the audit log as verified on this device (newest first).
func (e *Engine) Audit(space string) (*AuditLog, error) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return nil, errLocked
	}
	all, _ := e.loadDecoded(space)
	names := e.names(all)
	log := &AuditLog{Rows: []AuditRow{}}
	var h audit.Head
	if v.Get(vault.State, "audit-head/"+space, &h) == nil {
		log.Verified, log.HeadSeq = true, h.Seq
		log.HeadHash = hexs(h.Hash)
	}
	var problem string
	if v.Get(vault.State, "audit-problem/"+space, &problem) == nil {
		log.Verified, log.Problem = false, problem
	}
	e.eachAudit(space, func(en *audit.Entry) {
		log.Rows = append(log.Rows, AuditRow{Seq: en.Seq, Time: en.Time, ClientTime: en.ClientTime, Person: en.Person,
			Name: names.of(en.Person), Device: en.Device, Action: en.Action, What: describe(en, names), ItemID: en.ItemID, ItemRev: en.ItemRev})
	})
	for i, j := 0, len(log.Rows)-1; i < j; i, j = i+1, j-1 {
		log.Rows[i], log.Rows[j] = log.Rows[j], log.Rows[i]
	}
	return log, nil
}

func describe(en *audit.Entry, n nameBook) string {
	kind := strings.TrimPrefix(en.Detail, "kind=")
	noun := map[string]string{"message": "a message", "reaction": "a reaction", "record": "a name or info-bank entry", "event": "an event",
		"expense": "an expense", "file": "a document", "journal": "a journal entry", "checkin": "a check-in", "call": "a call record", "schedule": "a parenting schedule",
		"swap": "a parenting-time change request"}[kind]
	if noun == "" {
		noun = "an item"
	}
	switch en.Action {
	case audit.ItemCreate:
		if kind == "reaction" {
			return "reacted to a message"
		}
		return "added " + noun
	case audit.ItemRevise:
		return "changed " + noun + " (revision " + itoa(en.ItemRev) + "; the reason is kept in its history)"
	case audit.ItemView:
		return "opened a message for the first time"
	case audit.BlobUpload:
		return "uploaded an encrypted file"
	case audit.BlobDownload:
		return "downloaded a file"
	case audit.MemberInvite:
		return "created an invitation (" + strings.TrimPrefix(en.Detail, "role=") + ")"
	case audit.MemberJoin:
		return "joined as " + strings.TrimPrefix(en.Detail, "role=")
	case audit.MemberRemove:
		return "removed " + n.of(strings.TrimPrefix(en.Detail, "person="))
	case audit.KeysGrant:
		return "approved " + n.of(strings.TrimPrefix(en.Detail, "person=")) + " and shared the family's keys"
	case audit.EpochRotate:
		return "rotated the family's encryption keys"
	case audit.SpaceCreate:
		return "created the family space"
	case audit.ReportExport:
		return "exported a report"
	case audit.CallStart:
		return "started a call"
	case audit.CallEnd:
		return "ended a call"
	}
	return en.Action
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func hexs(b []byte) string {
	const hx = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2], out[i*2+1] = hx[c>>4], hx[c&15]
	}
	return string(out)
}
