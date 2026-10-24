package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"pp/core/envelope"
	"pp/engine/vault"
)

// The journal is private. It doesn't live in the family space (where the other parent would at least see that an
// entry was added); it lives in a separate space of this person's own, which only their devices hold keys for and
// which the apps never list as a family. Each entry notes which family it's about. Sharing an entry posts a copy
// into the family's messages - the journal itself stays private.

// JournalData is an entry's details (the text is the body's text).
type JournalData struct {
	Family string   `json:"family,omitempty"` // the family space it's about
	Date   string   `json:"date"`             // yyyy-mm-dd: the day it's about
	Mood   string   `json:"mood,omitempty"`   // good | okay | hard
	Tags   []string `json:"tags,omitempty"`
}

// JournalEntry is an entry as shown.
type JournalEntry struct {
	ID          string           `json:"id"`
	Rev         uint64           `json:"rev"`
	Data        JournalData      `json:"data"`
	Text        string           `json:"text"`
	Written     time.Time        `json:"written"`
	Updated     time.Time        `json:"updated"`
	Edited      bool             `json:"edited"`
	Removed     bool             `json:"removed"`
	Reason      string           `json:"reason,omitempty"`
	Attachments []AttachmentView `json:"attachments,omitempty"`
	History     []Revision       `json:"history"`
}

// JournalInput is a new entry, or (with Item and a reason) a change to one.
type JournalInput struct {
	Family string      `json:"family"`
	Item   string      `json:"item,omitempty"`
	Reason string      `json:"reason,omitempty"`
	Text   string      `json:"text"`
	Date   string      `json:"date"`
	Mood   string      `json:"mood,omitempty"`
	Tags   []string    `json:"tags,omitempty"`
	Files  []FileInput `json:"files,omitempty"`
}

const journalMarker = "journal"

// journalSpace finds this person's journal space, creating it the first time if asked.
func (e *Engine) journalSpace(create bool) (string, error) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return "", errLocked
	}
	var id string
	if v.Get(vault.State, "journal-space", &id) == nil && id != "" {
		return id, nil
	}
	var list []spaceView
	_ = v.Get(vault.State, "spaces", &list)
	for _, s := range list {
		if all, err := e.loadDecoded(s.Space.ID); err == nil && e.names(all).journal {
			_ = v.Put(vault.State, "journal-space", s.Space.ID)
			return s.Space.ID, nil
		}
	}
	if !create {
		return "", nil
	}
	e.mu.Lock()
	if e.v == nil || e.srv == nil {
		e.mu.Unlock()
		return "", errLocked
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	id, err := e.createSpaceLocked(ctx, "My journal", true)
	cancel()
	e.mu.Unlock()
	if err != nil {
		return "", err
	}
	_ = v.Put(vault.State, "journal-space", id)
	_ = v.Delete(vault.State, "pending-profile/"+id) // nobody else is there to see a name
	e.syncSpace(id)
	if _, err := e.post(id, envelope.Draft{Kind: envelope.KindRecord, Body: envelope.Body{Data: jsonData(record{Type: journalMarker})}}); err != nil {
		return "", err
	}
	go e.refreshRecovery() // a recovery kit made earlier should now include the journal's keys too
	return id, nil
}

func (in *JournalInput) check() error {
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" && len(in.Files) == 0 {
		return errors.New("write something, or add a photo or file")
	}
	if utf8.RuneCountInString(in.Text) > 20_000 {
		return errors.New("that entry is too long (20,000 characters at most)")
	}
	if _, err := time.Parse("2006-01-02", in.Date); err != nil {
		return errors.New("choose the day it's about")
	}
	switch in.Mood {
	case "", "good", "okay", "hard":
	default:
		return errors.New("unknown mood")
	}
	var tags []string
	for _, t := range in.Tags {
		if t = strings.TrimSpace(t); t != "" && utf8.RuneCountInString(t) <= 40 {
			tags = append(tags, t)
		}
	}
	if len(tags) > 20 {
		return errors.New("20 tags at most")
	}
	in.Tags = tags
	return nil
}

