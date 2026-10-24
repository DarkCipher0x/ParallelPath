package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"pp/core/audit"
	"pp/core/envelope"
	"pp/core/pqc"
	"pp/engine/remote"
	"pp/engine/vault"
)

// decoded is a verified, decrypted item revision (memory only).
type decoded struct {
	rec  itemRec
	body *envelope.Body
	err  string
}

// Revision is one version of a message, as shown in its history.
type Revision struct {
	Rev       uint64    `json:"rev"`
	Time      time.Time `json:"time"`
	Text      string    `json:"text"`
	Reason    string    `json:"reason,omitempty"`
	Retracted bool      `json:"retracted,omitempty"`
}

// Reaction is one emoji on a message.
type Reaction struct {
	Emoji  string   `json:"emoji"`
	Count  int      `json:"count"`
	Mine   bool     `json:"mine"`
	People []string `json:"people"` // names
}

// Receipt says who first opened a message, and when (from the verified audit log).
type Receipt struct {
	Person string    `json:"person"`
	Name   string    `json:"name"`
	Time   time.Time `json:"time"`
}

// AttachmentView is a file on a message (thumbnail included; the file itself is fetched on demand).
type AttachmentView struct {
	BlobID string `json:"blob_id"`
	Name   string `json:"name"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Millis int64  `json:"millis,omitempty"`
	Thumb  []byte `json:"thumb,omitempty"`
}

// Message is a chat message as the apps show it.
type Message struct {
	ID          string           `json:"id"`
	Rev         uint64           `json:"rev"`
	Author      string           `json:"author"`
	AuthorName  string           `json:"author_name"`
	Mine        bool             `json:"mine"`
	Sent        time.Time        `json:"sent"`
	Text        string           `json:"text"`
	ReplyTo     string           `json:"reply_to,omitempty"`
	Edited      bool             `json:"edited"`
	Retracted   bool             `json:"retracted"`
	Reason      string           `json:"reason,omitempty"` // reason for the latest change
	History     []Revision       `json:"history"`
	Attachments []AttachmentView `json:"attachments,omitempty"`
	Reactions   []Reaction       `json:"reactions,omitempty"`
	ReadBy      []Receipt        `json:"read_by,omitempty"`
	Locked      bool             `json:"locked,omitempty"` // can't be decrypted on this device (no key for its epoch)
	Call        *CallInfo        `json:"call,omitempty"`   // a call in the family's history (not a typed message)
	Problem     string           `json:"problem,omitempty"`
}

// loadDecoded verifies (already done at sync) and decrypts every cached revision of a space, in order.
func (e *Engine) loadDecoded(space string) ([]*decoded, error) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return nil, errLocked
	}
	var out []*decoded
	err := v.Each(vault.Items, space+"/", func(_ string, raw []byte) bool {
		var r itemRec
		if json.Unmarshal(raw, &r) != nil {
			return true
		}
		h := r.Item.Hash()
		ck := fmt.Sprintf("%x", h[:16])
		e.mu.Lock()
		d, ok := e.bodies[ck]
		e.mu.Unlock()
		if !ok {
			d = &decoded{rec: r}
			if key, ok := e.epochKey(v, space, r.Item.Epoch); !ok {
				d.err = "locked"
			} else if author, err := e.cert(v, r.Item.Device); err != nil {
				d.err = "author unknown"
			} else if b, err := envelope.Open(&r.Item, key, author); err != nil {
				d.err = "failed verification"
			} else {
				d.body = b
			}
			if d.err != "locked" { // retry undecryptable items once their epoch key arrives
				e.mu.Lock()
				if e.bodies != nil {
					e.bodies[ck] = d
				}
				e.mu.Unlock()
			}
		}
		out = append(out, d)
		return true
	})
	return out, err
}

// Messages returns the chat of a space, oldest first, with edit history, reactions and read receipts.
func (e *Engine) Messages(space string) ([]Message, error) {
	all, err := e.loadDecoded(space)
	if err != nil {
		return nil, err
	}
	names := e.namesFor(space, all)
	byID := map[string]*Message{}
	var order []string
	type react struct{ person, emoji string }
	reactState := map[string]map[react]bool{}

	for _, d := range all {
		it := &d.rec.Item
		switch it.Kind {
		case envelope.KindMessage:
			m := byID[it.ItemID]
			if m == nil {
				m = &Message{ID: it.ItemID, Author: it.Person, AuthorName: names.of(it.Person), Mine: it.Person == e.person, Sent: it.Created}
				byID[it.ItemID] = m
				order = append(order, it.ItemID)
			}
			m.Rev = it.Rev
			if d.body == nil {
				m.Locked = d.err == "locked"
				if d.err != "locked" {
					m.Problem = d.err
				}
				continue
			}
			b := d.body
			m.History = append(m.History, Revision{Rev: it.Rev, Time: it.Created, Text: b.Text, Reason: b.Reason, Retracted: b.Retracted})
			if it.Rev == 0 {
				m.ReplyTo = it.Ref
			}
			m.Text, m.Retracted, m.Reason, m.Edited = b.Text, b.Retracted, b.Reason, it.Rev > 0 && !b.Retracted
			m.Attachments = m.Attachments[:0]
			if !b.Retracted {
				for _, a := range b.Attachments {
					m.Attachments = append(m.Attachments, AttachmentView{BlobID: a.BlobID, Name: a.Name, Mime: a.Mime, Size: a.Size,
						Width: a.Width, Height: a.Height, Millis: a.Millis, Thumb: a.Thumb})
				}
			}
		case envelope.KindCall:
			var cd CallData
			if d.body == nil || json.Unmarshal(d.body.Data, &cd) != nil || cd.Event != "end" {
				continue // the "start" record is in the audit log; the conversation shows the finished call
			}
			byID[it.ItemID] = &Message{ID: it.ItemID, Author: it.Person, AuthorName: names.of(it.Person), Mine: it.Person == e.person,
				Sent: it.Created, History: []Revision{}, Call: &CallInfo{Video: cd.Video, Seconds: cd.Seconds, Missed: cd.Missed}}
			order = append(order, it.ItemID)
		case envelope.KindReaction:
			if d.body == nil || it.Ref == "" {
				continue
			}
			if reactState[it.Ref] == nil {
				reactState[it.Ref] = map[react]bool{}
			}
			reactState[it.Ref][react{it.Person, d.body.Emoji}] = !d.body.Remove
		}
	}

	for id, set := range reactState {
		m := byID[id]
		if m == nil {
			continue
		}
		byEmoji := map[string]*Reaction{}
		for r, on := range set {
			if !on {
				continue
			}
			x := byEmoji[r.emoji]
			if x == nil {
				x = &Reaction{Emoji: r.emoji}
				byEmoji[r.emoji] = x
			}
			x.Count++
			x.People = append(x.People, names.of(r.person))
			if r.person == e.person {
				x.Mine = true
			}
		}
		for _, x := range byEmoji {
			sort.Strings(x.People)
			m.Reactions = append(m.Reactions, *x)
		}
		sort.Slice(m.Reactions, func(i, j int) bool {
			if m.Reactions[i].Count != m.Reactions[j].Count {
				return m.Reactions[i].Count > m.Reactions[j].Count
			}
			return m.Reactions[i].Emoji < m.Reactions[j].Emoji
		})
	}

	// Read receipts from the verified audit log: first view of any revision by each person.
	e.eachAudit(space, func(en *audit.Entry) {
		if en.Action != audit.ItemView {
			return
		}
		m := byID[en.ItemID]
		if m == nil || en.Person == m.Author {
			return
		}
		for _, r := range m.ReadBy {
			if r.Person == en.Person {
				return
			}
		}
		m.ReadBy = append(m.ReadBy, Receipt{Person: en.Person, Name: names.of(en.Person), Time: en.Time})
	})

	out := make([]Message, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// latest returns the newest stored revision of an item.
func (e *Engine) latest(space, itemID string) (*decoded, error) {
	all, err := e.loadDecoded(space)
	if err != nil {
		return nil, err
	}
	var found *decoded
	for _, d := range all {
		if d.rec.Item.ItemID == itemID && (found == nil || d.rec.Item.Rev > found.rec.Item.Rev) {
			found = d
		}
	}
	if found == nil {
		return nil, errors.New("that message isn't on this device yet - try again in a moment")
	}
	return found, nil
}

// SendRequest is a new message.
type SendRequest struct {
	Space   string      `json:"space"`
	Text    string      `json:"text"`
	ReplyTo string      `json:"reply_to,omitempty"`
	Files   []FileInput `json:"files,omitempty"`
}

// Send encrypts any attachments, uploads them, then sends the message.
func (e *Engine) Send(req SendRequest) error {
	text := strings.TrimSpace(req.Text)
	if text == "" && len(req.Files) == 0 {
		return errors.New("type a message or add a file")
	}
	var atts []envelope.Attachment
	for _, f := range req.Files {
		a, err := e.uploadFile(req.Space, f)
		if err != nil {
			return err
		}
		atts = append(atts, *a)
	}
	_, err := e.post(req.Space, envelope.Draft{Kind: envelope.KindMessage, Ref: req.ReplyTo, Body: envelope.Body{Text: text, Attachments: atts}})
	return err
}

// React adds (or with remove=true, takes back) an emoji reaction.
func (e *Engine) React(space, itemID, emoji string, remove bool) error {
	emoji = strings.TrimSpace(emoji)
	if emoji == "" || len(emoji) > 32 {
		return errors.New("choose an emoji")
	}
	_, err := e.post(space, envelope.Draft{Kind: envelope.KindReaction, Ref: itemID, Body: envelope.Body{Emoji: emoji, Remove: remove}})
	return err
}

// Edit changes one of your messages. A reason is required and is kept forever with the history.
func (e *Engine) Edit(space, itemID, text, reason string) error {
	return e.revise(space, itemID, envelope.Body{Text: strings.TrimSpace(text), Reason: strings.TrimSpace(reason)})
}

// Retract withdraws one of your messages (it stays in the history, marked as withdrawn, with the reason).
func (e *Engine) Retract(space, itemID, reason string) error {
	return e.revise(space, itemID, envelope.Body{Retracted: true, Reason: strings.TrimSpace(reason)})
}

func (e *Engine) revise(space, itemID string, b envelope.Body) error {
	if b.Reason == "" {
		return errors.New("a reason is required - it's kept with the message history")
	}
	cur, err := e.latest(space, itemID)
	if err != nil {
		return err
	}
	if cur.rec.Item.Person != e.person {
		return errors.New("you can only change your own messages")
	}
	if cur.body != nil && cur.body.Retracted {
		return errors.New("this message was withdrawn and can't be changed again")
	}
	if !b.Retracted && cur.body != nil {
		b.Attachments = cur.body.Attachments // edits keep the original files
	}
	_, err = e.post(space, envelope.Draft{Prev: &cur.rec.Item, Body: b})
	return err
}

// MarkRead records that you opened a message (a read receipt the other parent sees, in the audit log).
func (e *Engine) MarkRead(space, itemID string) error {
	cur, err := e.latest(space, itemID)
	if err != nil {
		return err
	}
	it := &cur.rec.Item
	if it.Person == e.person {
		return nil
	}
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return errLocked
	}
	rk := fmt.Sprintf("read/%s/%s/%d", space, itemID, it.Rev)
	var done bool
	if v.Get(vault.State, rk, &done) == nil {
		return nil
	}
	h := it.Hash()
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.ItemView, ItemID: itemID, ItemRev: it.Rev, ItemHash: h[:]})
	if err != nil {
		return err
	}
	err = e.call("POST", "/api/v1/spaces/"+space+"/record", map[string]any{"intent": in}, nil)
	if err == nil || remote.IsStatus(err, 409) {
		return v.Put(vault.State, rk, true)
	}
	return err
}

// post seals and sends an item under the space's current epoch, with its audit intent, then syncs.
// If the keys were rotated in the meantime (409), it syncs and tries once more with the new epoch.
func (e *Engine) post(space string, d envelope.Draft) (*envelope.Item, error) {
	for attempt := 0; ; attempt++ {
		it, err := e.sealItem(space, d)
		if err != nil {
			return nil, err
		}
		h := it.Hash()
		action := audit.ItemCreate
		if it.Rev > 0 {
			action = audit.ItemRevise
		}
		in, err := e.intent(audit.Intent{SpaceID: space, Action: action, ItemID: it.ItemID, ItemRev: it.Rev, ItemHash: h[:], Detail: "kind=" + it.Kind})
		if err != nil {
			return nil, err
		}
		var res struct {
			Seq uint64 `json:"seq"`
		}
		err = e.call("POST", "/api/v1/spaces/"+space+"/items", map[string]any{"item": it, "intent": in}, &res)
		if remote.IsStatus(err, 409) && attempt == 0 {
			e.refreshKeys(space)
			continue
		}
		if err != nil {
			return nil, err
		}
		// Keep our own item right away (the next sync fetches the same sequence number and overwrites it identically),
		// so it shows instantly even while a background sync is still running.
		e.mu.Lock()
		v := e.v
		e.mu.Unlock()
		if v != nil && res.Seq > 0 {
			_ = v.Put(vault.Items, itemKey(space, res.Seq), itemRec{Seq: res.Seq, Item: *it})
		}
		e.syncSpace(space)
		e.listen("changed", map[string]string{"what": "space", "space_id": space})
		return it, nil
	}
}

func (e *Engine) sealItem(space string, d envelope.Draft) (*envelope.Item, error) {
	sv, err := e.spaceView(space)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.v == nil {
		return nil, errLocked
	}
	if !e.mayAdd(sv, d.Kind) {
		return nil, errors.New("the family's permissions don't let you add that here")
	}
	key, ok := e.epochKey(e.v, space, sv.Space.Epoch)
	if !ok {
		return nil, errors.New("you can't post yet: a parent still needs to approve this device")
	}
	defer pqc.Wipe(key)
	d.SpaceID = space
	return envelope.Seal(e.dev, e.person, sv.Space.Epoch, key, d, time.Now())
}

// intent fills in who/when and signs an audit intent with this device.
func (e *Engine) intent(in audit.Intent) (*audit.Intent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dev == nil {
		return nil, errLocked
	}
	in.Person, in.Device, in.ClientTime = e.person, e.dev.DeviceID(), time.Now()
	return audit.SignIntent(e.dev.Sign, in)
}

// eachAudit visits the verified audit entries cached for a space, in order.
func (e *Engine) eachAudit(space string, fn func(*audit.Entry)) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return
	}
	_ = v.Each(vault.State, "audit/"+space+"/", func(_ string, raw []byte) bool {
		var en audit.Entry
		if json.Unmarshal(raw, &en) == nil {
			fn(&en)
		}
		return true
	})
}

// CallInfo is a finished call, as shown in the conversation.
type CallInfo struct {
	Video   bool `json:"video"`
	Seconds int  `json:"seconds"`
	Missed  bool `json:"missed"`
}
