package app

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pp/core/audit"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
	"pp/engine/report"
)

// Court-ready reports. A report is made on this device from the decrypted, verified family record: a signed JSON
// file (the evidence: every item exactly as recorded, with edit and withdrawal history) and a PDF of the same for
// people to read. The JSON is signed with this person's identity key (ML-DSA-87 + Ed25519) and quotes the head of
// the family's audit chain, so anyone can check the report hasn't been altered since and match it against the
// chain later. Making a report is itself recorded in the family's audit log.

const reportFormat = "parallelpath-report/1"

// ReportInput chooses what goes in a report.
type ReportInput struct {
	Space    string   `json:"space"`
	From     string   `json:"from"` // yyyy-mm-dd
	To       string   `json:"to"`
	Zone     string   `json:"zone"`
	Sections []string `json:"sections"` // messages, calendar, expenses, documents, info, audit, journal
}

// ReportData is the signed content.
type ReportData struct {
	Format    string          `json:"format"`
	ID        string          `json:"id"`
	Family    string          `json:"family"`
	FamilyID  string          `json:"family_id"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	Zone      string          `json:"zone"`
	Generated time.Time       `json:"generated"`
	By        ReportPerson    `json:"by"`
	Members   []ReportPerson  `json:"members"`
	AuditHead ReportAuditHead `json:"audit_head"`
	Messages  []Message       `json:"messages,omitempty"`
	// Conversations: the author's private conversations in the family (section "conversations").
	Conversations []ReportConversation `json:"conversations,omitempty"`
	Calendar      *CalendarView        `json:"calendar,omitempty"`
	Expenses      *ExpensesView        `json:"expenses,omitempty"`
	Documents     []Doc                `json:"documents,omitempty"`
	Info          *InfoView            `json:"info,omitempty"`
	Audit         []AuditRow           `json:"audit,omitempty"`
	Journal       []JournalEntry       `json:"journal,omitempty"`
}

// ReportPerson is someone named in a report.
type ReportPerson struct {
	Person string `json:"person"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
}

// ReportAuditHead is the family audit chain's head when the report was made.
type ReportAuditHead struct {
	Seq      uint64 `json:"seq"`
	Hash     string `json:"hash"`
	Verified bool   `json:"verified"`
}

// SignedReport is the JSON file: the report exactly as signed, the signature and the signer's public key.
type SignedReport struct {
	Report    json.RawMessage `json:"report"`
	Signature []byte          `json:"signature"`
	Signer    []byte          `json:"signer"` // the identity public key (its fingerprint is the person ID)
}

// ReportConversation is one private conversation in a report.
type ReportConversation struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"` // the other people in it
	Messages []Message `json:"messages"`
}

// ReportFiles are where the report was written (this device's private cache, wiped when the app locks).
type ReportFiles struct {
	ID     string `json:"id"`
	PDF    string `json:"pdf"`
	JSON   string `json:"json"`
	CSV    string `json:"csv"`    // a spreadsheet copy (convenience; the signed JSON is the evidence)
	SHA3   string `json:"sha3"`   // SHA3-512 of the signed report
	Signer string `json:"signer"` // the person ID (identity key fingerprint)
}

const purposeReport = "report"

var reportSections = map[string]bool{"messages": true, "conversations": true, "calendar": true, "expenses": true, "documents": true, "info": true,
	"audit": true, "journal": true}