// SaveJournal writes a new entry, or changes one (with a reason; every version is kept).
func (e *Engine) SaveJournal(in JournalInput) (string, error) {
	if err := in.check(); err != nil {
		return "", err
	}
	space, err := e.journalSpace(true)
	if err != nil {
		return "", err
	}
	var atts []envelope.Attachment
	if in.Item != "" {
		cur, err := e.latest(space, in.Item)
		if err != nil {
			return "", err
		}
		if cur.body != nil {
			atts = cur.body.Attachments
		}
	}
	for _, f := range in.Files {
		a, err := e.uploadFile(space, f)
		if err != nil {
			return "", err
		}
		atts = append(atts, *a)
	}
	body := envelope.Body{Text: in.Text, Attachments: atts,
		Data: jsonData(JournalData{Family: in.Family, Date: in.Date, Mood: in.Mood, Tags: in.Tags})}
	if in.Item == "" {
		it, err := e.post(space, envelope.Draft{Kind: envelope.KindJournal, Body: body})
		if err != nil {
			return "", err
		}
		return it.ItemID, nil
	}
	body.Reason = strings.TrimSpace(in.Reason)
	return in.Item, e.reviseKind(space, in.Item, envelope.KindJournal, body)
}

// RemoveJournal removes an entry (its versions stay readable to you, marked removed).
func (e *Engine) RemoveJournal(item, reason string) error {
	space, err := e.journalSpace(false)
	if err != nil || space == "" {
		return errors.New("there's no journal yet")
	}
	return e.reviseKind(space, item, envelope.KindJournal, envelope.Body{Retracted: true, Reason: strings.TrimSpace(reason)})
}

// Journal lists your entries about a family (all families if family is empty), newest day first.
func (e *Engine) Journal(family string) ([]JournalEntry, error) {
	out := []JournalEntry{}
	space, err := e.journalSpace(false)
	if err != nil || space == "" {
		return out, err
	}
	all, err := e.loadDecoded(space)
	if err != nil {
		return nil, err
	}
	threads, order := threadsOf(all, envelope.KindJournal)
	for _, id := range order {
		t := threads[id]
		cur := t.latest()
		var data JournalData
		var live *envelope.Body
		for i := len(t.revs) - 1; i >= 0; i-- {
			if b := t.revs[i].body; b != nil && !b.Retracted && json.Unmarshal(b.Data, &data) == nil {
				live = b
				break
			}
		}
		if live == nil || (family != "" && data.Family != family) {
			continue
		}
		en := JournalEntry{ID: id, Rev: cur.rec.Item.Rev, Data: data, Text: live.Text, Written: t.revs[0].rec.Item.Created,
			Updated: cur.rec.Item.Created, History: history(t)}
		if cur.body != nil {
			en.Removed, en.Reason = cur.body.Retracted, cur.body.Reason
			en.Edited = cur.rec.Item.Rev > 0 && !cur.body.Retracted
		}
		for _, a := range live.Attachments {
			en.Attachments = append(en.Attachments, AttachmentView{BlobID: a.BlobID, Name: a.Name, Mime: a.Mime, Size: a.Size,
				Width: a.Width, Height: a.Height, Thumb: a.Thumb})
		}
		out = append(out, en)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Data.Date != out[j].Data.Date {
			return out[i].Data.Date > out[j].Data.Date
		}
		return out[i].Written.After(out[j].Written)
	})
	return out, nil
}

// OpenJournalFile decrypts a file from one of your entries (see OpenAttachment).
func (e *Engine) OpenJournalFile(item, blob string) (string, error) {
	space, err := e.journalSpace(false)
	if err != nil || space == "" {
		return "", errors.New("there's no journal yet")
	}
	return e.OpenAttachment(space, item, blob)
}

// ShareJournal posts a copy of an entry (and its files, re-encrypted for the family) into the family's messages.
// The family sees the copy as an ordinary message from you; your journal stays private.
func (e *Engine) ShareJournal(family, item string) error {
	entries, err := e.Journal("")
	if err != nil {
		return err
	}
	var en *JournalEntry
	for i := range entries {
		if entries[i].ID == item {
			en = &entries[i]
		}
	}
	if en == nil || en.Removed {
		return errors.New("that entry can't be shared")
	}
	var files []FileInput
	for _, a := range en.Attachments {
		p, err := e.OpenJournalFile(item, a.BlobID)
		if err != nil {
			return err
		}
		defer os.RemoveAll(filepath.Dir(p))
		files = append(files, FileInput{Path: p, Name: a.Name, Mime: a.Mime, Thumb: a.Thumb, Width: a.Width, Height: a.Height})
	}
	day, _ := time.Parse("2006-01-02", en.Data.Date)
	text := "Shared from my journal (" + day.Format("Mon 2 Jan 2006") + "):\n\n" + en.Text
	return e.Send(SendRequest{Space: family, Text: text, Files: files})
}
