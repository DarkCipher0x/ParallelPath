package app

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"pp/core/envelope"
)

// The info bank is the family's shared reference: the children, and facts about each (doctor, school, allergies,
// medication, sizes, contacts...). Entries are records (kind "record") that either parent can update. An update is
// a new record that points (Ref) at the one it replaces and says why, so the whole history stays, whoever changed
// it. If both parents update the same entry at once, both versions show until one is updated again.

// InfoData is a child or an info-bank entry.
type InfoData struct {
	Type    string `json:"type"`              // child | info
	Child   string `json:"child,omitempty"`   // info: the child's record chain ID ("" = the whole family)
	Section string `json:"section,omitempty"` // info: medical, school, contacts, clothing, routines, documents, other
	Label   string `json:"label,omitempty"`   // info: "GP", "Shoe size"...; child: the child's name
	Value   string `json:"value,omitempty"`   // info: the detail; child: date of birth (yyyy-mm-dd, optional)
	Why     string `json:"why,omitempty"`     // why it was updated or removed (updates only)
	Removed bool   `json:"removed,omitempty"`
}

var infoSections = map[string]bool{"medical": true, "school": true, "contacts": true, "clothing": true, "routines": true,
	"documents": true, "other": true}

// InfoVersion is one version of an entry.
type InfoVersion struct {
	ID      string    `json:"id"`
	Person  string    `json:"person"`
	Name    string    `json:"name"`
	Time    time.Time `json:"time"`
	Label   string    `json:"label"`
	Value   string    `json:"value"`
	Note    string    `json:"note,omitempty"`
	Why     string    `json:"why,omitempty"`
	Removed bool      `json:"removed,omitempty"`
}

// InfoEntry is the current version of an entry (or child), with its history.
type InfoEntry struct {
	ID          string           `json:"id"`    // this version's item (pass it to update or remove)
	Chain       string           `json:"chain"` // the first version's item: stable for the entry's life
	Type        string           `json:"type"`
	Child       string           `json:"child,omitempty"`
	Section     string           `json:"section,omitempty"`
	Label       string           `json:"label"`
	Value       string           `json:"value"`
	Note        string           `json:"note,omitempty"`
	Attachments []AttachmentView `json:"attachments,omitempty"`
	ByName      string           `json:"by_name"`
	Mine        bool             `json:"mine"`
	Updated     time.Time        `json:"updated"`
	Conflict    bool             `json:"conflict,omitempty"` // updated by two people at once
	History     []InfoVersion    `json:"history"`            // oldest first
}

// InfoView is the whole info bank.
type InfoView struct {
	Children []InfoEntry `json:"children"`
	Entries  []InfoEntry `json:"entries"`
	Removed  []InfoEntry `json:"removed"` // removed entries and children, kept for the record
}

// InfoInput adds or updates an entry or a child. Replaces is the version being updated (empty for a new one).
type InfoInput struct {
	Space    string      `json:"space"`
	Replaces string      `json:"replaces,omitempty"`
	Why      string      `json:"why,omitempty"`
	Type     string      `json:"type"`
	Child    string      `json:"child,omitempty"`
	Section  string      `json:"section,omitempty"`
	Label    string      `json:"label"`
	Value    string      `json:"value"`
	Note     string      `json:"note,omitempty"`
	Files    []FileInput `json:"files,omitempty"`
	Remove   bool        `json:"remove,omitempty"`
}

// SaveInfo adds an entry or child, or updates/removes the current version of one (any parent; a reason is needed
// to update or remove, and kept).
func (e *Engine) SaveInfo(in InfoInput) (string, error) {
	sv, err := e.spaceView(in.Space)
	if err != nil {
		return "", err
	}
	if sv.Me.Role != "parent" {
		return "", errors.New("only parents can change the info bank")
	}
	d := InfoData{Type: in.Type, Child: in.Child, Section: in.Section, Label: strings.TrimSpace(in.Label), Value: strings.TrimSpace(in.Value),
		Why: strings.TrimSpace(in.Why), Removed: in.Remove}
	note := strings.TrimSpace(in.Note)
	var prev *InfoEntry
	if in.Replaces != "" {
		v, err := e.InfoBank(in.Space)
		if err != nil {
			return "", err
		}
		for _, list := range [][]InfoEntry{v.Children, v.Entries} {
			for i := range list {
				if list[i].ID == in.Replaces {
					prev = &list[i]
				}
			}
		}
		if prev == nil {
			return "", errors.New("that entry has changed since you opened it - look again and retry")
		}
		if d.Why == "" {
			return "", errors.New("say why it's changing - it's kept with the history")
		}
		d.Type = prev.Type
		if d.Removed {
			d.Child, d.Section, d.Label, d.Value, note = prev.Child, prev.Section, prev.Label, prev.Value, prev.Note
		}
	} else if in.Remove {
		return "", errors.New("choose what to remove")
	}
	switch d.Type {
	case "child":
		d.Child, d.Section = "", ""
		if d.Label == "" || utf8.RuneCountInString(d.Label) > 60 {
			return "", errors.New("enter the child's name")
		}
		if d.Value != "" {
			if _, err := time.Parse("2006-01-02", d.Value); err != nil {
				return "", errors.New("the date of birth isn't valid")
			}
		}
	case "info":
		if !infoSections[d.Section] {
			return "", errors.New("choose a section")
		}
		if d.Label == "" || utf8.RuneCountInString(d.Label) > 80 {
			return "", errors.New("give it a label, such as \"GP\" or \"Shoe size\"")
		}
		if utf8.RuneCountInString(d.Value) > 2000 {
			return "", errors.New("that's too long")
		}
	default:
		return "", errors.New("choose a child or an entry")
	}
	if utf8.RuneCountInString(note) > 4000 || utf8.RuneCountInString(d.Why) > envelope.MaxReason {
		return "", errors.New("that's too long")
	}
	var atts []envelope.Attachment
	if prev != nil { // an update keeps the files it had; new ones are added
		if cur, err := e.latest(in.Space, prev.ID); err == nil && cur.body != nil {
			atts = cur.body.Attachments
		}
	}
	if !in.Remove {
		for _, f := range in.Files {
			a, err := e.uploadFile(in.Space, f)
			if err != nil {
				return "", err
			}
			atts = append(atts, *a)
		}
	}
	it, err := e.post(in.Space, envelope.Draft{Kind: envelope.KindInfo, Ref: in.Replaces,
		Body: envelope.Body{Text: note, Data: jsonData(d), Attachments: atts}})
	if err != nil {
		return "", err
	}
	return it.ItemID, nil
}

