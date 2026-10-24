package app

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"pp/core/audit"
	"pp/core/envelope"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/core/wire"
	"pp/engine/vault"
)

// Private conversations: a space of their own inside a family, for some of its members, with keys only their
// devices hold (see server/internal/svc/conversations.go). Messages, reactions and calls work in them exactly as
// in the family chat; who may start one with whom, and call whom, follows the family's permissions.

// Perms mirror the server's: what a member may do in a family (set by the parent who invited them).
type Perms struct {
	See         []string `json:"see"`
	Add         []string `json:"add"`
	Message     string   `json:"message"` // anyone | parents | chosen | nobody
	MessageWith []string `json:"message_with,omitempty"`
	Call        string   `json:"call"`
	CallWith    []string `json:"call_with,omitempty"`
	Video       bool     `json:"video"`
	FamilyCall  bool     `json:"family_call"`
}

// reaches reports whether a rule allows contacting a person with a given family role.
func reaches(rule string, with []string, person, role string) bool {
	switch rule {
	case "anyone":
		return true
	case "parents":
		return role == "parent"
	case "chosen":
		return slices.Contains(with, person)
	}
	return false
}

// Conversation is a private conversation as listed in a family.
type Conversation struct {
	ID       string       `json:"id"`
	Family   string       `json:"family"`
	Title    string       `json:"title"` // the other people's names
	With     []MemberView `json:"with"`  // everyone else in it
	LastText string       `json:"last_text,omitempty"`
	LastTime time.Time    `json:"last_time,omitempty"`
	Unread   int          `json:"unread"`
	CanWrite bool         `json:"can_write"` // the family still allows me to message everyone in it
	CanCall  bool         `json:"can_call"`
	Video    bool         `json:"video"`   // video calls allowed (else voice only)
	Waiting  bool         `json:"waiting"` // this device doesn't have the keys yet
}