// Report makes a court-ready report.
func (e *Engine) Report(in ReportInput) (*ReportFiles, error) {
	loc, err := time.LoadLocation(in.Zone)
	if err != nil {
		return nil, errors.New("unknown time zone")
	}
	from, err1 := time.ParseInLocation("2006-01-02", in.From, loc)
	to, err2 := time.ParseInLocation("2006-01-02", in.To, loc)
	if err1 != nil || err2 != nil || to.Before(from) {
		return nil, errors.New("choose the dates the report covers")
	}
	end := to.AddDate(0, 0, 1)
	want := map[string]bool{}
	for _, s := range in.Sections {
		if !reportSections[s] {
			return nil, fmt.Errorf("unknown report section %q", s)
		}
		want[s] = true
	}
	if len(want) == 0 {
		return nil, errors.New("choose what to include")
	}
	e.syncSpace(in.Space) // the latest items and audit entries, so the quoted chain head is current
	sv, err := e.spaceView(in.Space)
	if err != nil {
		return nil, err
	}
	all, err := e.loadDecoded(in.Space)
	if err != nil {
		return nil, err
	}
	names := e.names(all)
	e.mu.Lock()
	ident, person := e.ident, e.person
	e.mu.Unlock()
	if ident == nil {
		return nil, errLocked
	}
	inRange := func(t time.Time) bool { return !t.Before(from) && t.Before(end) }

	d := ReportData{Format: reportFormat, ID: wire.NewID(), Family: names.space, FamilyID: in.Space, From: in.From, To: in.To, Zone: in.Zone,
		Generated: time.Now().UTC(), By: ReportPerson{Person: person, Name: names.of(person)}}
	for _, m := range sv.Members {
		d.Members = append(d.Members, ReportPerson{Person: m.Person, Name: names.of(m.Person), Role: m.Role})
	}
	log, err := e.Audit(in.Space)
	if err != nil {
		return nil, err
	}
	d.AuditHead = ReportAuditHead{Seq: log.HeadSeq, Hash: log.HeadHash, Verified: log.Verified}

	if want["messages"] {
		ms, err := e.Messages(in.Space)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			if inRange(m.Sent) {
				m.Attachments = stripThumbs(m.Attachments)
				d.Messages = append(d.Messages, m)
			}
		}
	}
	if want["conversations"] {
		cs, err := e.Conversations(in.Space)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			ms, err := e.Messages(c.ID)
			if err != nil {
				continue
			}
			rc := ReportConversation{ID: c.ID, Title: c.Title}
			for _, m := range ms {
				if inRange(m.Sent) {
					m.Attachments = stripThumbs(m.Attachments)
					rc.Messages = append(rc.Messages, m)
				}
			}
			if len(rc.Messages) > 0 {
				d.Conversations = append(d.Conversations, rc)
			}
		}
	}
	if want["calendar"] {
		if d.Calendar, err = e.Calendar(in.Space, in.From, in.To, in.Zone); err != nil {
			return nil, err
		}
	}
	if want["expenses"] {
		v, err := e.Expenses(in.Space)
		if err != nil {
			return nil, err
		}
		kept := v.Expenses[:0]
		for _, x := range v.Expenses {
			if day, err := time.ParseInLocation("2006-01-02", x.Data.Date, loc); err == nil && inRange(day) {
				x.Receipts = stripThumbs(x.Receipts)
				kept = append(kept, x)
			}
		}
		v.Expenses = kept
		d.Expenses = v
	}
	if want["documents"] {
		docs, err := e.Documents(in.Space)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			if doc.Type == "file" && !doc.Implied {
				if doc.File != nil {
					f := *doc.File
					f.Thumb = nil
					doc.File = &f
				}
				for i := range doc.Versions {
					doc.Versions[i].File.Thumb = nil
				}
				d.Documents = append(d.Documents, doc)
			}
		}
	}
	if want["info"] {
		if d.Info, err = e.InfoBank(in.Space); err != nil {
			return nil, err
		}
	}
	if want["audit"] {
		for _, r := range log.Rows {
			if inRange(r.Time) {
				d.Audit = append(d.Audit, r)
			}
		}
		sort.Slice(d.Audit, func(i, j int) bool { return d.Audit[i].Seq < d.Audit[j].Seq })
	}
	if want["journal"] {
		js, err := e.Journal(in.Space)
		if err != nil {
			return nil, err
		}
		for _, j := range js {
			if day, err := time.ParseInLocation("2006-01-02", j.Data.Date, loc); err == nil && inRange(day) && !j.Removed {
				j.Attachments = stripThumbs(j.Attachments)
				d.Journal = append(d.Journal, j)
			}
		}
	}

	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	sig, err := ident.Key.Sign(purposeReport, raw)
	if err != nil {
		return nil, err
	}
	// Compact on purpose: indenting would re-format the embedded report and it would no longer match its signature.
	signed, _ := json.Marshal(SignedReport{Report: raw, Signature: sig, Signer: ident.Key.Public().Bytes()})
	sum := pqc.Hash(raw)
	files := &ReportFiles{ID: d.ID, SHA3: hex.EncodeToString(sum[:]), Signer: person}

	dir := filepath.Join(e.cacheDir(), randHex(8))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	base := "ParallelPath report " + in.From + " to " + in.To
	files.JSON = filepath.Join(dir, base+".json")
	files.PDF = filepath.Join(dir, base+".pdf")
	if err := os.WriteFile(files.JSON, signed, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(files.PDF, renderReport(&d, loc, files, hex.EncodeToString(sig[:32])), 0o600); err != nil {
		return nil, err
	}
	files.CSV = filepath.Join(dir, base+".csv")
	if err := os.WriteFile(files.CSV, reportCSV(&d, loc), 0o600); err != nil {
		return nil, err
	}
	// The family's audit log records that a report was made (by whom and when; not what's in it).
	if intent, err := e.intent(audit.Intent{SpaceID: in.Space, Action: audit.ReportExport, ItemID: d.ID, Detail: "range=" + in.From + ".." + in.To}); err == nil {
		_ = e.call("POST", "/api/v1/spaces/"+in.Space+"/record", map[string]any{"intent": intent}, nil)
	}
	return files, nil
}

