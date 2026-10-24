package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"pp/core/audit"
	"pp/core/envelope"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
	"pp/engine/vault"
)

// record is the structured content of a KindRecord item.
type record struct {
	Type string `json:"type"` // "profile" (a person's name in this space) or "space" (the family's name)
	Name string `json:"name"`
}

type nameBook struct {
	people  map[string]string
	space   string
	journal bool // this is someone's private journal space, not a family
}

func (n nameBook) of(person string) string {
	if s := n.people[person]; s != "" {
		return s
	}
	return "Member " + person[:6]
}

// names reads the latest profile/space records (encrypted inside the space, so the server never sees names).
func (e *Engine) names(all []*decoded) nameBook {
	n := nameBook{people: map[string]string{}}
	for _, d := range all {
		if d.rec.Item.Kind != envelope.KindRecord || d.body == nil || d.body.Retracted {
			continue
		}
		var r record
		if json.Unmarshal(d.body.Data, &r) != nil {
			continue
		}
		switch r.Type {
		case "profile":
			n.people[d.rec.Item.Person] = r.Name
		case "space":
			n.space = r.Name
		case journalMarker:
			n.journal = true
		}
	}
	return n
}

// MemberView is a person in a family, as shown in the app.
type MemberView struct {
	Person   string    `json:"person"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Status   string    `json:"status"`
	Me       bool      `json:"me"`
	ReadOnly bool      `json:"read_only"`
	NotAfter time.Time `json:"not_after,omitempty"`
	// HistoryFrom: professionals limited to the history from this date on.
	HistoryFrom time.Time `json:"history_from,omitempty"`
	NeedsKeys   bool      `json:"needs_approval"`       // joined, waiting for a parent to approve them
	Perms       *Perms    `json:"perms,omitempty"`      // what they may do in the family
	InvitedBy   string    `json:"invited_by,omitempty"` // the parent who invited them (who may change Perms)
	Joined      time.Time `json:"joined"`
}

// SpaceSummary is a family space in the list.
type SpaceSummary struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	MyRole       string       `json:"my_role"`
	ReadOnly     bool         `json:"read_only"`
	Waiting      bool         `json:"waiting_for_approval"`
	Members      []MemberView `json:"members"`
	Unread       int          `json:"unread"`
	LastText     string       `json:"last_text,omitempty"`
	LastTime     time.Time    `json:"last_time,omitempty"`
	AuditProblem string       `json:"audit_problem,omitempty"`
	ChildrenChat bool         `json:"children_chat"` // children in the family may send messages
	CanChat      bool         `json:"can_chat"`      // this person may send messages here
}

// Spaces lists this person's family spaces.
func (e *Engine) Spaces() ([]SpaceSummary, error) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return nil, errLocked
	}
	var list []spaceView
	if err := v.Get(vault.State, "spaces", &list); err != nil {
		return []SpaceSummary{}, nil
	}
	out := []SpaceSummary{}
	var journal string
	_ = v.Get(vault.State, "journal-space", &journal)
	for _, s := range list {
		id := s.Space.ID
		if id == journal {
			continue
		}
		var sv spaceView
		if v.Get(vault.State, "space/"+id, &sv) != nil {
			sv = s
		}
		if sv.Space.Parent != "" || s.Space.Parent != "" {
			continue // a private conversation: listed inside its family (Conversations)
		}
		all, _ := e.loadDecoded(id)
		names := e.names(all)
		if names.journal {
			continue // the private journal isn't a family
		}
		sum := SpaceSummary{ID: id, Name: names.space, MyRole: sv.Me.Role, ReadOnly: sv.Me.ReadOnly, Waiting: !sv.Me.HasKeys,
			ChildrenChat: sv.Space.ChildrenChat}
		sum.CanChat = !sv.Me.ReadOnly || (sv.Me.Role == "child" && sv.Space.ChildrenChat)
		if sum.Name == "" {
			sum.Name = "Family space"
		}
		for _, m := range sv.Members {
			sum.Members = append(sum.Members, MemberView{Person: m.Person, Name: names.of(m.Person), Role: m.Role, Status: m.Status,
				Me: m.Person == e.person, ReadOnly: m.ReadOnly, NotAfter: m.NotAfter, NeedsKeys: m.Status == "active" && !m.HasKeys,
				Joined: m.Joined, HistoryFrom: m.HistoryFrom, Perms: m.Perms, InvitedBy: m.InvitedBy})
		}
		for _, d := range all {
			it := &d.rec.Item
			if it.Kind != envelope.KindMessage || d.body == nil {
				continue
			}
			if it.Rev == 0 || it.Created.After(sum.LastTime) {
				sum.LastText, sum.LastTime = previewText(d.body), it.Created
			}
			if it.Rev == 0 && it.Person != e.person {
				var read bool
				if v.Get(vault.State, fmt.Sprintf("read/%s/%s/%d", id, it.ItemID, 0), &read) != nil {
					sum.Unread++
				}
			}
		}
		var problem string
		if v.Get(vault.State, "audit-problem/"+id, &problem) == nil {
			sum.AuditProblem = problem
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastTime.After(out[j].LastTime) })
	return out, nil
}

func previewText(b *envelope.Body) string {
	switch {
	case b.Retracted:
		return "Message withdrawn"
	case b.Text != "":
		r := []rune(b.Text)
		if len(r) > 80 {
			return string(r[:80]) + "…"
		}
		return b.Text
	case len(b.Attachments) > 0:
		return "📎 " + b.Attachments[0].Name
	}
	return ""
}

// Account is this person's account on the server: whether they may start another family (the server's operator
// decides that; parents only invite people into their own family).
type Account struct {
	FamiliesLeft   int  `json:"families_left"`
	CanStartFamily bool `json:"can_start_family"`
}

// Account asks the server what this account may do.
func (e *Engine) Account() (*Account, error) {
	e.mu.Lock()
	srv := e.srv
	e.mu.Unlock()
	if srv == nil {
		return nil, errLocked
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var a Account
	return &a, srv.Call(ctx, "GET", "/api/v1/account", nil, &a)
}

// CreateSpace starts another family space with this person as its parent (when the server's operator allowed it).
func (e *Engine) CreateSpace(name string) (string, error) {
	e.mu.Lock()
	if e.v == nil {
		e.mu.Unlock()
		return "", errLocked
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	id, err := e.createSpaceLocked(ctx, name, false)
	e.mu.Unlock()
	if err == nil {
		e.SyncAll()
	}
	return id, err
}

// createSpaceLocked creates the space with a fresh epoch key granted to this device, and queues the family name
// and this person's name to be posted (encrypted) by the next sync.
func (e *Engine) createSpaceLocked(ctx context.Context, name string, journal bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return "", errors.New("give the family space a name (up to 80 characters)")
	}
	var cert keys.DeviceCert
	if err := e.v.Get(vault.Keys, "cert", &cert); err != nil {
		return "", err
	}
	me, err := cert.Verify()
	if err != nil {
		return "", err
	}
	space := wire.NewID()
	key := keys.NewEpochKey()
	defer pqc.Wipe(key)
	g, err := keys.Grant(e.dev, me, space, 1, key, false, time.Time{}, time.Now())
	if err != nil {
		return "", err
	}
	detail := "" // a family; the server only lets someone start one if its operator allowed it
	if journal {
		detail = "kind=journal"
	}
	in, err := audit.SignIntent(e.dev.Sign, audit.Intent{SpaceID: space, Action: audit.SpaceCreate, Detail: detail, Person: e.person,
		Device: e.dev.DeviceID(), ClientTime: time.Now()})
	if err != nil {
		return "", err
	}
	if err := e.srv.Call(ctx, "POST", "/api/v1/spaces", map[string]any{"space_id": space, "intent": in, "grants": []keys.EpochGrant{*g}}, nil); err != nil {
		return "", err
	}
	var myName string
	_ = e.v.Get(vault.State, "my-name", &myName)
	return space, firstErr(
		e.v.Put(vault.Epochs, epochKeyName(space, 1), key),
		e.v.Put(vault.State, "pending-spacename/"+space, name),
		e.v.Put(vault.State, "pending-profile/"+space, myName),
	)
}

// SetMyName sets how this person appears in a family space.
func (e *Engine) SetMyName(space, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return errors.New("enter a name (up to 60 characters)")
	}
	return e.putRecord(space, record{Type: "profile", Name: name})
}

// RenameSpace changes the family's name (parents only; recorded and kept in history like everything else).
func (e *Engine) RenameSpace(space, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return errors.New("enter a name (up to 80 characters)")
	}
	return e.putRecord(space, record{Type: "space", Name: name})
}

// putRecord posts a new record, or a revision of this person's existing one of the same type (with a reason).
func (e *Engine) putRecord(space string, r record) error {
	all, err := e.loadDecoded(space)
	if err != nil {
		return err
	}
	var prev *envelope.Item
	for _, d := range all {
		var old record
		if d.rec.Item.Kind == envelope.KindRecord && d.rec.Item.Person == e.person && d.body != nil &&
			json.Unmarshal(d.body.Data, &old) == nil && old.Type == r.Type {
			it := d.rec.Item
			prev = &it
		}
	}
	d := envelope.Draft{Kind: envelope.KindRecord, Body: envelope.Body{Data: jsonData(r)}}
	if prev != nil {
		d.Prev, d.Body.Reason = prev, "name changed"
	}
	_, err = e.post(space, d)
	return err
}

// Invite creates a one-time code for someone to join (parents only). Professionals need an end date, and may be
// limited to the history from a date on (historyFrom; zero = all of it).
//
// perms are what the person may do (nil: the role's defaults); only the inviting parent can change them later.
func (e *Engine) Invite(space, role string, notAfter, historyFrom time.Time, perms *Perms) (string, time.Time, error) {
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.MemberInvite, Detail: "role=" + role})
	if err != nil {
		return "", time.Time{}, err
	}
	var res struct {
		Code    string    `json:"code"`
		Expires time.Time `json:"expires"`
	}
	err = e.call("POST", "/api/v1/spaces/"+space+"/invites", map[string]any{"role": role, "not_after": notAfter, "history_from": historyFrom, "perms": perms, "intent": in}, &res)
	return res.Code, res.Expires, err
}

// SetChildrenChat lets children in the family send messages and reactions (or stops them). Parents only.
func (e *Engine) SetChildrenChat(space string, on bool) error {
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.SpaceSettings, Detail: fmt.Sprintf("children_chat=%t", on)})
	if err != nil {
		return err
	}
	if err := e.call("POST", "/api/v1/spaces/"+space+"/settings", map[string]any{"children_chat": on, "intent": in}, nil); err != nil {
		return err
	}
	e.syncSpace(space)
	return nil
}

// epochsInWindow says which epochs hold anything first created from a date on: a professional with a history window
// only gets those keys (plus the current one), so older history stays unreadable to them, not just hidden.
func (e *Engine) epochsInWindow(v *vault.Vault, space string, from time.Time, current uint64) map[uint64]bool {
	in := map[uint64]bool{current: true}
	_ = v.Each(vault.Items, space+"/", func(_ string, raw []byte) bool {
		var r itemRec
		// Names don't pull in an older key: they're posted again under the current one (carryNamesForWindows).
		if json.Unmarshal(raw, &r) == nil && !r.Item.Created.Before(from) && r.Item.Kind != envelope.KindRecord {
			in[r.Item.Epoch] = true
		}
		return true
	})
	return in
}

// carryNamesForWindows posts this person's name again (and, for a parent, the family's name) while a professional
// with a history window is in the family and the last copy is from before their window: they then see who's who
// without ever getting the older keys. A fresh record each time, so it falls inside their window.
func (e *Engine) carryNamesForWindows(v *vault.Vault, space string) {
	var sv spaceView
	if v.Get(vault.State, "space/"+space, &sv) != nil || sv.Space.Parent != "" {
		return
	}
	var from time.Time
	for _, m := range sv.Members {
		if m.Status == "active" && !m.HistoryFrom.IsZero() && (m.NotAfter.IsZero() || time.Now().Before(m.NotAfter)) && m.HistoryFrom.After(from) {
			from = m.HistoryFrom
		}
	}
	if from.IsZero() {
		return
	}
	k, ok := e.epochKey(v, space, sv.Space.Epoch)
	if !ok {
		return
	}
	pqc.Wipe(k)
	all, err := e.loadDecoded(space)
	if err != nil {
		return
	}
	var mine, family *decoded
	for _, d := range all {
		var r record
		if d.rec.Item.Kind != envelope.KindRecord || d.body == nil || json.Unmarshal(d.body.Data, &r) != nil {
			continue
		}
		switch {
		case r.Type == "profile" && d.rec.Item.Person == e.person:
			mine = d
		case r.Type == "space":
			family = d
		}
	}
	repost := func(d *decoded) {
		if d != nil && d.rec.Item.Created.Before(from) {
			_, _ = e.post(space, envelope.Draft{Kind: envelope.KindRecord, Body: envelope.Body{Data: d.body.Data}})
		}
	}
	repost(mine)
	if sv.Me.Role == "parent" {
		repost(family)
	}
}

// SafetyNumber is 60 digits both people compare (in person or on a call) before approving: it changes if either
// identity key is different from what the other side sees, which exposes a server pretending to be someone.
func (e *Engine) SafetyNumber(person string) (string, error) {
	certs, err := e.personCerts(person)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	mine := e.ident.Key.Public().Bytes()
	e.mu.Unlock()
	theirs := certs[0].Identity.Bytes()
	a, b := mine, theirs
	if string(a) > string(b) {
		a, b = b, a
	}
	h := pqc.Hash([]byte("pp/safety-number/v1"), a, b)
	n := new(big.Int).SetBytes(h[:32])
	digits := fmt.Sprintf("%060s", n.Text(10))
	digits = digits[len(digits)-60:]
	var sb strings.Builder
	for i := 0; i < 60; i += 5 {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(digits[i : i+5])
	}
	return sb.String(), nil
}

// personCerts fetches and verifies all of a person's devices. All must share one identity key.
func (e *Engine) personCerts(person string) ([]*keys.VerifiedDevice, error) {
	var res struct {
		Certs []keys.DeviceCert `json:"certs"`
	}
	if err := e.call("GET", "/api/v1/people/"+person+"/devices", nil, &res); err != nil {
		return nil, err
	}
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	var out []*keys.VerifiedDevice
	for i := range res.Certs {
		vd, err := res.Certs[i].Verify()
		if err != nil || vd.PersonID != person {
			return nil, errors.New("a device certificate from the server failed verification")
		}
		if v != nil {
			_ = v.Put(vault.Certs, vd.DeviceID, res.Certs[i])
		}
		out = append(out, vd)
	}
	if len(out) == 0 {
		return nil, errors.New("that person has no active devices")
	}
	return out, nil
}

// Approve shares the family's keys (every epoch, so they can read the whole history) with all of a new member's
// devices. Do this only after comparing the safety number with them.
func (e *Engine) Approve(space, person string) error {
	sv, err := e.spaceView(space)
	if err != nil {
		return err
	}
	if sv.Me.Role != "parent" {
		return errors.New("only a parent can approve people")
	}
	var m *member
	for i := range sv.Members {
		if sv.Members[i].Person == person && sv.Members[i].Status == "active" {
			m = &sv.Members[i]
		}
	}
	if m == nil {
		return errors.New("that person isn't an active member")
	}
	devs, err := e.personCerts(person)
	if err != nil {
		return err
	}
	e.mu.Lock()
	v, dev := e.v, e.dev
	e.mu.Unlock()
	if v == nil {
		return errLocked
	}
	var window map[uint64]bool
	if !m.HistoryFrom.IsZero() {
		window = e.epochsInWindow(v, space, m.HistoryFrom, sv.Space.Epoch)
	}
	for epoch := uint64(1); epoch <= sv.Space.Epoch; epoch++ {
		if window != nil && !window[epoch] {
			continue // only history before their window: they don't get that key
		}
		key, ok := e.epochKey(v, space, epoch)
		if !ok {
			continue // this device never had that epoch; another parent can share it
		}
		var grants []keys.EpochGrant
		for _, d := range devs {
			g, err := keys.Grant(dev, d, space, epoch, key, m.ReadOnly, m.NotAfter, time.Now())
			if err != nil {
				pqc.Wipe(key)
				return err
			}
			grants = append(grants, *g)
		}
		pqc.Wipe(key)
		in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.KeysGrant, Detail: "person=" + person})
		if err != nil {
			return err
		}
		if err := e.call("POST", "/api/v1/spaces/"+space+"/keys", map[string]any{"epoch": epoch, "grants": grants, "intent": in}, nil); err != nil {
			return err
		}
	}
	e.syncSpace(space)
	return nil
}

// Remove ends someone's access (parents only) and immediately rotates the family's keys, so nothing new can be
// read with anything they still hold.
func (e *Engine) Remove(space, person string) error {
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.MemberRemove, Detail: "person=" + person})
	if err != nil {
		return err
	}
	if err := e.call("POST", "/api/v1/spaces/"+space+"/members/"+person+"/remove", map[string]any{"intent": in}, nil); err != nil {
		return err
	}
	return e.Rotate(space)
}

// Rotate moves the space to a new epoch key, wrapped to every device of every remaining member.
func (e *Engine) Rotate(space string) error {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return errLocked
	}
	if err := e.syncSpaceView(v, space); err != nil {
		return err
	}
	sv, err := e.spaceView(space)
	if err != nil {
		return err
	}
	next := sv.Space.Epoch + 1
	key := keys.NewEpochKey()
	defer pqc.Wipe(key)
	e.mu.Lock()
	dev := e.dev
	e.mu.Unlock()
	// Only devices that already hold the key are re-keyed: a rotation never approves a new device.
	keyed := map[string]bool{}
	for _, d := range sv.KeyedDevices {
		keyed[d] = true
	}
	var grants []keys.EpochGrant
	for _, m := range sv.Members {
		if m.Status != "active" || (!m.NotAfter.IsZero() && time.Now().After(m.NotAfter)) {
			continue
		}
		devs, err := e.personCerts(m.Person)
		if err != nil {
			continue // no devices left (e.g. all revoked): nothing to re-key for them
		}
		for _, d := range devs {
			if !keyed[d.DeviceID] {
				continue
			}
			g, err := keys.Grant(dev, d, space, next, key, m.ReadOnly, m.NotAfter, time.Now())
			if err != nil {
				return err
			}
			grants = append(grants, *g)
		}
	}
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.EpochRotate})
	if err != nil {
		return err
	}
	if err := e.call("POST", "/api/v1/spaces/"+space+"/keys", map[string]any{"epoch": next, "grants": grants, "intent": in}, nil); err != nil {
		return err
	}
	if err := v.Put(vault.Epochs, epochKeyName(space, next), key); err != nil {
		return err
	}
	_ = e.syncSpaceView(v, space)
	e.syncSpace(space)
	return nil
}