// Contact is someone in a family this person may start a conversation with and/or call.
type Contact struct {
	Person  string `json:"person"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Message bool   `json:"message"`
	Call    bool   `json:"call"`
}

// familyOf returns the family a conversation belongs to ("" for a family itself).
func (e *Engine) familyOf(v *vault.Vault, space string) string {
	var sv spaceView
	if v.Get(vault.State, "space/"+space, &sv) == nil {
		return sv.Space.Parent
	}
	return ""
}

// namesFor is names(all), plus - in a conversation - the family's names (a conversation doesn't repeat them).
func (e *Engine) namesFor(space string, all []*decoded) nameBook {
	n := e.names(all)
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return n
	}
	if fam := e.familyOf(v, space); fam != "" {
		if fall, err := e.loadDecoded(fam); err == nil {
			fn := e.names(fall)
			for p, name := range fn.people {
				if n.people[p] == "" {
					n.people[p] = name
				}
			}
			n.space = fn.space
		}
	}
	return n
}

// myPerms are this person's effective permissions in a family.
func (e *Engine) myPerms(sv *spaceView) Perms {
	if sv.Me.Perms != nil {
		return *sv.Me.Perms
	}
	if sv.Me.Role == "parent" {
		all := []string{"messages", "calendar", "expenses", "documents", "info"}
		return Perms{See: all, Add: all, Message: "anyone", Call: "anyone", Video: true, FamilyCall: true}
	}
	return Perms{Message: "nobody", Call: "nobody"}
}

// sectionOf mirrors the server: which part of the family's record an item kind belongs to.
func sectionOf(kind string) string {
	switch kind {
	case envelope.KindMessage, envelope.KindReaction, envelope.KindCall:
		return "messages"
	case envelope.KindEvent, envelope.KindSchedule, envelope.KindSwap, envelope.KindCheckIn:
		return "calendar"
	case envelope.KindExpense:
		return "expenses"
	case envelope.KindFile:
		return "documents"
	case envelope.KindInfo:
		return "info"
	}
	return ""
}

// mayAdd mirrors the server's rule for adding an item (the server decides; this just gives a clear message first).
func (e *Engine) mayAdd(sv *spaceView, kind string) bool {
	switch {
	case sv.Space.Parent != "" || sv.Me.Role == "parent" || kind == envelope.KindRecord || kind == envelope.KindJournal:
		return true // conversations: the server checks the family's permissions
	case kind == envelope.KindExpense || kind == envelope.KindSchedule || kind == envelope.KindSwap:
		return false
	}
	p := e.myPerms(sv)
	if sv.Me.Role == "child" && sv.Space.ChildrenChat && sectionOf(kind) == "messages" {
		return true
	}
	return slices.Contains(p.Add, sectionOf(kind))
}

// Contacts lists the people in a family this person may message privately and/or call.
func (e *Engine) Contacts(family string) ([]Contact, error) {
	sv, err := e.spaceView(family)
	if err != nil {
		return nil, err
	}
	all, _ := e.loadDecoded(family)
	names := e.names(all)
	p := e.myPerms(sv)
	out := []Contact{}
	for _, m := range sv.Members {
		if m.Person == e.person || m.Status != "active" || !m.HasKeys {
			continue
		}
		c := Contact{Person: m.Person, Name: names.of(m.Person), Role: m.Role, Message: reaches(p.Message, p.MessageWith, m.Person, m.Role),
			Call: reaches(p.Call, p.CallWith, m.Person, m.Role)}
		// They must be allowed to reach me back, or the conversation would be one-way.
		if m.Perms != nil {
			c.Message = c.Message && reaches(m.Perms.Message, m.Perms.MessageWith, e.person, sv.Me.Role)
		}
		if c.Message || c.Call {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Conversations lists this person's private conversations in a family, newest first.
func (e *Engine) Conversations(family string) ([]Conversation, error) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return nil, errLocked
	}
	fam, err := e.spaceView(family)
	if err != nil {
		return nil, err
	}
	var list []spaceView
	_ = v.Get(vault.State, "spaces", &list)
	fall, _ := e.loadDecoded(family)
	fnames := e.names(fall)
	roles := map[string]string{}
	for _, m := range fam.Members {
		roles[m.Person] = m.Role
	}
	p := e.myPerms(fam)
	out := []Conversation{}
	for _, s := range list {
		if s.Space.Parent != family {
			continue
		}
		var sv spaceView
		if v.Get(vault.State, "space/"+s.Space.ID, &sv) != nil {
			sv = s
		}
		c := Conversation{ID: s.Space.ID, Family: family, Waiting: !sv.Me.HasKeys, CanWrite: true, CanCall: true, Video: p.Video}
		var titles []string
		for _, m := range sv.Members {
			if m.Person == e.person || m.Status != "active" {
				continue
			}
			c.With = append(c.With, MemberView{Person: m.Person, Name: fnames.of(m.Person), Role: roles[m.Person], Status: m.Status})
			titles = append(titles, fnames.of(m.Person))
			c.CanWrite = c.CanWrite && reaches(p.Message, p.MessageWith, m.Person, roles[m.Person])
			c.CanCall = c.CanCall && reaches(p.Call, p.CallWith, m.Person, roles[m.Person])
		}
		if len(titles) == 0 {
			c.Title, c.CanWrite, c.CanCall = "Nobody else is in this conversation now", false, false
		} else {
			sort.Strings(titles)
			c.Title = joinNames(titles)
		}
		all, _ := e.loadDecoded(s.Space.ID)
		for _, d := range all {
			it := &d.rec.Item
			if it.Kind != envelope.KindMessage || d.body == nil {
				continue
			}
			if it.Rev == 0 || it.Created.After(c.LastTime) {
				c.LastText, c.LastTime = previewText(d.body), it.Created
			}
			if it.Rev == 0 && it.Person != e.person {
				var read bool
				if v.Get(vault.State, fmt.Sprintf("read/%s/%s/%d", s.Space.ID, it.ItemID, 0), &read) != nil {
					c.Unread++
				}
			}
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastTime.After(out[j].LastTime) })
	return out, nil
}

func joinNames(n []string) string {
	switch len(n) {
	case 1:
		return n[0]
	case 2:
		return n[0] + " and " + n[1]
	}
	return fmt.Sprintf("%s, %s and %d more", n[0], n[1], len(n)-2)
}

// StartConversation opens a private conversation with some people in a family - or returns the existing one with
// exactly the same people.
func (e *Engine) StartConversation(family string, people []string) (string, error) {
	if len(people) == 0 {
		return "", errors.New("choose who to talk to")
	}
	sv, err := e.spaceView(family)
	if err != nil {
		return "", err
	}
	if sv.Space.Parent != "" {
		return "", errors.New("start a conversation from the family")
	}
	want := append([]string{e.person}, people...)
	slices.Sort(want)
	want = slices.Compact(want)
	existing, _ := e.Conversations(family)
	for _, c := range existing {
		have := []string{e.person}
		for _, m := range c.With {
			have = append(have, m.Person)
		}
		slices.Sort(have)
		if slices.Equal(have, want) {
			return c.ID, nil
		}
	}
	members := map[string]member{}
	for _, m := range sv.Members {
		members[m.Person] = m
	}
	p := e.myPerms(sv)
	fall, _ := e.loadDecoded(family)
	fnames := e.names(fall)
	for _, who := range people {
		m, ok := members[who]
		if !ok || m.Status != "active" {
			return "", errors.New("that person isn't in the family")
		}
		if !reaches(p.Message, p.MessageWith, who, m.Role) {
			return "", errors.New("the family's permissions don't let you message " + fnames.of(who) + " privately")
		}
	}
	e.mu.Lock()
	v, dev := e.v, e.dev
	e.mu.Unlock()
	if v == nil {
		return "", errLocked
	}
	conv := wire.NewID()
	key := keys.NewEpochKey()
	defer pqc.Wipe(key)
	var grants []keys.EpochGrant
	for _, who := range want {
		certs, err := e.personCerts(who)
		if err != nil {
			return "", err
		}
		for _, d := range certs {
			g, err := keys.Grant(dev, d, conv, 1, key, false, members[who].NotAfter, time.Now())
			if err != nil {
				return "", err
			}
			grants = append(grants, *g)
		}
	}
	in, err := e.intent(audit.Intent{SpaceID: conv, Action: audit.SpaceCreate, Detail: "parent=" + family})
	if err != nil {
		return "", err
	}
	others := slices.DeleteFunc(slices.Clone(want), func(s string) bool { return s == e.person })
	if err := e.call("POST", "/api/v1/spaces/"+family+"/conversations",
		map[string]any{"space_id": conv, "participants": others, "intent": in, "grants": grants}, nil); err != nil {
		return "", err
	}
	if err := v.Put(vault.Epochs, epochKeyName(conv, 1), key); err != nil {
		return "", err
	}
	e.SyncAll()
	return conv, nil
}

// admitDevices gives a conversation's keys to participants' devices approved in the family since it began (a
// linked phone; a recovered device once the family re-approved it). Any participant holding the key does it.
func (e *Engine) admitDevices(v *vault.Vault, space string) {
	var sv spaceView
	if v.Get(vault.State, "space/"+space, &sv) != nil || sv.Space.Parent == "" || len(sv.FamilyKeyed) == 0 {
		return
	}
	keyed := map[string]bool{}
	for _, d := range sv.KeyedDevices {
		keyed[d] = true
	}
	var missing []string
	for _, d := range sv.FamilyKeyed {
		if !keyed[d] {
			missing = append(missing, d)
		}
	}
	if len(missing) == 0 {
		return
	}
	key, ok := e.epochKey(v, space, sv.Space.Epoch)
	if !ok {
		return
	}
	defer pqc.Wipe(key)
	e.mu.Lock()
	dev := e.dev
	e.mu.Unlock()
	notAfter := map[string]time.Time{}
	for _, m := range sv.Members {
		notAfter[m.Person] = m.NotAfter
	}
	var grants []keys.EpochGrant
	for _, id := range missing {
		d, err := e.cert(v, id)
		if err != nil {
			continue
		}
		if g, err := keys.Grant(dev, d, space, sv.Space.Epoch, key, false, notAfter[d.PersonID], time.Now()); err == nil {
			grants = append(grants, *g)
		}
	}
	if len(grants) == 0 {
		return
	}
	in, err := e.intent(audit.Intent{SpaceID: space, Action: audit.KeysGrant, Detail: "person=" + e.person})
	if err != nil {
		return
	}
	_ = e.call("POST", "/api/v1/spaces/"+space+"/keys", map[string]any{"epoch": sv.Space.Epoch, "grants": grants, "intent": in}, nil)
}

// SetPermissions changes what someone this parent invited may do in the family (recorded in the audit log).
func (e *Engine) SetPermissions(family, person string, p Perms) error {
	in, err := e.intent(audit.Intent{SpaceID: family, Action: audit.MemberPerms, Detail: "person=" + person})
	if err != nil {
		return err
	}
	if err := e.call("POST", "/api/v1/spaces/"+family+"/members/"+person+"/permissions", map[string]any{"perms": p, "intent": in}, nil); err != nil {
		return err
	}
	e.syncSpace(family)
	return nil
}