func stripThumbs(as []AttachmentView) []AttachmentView {
	out := make([]AttachmentView, len(as))
	for i, a := range as {
		a.Thumb = nil
		out[i] = a
	}
	return out
}

// ReportCheck is the result of verifying a report file.
type ReportCheck struct {
	Valid     bool      `json:"valid"`
	Problem   string    `json:"problem,omitempty"`
	ID        string    `json:"id,omitempty"`
	Family    string    `json:"family,omitempty"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	ByName    string    `json:"by_name,omitempty"`
	Signer    string    `json:"signer,omitempty"` // person ID = identity key fingerprint
	Generated time.Time `json:"generated,omitempty"`
	AuditSeq  uint64    `json:"audit_seq,omitempty"`
	AuditHash string    `json:"audit_hash,omitempty"`
	SHA3      string    `json:"sha3,omitempty"`
	// ChainMatch: this device is in the family and its verified audit chain has the same entry at that sequence
	// number ("" when this device can't check).
	ChainMatch string `json:"chain_match,omitempty"`
}

// VerifyReport checks a report's JSON file: the signature, who signed it, and (where this device is in the family)
// that its audit-chain head matches the chain as recorded.
func (e *Engine) VerifyReport(path string) (*ReportCheck, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return e.verifyReportBytes(b), nil
}

func (e *Engine) verifyReportBytes(b []byte) *ReportCheck {
	var s SignedReport
	if json.Unmarshal(b, &s) != nil || len(s.Report) == 0 {
		return &ReportCheck{Problem: "this isn't a ParallelPath report file (choose the .json file)"}
	}
	vk, err := pqc.ParseVerifyKey(s.Signer)
	if err != nil {
		return &ReportCheck{Problem: "the report's signer key is damaged"}
	}
	if vk.Verify(purposeReport, s.Report, s.Signature) != nil {
		return &ReportCheck{Problem: "the signature doesn't match: the report has been changed since it was signed"}
	}
	var d ReportData
	if json.Unmarshal(s.Report, &d) != nil || d.Format != reportFormat {
		return &ReportCheck{Problem: "unknown report format"}
	}
	signer := keys.PersonID(vk)
	sum := pqc.Hash(s.Report)
	c := &ReportCheck{Valid: true, ID: d.ID, Family: d.Family, From: d.From, To: d.To, ByName: d.By.Name, Signer: signer,
		Generated: d.Generated, AuditSeq: d.AuditHead.Seq, AuditHash: d.AuditHead.Hash, SHA3: hex.EncodeToString(sum[:])}
	if d.By.Person != signer {
		c.Valid, c.Problem = false, "the report names a different author from the key that signed it"
		return c
	}
	// Where we can, compare the quoted chain head with our own verified copy of the family's chain.
	e.eachAudit(d.FamilyID, func(en *audit.Entry) {
		if en.Seq != d.AuditHead.Seq {
			return
		}
		h := en.Hash()
		if hexs(h[:]) == d.AuditHead.Hash {
			c.ChainMatch = "matches this device's verified copy of the family's audit chain"
		} else {
			c.ChainMatch = "DOES NOT MATCH this device's copy of the family's audit chain"
			c.Valid = false
			c.Problem = "the audit-chain head in the report differs from the family's chain"
		}
	})
	return c
}

func renderReport(d *ReportData, loc *time.Location, files *ReportFiles, sigStart string) []byte {
	p := report.New("ParallelPath report - " + d.Family)
	when := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.In(loc).Format("Mon 2 Jan 2006 15:04")
	}
	day := func(s string) string {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.Format("Mon 2 Jan 2006")
		}
		return s
	}
	p.Title("ParallelPath family record")
	p.KV("Family", d.Family)
	p.KV("Period", day(d.From)+" to "+day(d.To)+" ("+d.Zone+")")
	p.KV("Prepared by", d.By.Name)
	p.KV("Prepared at", when(d.Generated))
	var members []string
	for _, m := range d.Members {
		members = append(members, m.Name+" ("+m.Role+")")
	}
	p.KV("Members", strings.Join(members, ", "))
	p.KV("Report ID", d.ID)
	p.Gap(6)
	p.Para("Everything in this report was end-to-end encrypted with post-quantum cryptography and decrypted only on the " +
		"device that made it. Every message and record is signed by the device of the person who wrote it; nothing can be " +
		"edited or withdrawn without a reason, and the earlier versions are shown below. The same report, signed, is in the " +
		"accompanying .json file - see \"Checking this report\" at the end.")

	renderMessages := func(list []Message) {
		for _, m := range list {
			if m.Call != nil {
				kind := "Voice call"
				if m.Call.Video {
					kind = "Video call"
				}
				length := fmt.Sprintf("%d:%02d", m.Call.Seconds/60, m.Call.Seconds%60)
				if m.Call.Missed {
					length = "missed"
				}
				p.Small(when(m.Sent) + " - " + kind + " from " + m.AuthorName + " - " + length)
				continue
			}
			p.H2(when(m.Sent) + " - " + m.AuthorName)
			switch {
			case m.Locked:
				p.Warn("This message can't be read on this device.")
			case m.Retracted:
				p.Warn("Withdrawn. Reason: " + m.Reason)
			default:
				p.Indent(m.Text)
			}
			for _, a := range m.Attachments {
				p.Small(fmt.Sprintf("Attachment: %s (%s, %d bytes)", a.Name, a.Mime, a.Size))
			}
			if len(m.History) > 1 {
				for _, h := range m.History {
					label := "Original"
					if h.Rev > 0 {
						label = "Changed " + when(h.Time) + " - reason: " + h.Reason
					}
					if h.Retracted {
						label = "Withdrawn " + when(h.Time) + " - reason: " + h.Reason
					}
					p.Small(label + (map[bool]string{true: "", false: ": \"" + h.Text + "\""}[h.Retracted]))
				}
			}
			for _, r := range m.ReadBy {
				p.Small("Read by " + r.Name + " " + when(r.Time))
			}
		}
	}
	if d.Messages != nil {
		p.H1(fmt.Sprintf("Messages (%d)", len(d.Messages)))
		renderMessages(d.Messages)
	}
	for _, c := range d.Conversations {
		p.H1(fmt.Sprintf("Private conversation with %s (%d)", c.Title, len(c.Messages)))
		p.Small("Only " + c.Title + " and " + d.By.Name + " could read this conversation; the rest of the family couldn't.")
		renderMessages(c.Messages)
	}

	if c := d.Calendar; c != nil {
		p.H1("Parenting time and calendar")
		for _, s := range c.Schedules {
			p.H2("Schedule proposal by " + s.AuthorName + " - " + strings.ToUpper(s.State))
			if s.Schedule != nil {
				p.Indent(fmt.Sprintf("%s from %s, handovers at %s", s.Schedule.Template, day(s.Schedule.From), s.Schedule.Pattern.Exchange))
			}
			proposalLines(p, s, when)
		}
		for _, ch := range c.Changes {
			p.H2("Change request by " + ch.AuthorName + " - " + strings.ToUpper(ch.State))
			for _, x := range ch.Changes {
				p.Indent(fmt.Sprintf("With %s from %s to %s", nameIn(c.Parents, x.Parent), when(x.From), when(x.To)))
			}
			proposalLines(p, ch, when)
		}
		if len(c.Days) > 0 {
			p.H2("Who had the children")
			for _, dd := range c.Days {
				line := day(dd.Date) + ": " + nameIn(c.Parents, dd.Start)
				if dd.End != dd.Start {
					line += " then " + nameIn(c.Parents, dd.End)
				}
				for _, h := range dd.Handovers {
					line += fmt.Sprintf(" (handover %s to %s)", h.Time.In(loc).Format("15:04"), nameIn(c.Parents, h.To))
				}
				if dd.Changed {
					line += " - agreed change"
				}
				p.Small(line)
			}
		}
		if len(c.Events) > 0 {
			p.H2("Events")
			for _, ev := range c.Events {
				line := when(ev.Start) + " - " + ev.Data.Title
				if ev.Data.Location != "" {
					line += " at " + ev.Data.Location
				}
				line += " (added by " + ev.AuthorName + ")"
				if ev.Cancelled {
					line += " - CANCELLED: " + ev.Reason
				}
				p.Small(line)
			}
		}
		if len(c.CheckIns) > 0 {
			p.H2("Check-ins")
			for _, ci := range c.CheckIns {
				line := when(ci.Data.At) + " - " + ci.Name
				if ci.Data.Place != "" {
					line += " at " + ci.Data.Place
				}
				if ci.Data.Lat != nil && ci.Data.Lon != nil {
					line += fmt.Sprintf(" (%.5f, %.5f, +/-%.0f m)", *ci.Data.Lat, *ci.Data.Lon, ci.Data.Accuracy)
				}
				line += " - received by the server " + when(ci.Received)
				p.Small(line)
				if ci.Note != "" {
					p.Small("Note: " + ci.Note)
				}
			}
		}
	}

	if x := d.Expenses; x != nil {
		p.H1(fmt.Sprintf("Expenses (%d)", len(x.Expenses)))
		for _, b := range x.Balances {
			p.Para(fmt.Sprintf("Balance: %s owes %s %s", b.FromName, b.ToName, money(b.Amount, b.Currency)))
		}
		for _, ex := range x.Expenses {
			p.H2(fmt.Sprintf("%s - %s - %s", day(ex.Data.Date), ex.Data.Title, money(ex.Data.Amount, ex.Data.Currency)))
			p.Indent(fmt.Sprintf("Paid by %s; %d%% (%s) asked of the other parent. Status: %s.", ex.AuthorName, ex.Data.Share,
				money(ex.Owed, ex.Data.Currency), ex.State))
			if ex.Note != "" {
				p.Small("Note: " + ex.Note)
			}
			for _, a := range ex.Answers {
				line := when(a.Time) + " - " + a.Name + ": " + a.Answer
				if a.Method != "" {
					line += " by " + a.Method
				}
				if a.Reference != "" {
					line += " (ref " + a.Reference + ")"
				}
				if a.Comment != "" {
					line += " - \"" + a.Comment + "\""
				}
				if !a.Current {
					line += " (to an earlier version)"
				}
				p.Small(line)
			}
			for _, r := range ex.Receipts {
				p.Small("Receipt: " + r.Name)
			}
		}
	}

	if d.Documents != nil {
		p.H1(fmt.Sprintf("Documents (%d)", len(d.Documents)))
		for _, doc := range d.Documents {
			status := ""
			if doc.Removed {
				status = " - REMOVED: " + doc.Reason
			}
			p.H2(doc.Path + status)
			p.Small(fmt.Sprintf("Added by %s %s; %d version(s)", doc.AuthorName, when(doc.Created), len(doc.Versions)))
			for i, v := range doc.Versions {
				line := fmt.Sprintf("Version %d: %s (%d bytes) %s", i+1, v.File.Name, v.File.Size, when(v.Time))
				if v.Reason != "" {
					line += " - " + v.Reason
				}
				p.Small(line)
			}
			for _, o := range doc.Opened {
				p.Small("Opened by " + o.Name + " " + when(o.Time))
			}
		}
	}

	if in := d.Info; in != nil {
		p.H1("Info bank")
		childName := map[string]string{"": "Whole family"}
		for _, c := range in.Children {
			childName[c.Chain] = c.Label
			line := c.Label
			if c.Value != "" {
				line += " (born " + day(c.Value) + ")"
			}
			p.Para(line)
		}
		for _, en := range in.Entries {
			p.Small(fmt.Sprintf("%s - %s - %s: %s (by %s, %s)", childName[en.Child], en.Section, en.Label, en.Value, en.ByName, when(en.Updated)))
		}
	}

	if d.Journal != nil {
		p.H1(fmt.Sprintf("%s's journal (private notes, %d)", d.By.Name, len(d.Journal)))
		for _, j := range d.Journal {
			p.H2(day(j.Data.Date) + map[bool]string{true: " (edited)", false: ""}[j.Edited])
			p.Indent(j.Text)
			p.Small("Written " + when(j.Written))
		}
	}

	if d.Audit != nil {
		p.H1(fmt.Sprintf("Audit log (%d entries)", len(d.Audit)))
		for _, r := range d.Audit {
			p.Small(fmt.Sprintf("#%d %s - %s %s", r.Seq, when(r.Time), r.Name, r.What))
		}
	}

	p.H1("Checking this report")
	p.Para("This PDF is a readable copy. The evidence is the accompanying .json file, which is signed with " + d.By.Name +
		"'s identity key (ML-DSA-87 and Ed25519). To check it, open it in ParallelPath (Audit log > Verify a report), or on " +
		"the family's server run: ppserver verify-report <file.json>. The check confirms nothing has been changed since it was " +
		"signed, who signed it, and - in ParallelPath - that the audit-chain head below matches the family's chain.")
	p.KV("Signed by", d.By.Name)
	p.KV("Identity fingerprint", files.Signer)
	p.KV("Audit chain head", fmt.Sprintf("entry #%d (verified: %t)", d.AuditHead.Seq, d.AuditHead.Verified))
	p.Mono(d.AuditHead.Hash)
	p.KV("Report SHA3-512", "")
	p.Mono(files.SHA3)
	p.KV("Signature begins", "")
	p.Mono(sigStart)
	return p.Bytes(func(page, total int) string {
		return fmt.Sprintf("ParallelPath report %s  ·  %s  ·  page %d of %d", d.ID, d.Family, page, total)
	})
}

func proposalLines(p *report.PDF, s Proposal, when func(time.Time) string) {
	p.Small("Proposed " + when(s.Created) + map[bool]string{true: "; changed " + when(s.Updated), false: ""}[s.Rev > 0])
	if s.Note != "" {
		p.Small("Note: \"" + s.Note + "\"")
	}
	for _, a := range s.Answers {
		line := when(a.Time) + " - " + a.Name + " " + a.Answer + "ed"
		if a.Comment != "" {
			line += ": \"" + a.Comment + "\""
		}
		p.Small(line)
	}
}

func nameIn(ps []Person, id string) string {
	for _, p := range ps {
		if p.ID == id {
			return p.Name
		}
	}
	if id == "" {
		return "nobody"
	}
	return "someone"
}

func money(minor int64, cur string) string {
	sym := map[string]string{"GBP": "£", "EUR": "€", "USD": "$"}[cur]
	s := fmt.Sprintf("%d.%02d", minor/100, minor%100)
	if sym == "" {
		return s + " " + cur
	}
	return sym + s
}

// VerifyReportJSON checks report bytes without an engine (the server's verify-report command): the signature and
// the signer only.
func VerifyReportJSON(b []byte) *ReportCheck { return (&Engine{}).verifyReportBytes(b) }