// InfoBank works out the current version of every child and entry, with their histories.
func (e *Engine) InfoBank(space string) (*InfoView, error) {
	all, err := e.loadDecoded(space)
	if err != nil {
		return nil, err
	}
	names := e.names(all)
	type node struct {
		d    *decoded
		data InfoData
	}
	nodes := map[string]*node{}
	var order []string
	replaced := map[string]bool{}
	for _, d := range all {
		it := &d.rec.Item
		if (it.Kind != envelope.KindInfo && it.Kind != envelope.KindRecord) || it.Rev != 0 || d.body == nil || d.body.Retracted {
			continue // info versions are never revised: an update is a new record
		}
		var data InfoData
		if json.Unmarshal(d.body.Data, &data) != nil || (data.Type != "child" && data.Type != "info") {
			continue
		}
		nodes[it.ItemID] = &node{d: d, data: data}
		order = append(order, it.ItemID)
		if it.Ref != "" {
			replaced[it.Ref] = true
		}
	}
	// Each version's chain: follow Ref back to the first version.
	chainOf := func(id string) (string, []string) {
		var path []string
		for seen := 0; id != "" && seen < 10_000; seen++ {
			n := nodes[id]
			if n == nil {
				break
			}
			path = append([]string{id}, path...)
			if n.d.rec.Item.Ref == "" {
				break
			}
			id = n.d.rec.Item.Ref
		}
		if len(path) == 0 {
			return "", nil
		}
		return path[0], path
	}
	view := &InfoView{Children: []InfoEntry{}, Entries: []InfoEntry{}, Removed: []InfoEntry{}}
	heads := map[string]int{} // chain -> number of current versions (more than one = conflict)
	var cur []InfoEntry
	for _, id := range order {
		if replaced[id] {
			continue
		}
		n := nodes[id]
		chain, path := chainOf(id)
		en := InfoEntry{ID: id, Chain: chain, Type: n.data.Type, Child: n.data.Child, Section: n.data.Section, Label: n.data.Label,
			Value: n.data.Value, Note: n.d.body.Text, ByName: names.of(n.d.rec.Item.Person), Mine: n.d.rec.Item.Person == e.person,
			Updated: n.d.rec.Item.Created}
		for _, a := range n.d.body.Attachments {
			en.Attachments = append(en.Attachments, AttachmentView{BlobID: a.BlobID, Name: a.Name, Mime: a.Mime, Size: a.Size,
				Width: a.Width, Height: a.Height, Thumb: a.Thumb})
		}
		for _, p := range path {
			pn := nodes[p]
			en.History = append(en.History, InfoVersion{ID: p, Person: pn.d.rec.Item.Person, Name: names.of(pn.d.rec.Item.Person),
				Time: pn.d.rec.Item.Created, Label: pn.data.Label, Value: pn.data.Value, Note: pn.d.body.Text, Why: pn.data.Why,
				Removed: pn.data.Removed})
		}
		heads[chain]++
		if n.data.Removed {
			view.Removed = append(view.Removed, en)
			continue
		}
		cur = append(cur, en)
	}
	for _, en := range cur {
		en.Conflict = heads[en.Chain] > 1
		if en.Type == "child" {
			view.Children = append(view.Children, en)
		} else {
			view.Entries = append(view.Entries, en)
		}
	}
	sort.SliceStable(view.Children, func(i, j int) bool {
		return strings.ToLower(view.Children[i].Label) < strings.ToLower(view.Children[j].Label)
	})
	sort.SliceStable(view.Entries, func(i, j int) bool {
		a, b := view.Entries[i], view.Entries[j]
		if a.Section != b.Section {
			return a.Section < b.Section
		}
		return strings.ToLower(a.Label) < strings.ToLower(b.Label)
	})
	sort.SliceStable(view.Removed, func(i, j int) bool { return view.Removed[i].Updated.After(view.Removed[j].Updated) })
	return view, nil
}
