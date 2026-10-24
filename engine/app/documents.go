package app

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"pp/core/audit"
	"pp/core/envelope"
)

// Documents are family items of kind "file": folders, and files with an encrypted attachment. A new version of a
// file, a rename or a move is a new revision of it (by whoever added it, with a reason), so every version stays
// readable; removing it is a retraction (also with a reason). Opening a file is recorded in the audit log, and the
// apps show who opened what, and when.

// DocData is a folder or file's content. Folder is the path of the folder it's in ("" is the top level; parts are
// joined with "/").
type DocData struct {
	Type   string `json:"type"` // folder | file
	Folder string `json:"folder"`
	Name   string `json:"name"`
}

// DocVersion is one version of a file.
type DocVersion struct {
	Rev    uint64         `json:"rev"`
	Time   time.Time      `json:"time"`
	Reason string         `json:"reason,omitempty"`
	File   AttachmentView `json:"file"`
}

// DocAccess is someone opening a file (from the verified audit log).
type DocAccess struct {
	Person string    `json:"person"`
	Name   string    `json:"name"`
	Time   time.Time `json:"time"`
	Rev    uint64    `json:"rev"` // which version
}

// Doc is a folder or a file, as the Documents screen shows it.
type Doc struct {
	ID         string          `json:"id"`
	Rev        uint64          `json:"rev"`
	Type       string          `json:"type"`
	Folder     string          `json:"folder"`
	Name       string          `json:"name"`
	Path       string          `json:"path"` // Folder/Name
	Note       string          `json:"note,omitempty"`
	Author     string          `json:"author"`
	AuthorName string          `json:"author_name"`
	Mine       bool            `json:"mine"`
	Created    time.Time       `json:"created"`
	Updated    time.Time       `json:"updated"`
	Removed    bool            `json:"removed"`
	Reason     string          `json:"reason,omitempty"`
	File       *AttachmentView `json:"file,omitempty"` // the current version
	Versions   []DocVersion    `json:"versions,omitempty"`
	Opened     []DocAccess     `json:"opened,omitempty"`
	History    []Revision      `json:"history"`
	Implied    bool            `json:"implied,omitempty"` // a folder that only exists because files are in it
}

// DocInput adds a folder or a file, or (with Item and a reason) renames, moves or replaces one of yours.
type DocInput struct {
	Space  string     `json:"space"`
	Item   string     `json:"item,omitempty"`
	Reason string     `json:"reason,omitempty"`
	Type   string     `json:"type"` // folder | file (ignored when changing)
	Folder string     `json:"folder"`
	Name   string     `json:"name,omitempty"` // defaults to the file's own name
	Note   string     `json:"note,omitempty"`
	File   *FileInput `json:"file,omitempty"` // a file (new, or a new version)
}

func cleanFolder(f string) (string, error) {
	var parts []string
	for _, p := range strings.Split(strings.ReplaceAll(f, "\\", "/"), "/") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "." || p == ".." || utf8.RuneCountInString(p) > 80 || strings.ContainsAny(p, "<>:\"|?*") {
			return "", errors.New("that folder name can't be used")
		}
		parts = append(parts, p)
	}
	if len(parts) > 12 {
		return "", errors.New("folders can be nested 12 deep at most")
	}
	return strings.Join(parts, "/"), nil
}

func joinPath(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

// SaveDocument adds a folder or file, or changes one of yours (a new version, a new name or folder) with a reason.
func (e *Engine) SaveDocument(in DocInput) (string, error) {
	folder, err := cleanFolder(in.Folder)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(in.Note) > 4000 {
		return "", errors.New("the note is too long")
	}
	var prev *decoded
	var data DocData
	if in.Item != "" {
		if prev, err = e.latest(in.Space, in.Item); err != nil {
			return "", err
		}
		if prev.rec.Item.Kind != envelope.KindFile || prev.body == nil || json.Unmarshal(prev.body.Data, &data) != nil {
			return "", errors.New("that isn't a document")
		}
	} else {
		data.Type = in.Type
		if data.Type != "folder" && data.Type != "file" {
			return "", errors.New("choose a folder or a file")
		}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" && in.File != nil && in.Item == "" {
		name = in.File.Name
		if name == "" {
			name = in.File.Path
		}
	}
	if name == "" && prev != nil {
		name = data.Name
	}
	name = cleanName(name)
	if data.Type == "folder" {
		if name, err = cleanFolder(name); err != nil || name == "" || strings.Contains(name, "/") {
			return "", errors.New("give the folder a name (no slashes)")
		}
	}
	if data.Type == "file" && in.Item == "" && in.File == nil {
		return "", errors.New("choose a file")
	}

	// No two live things with the same name in one folder.
	docs, err := e.Documents(in.Space)
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		if d.ID != in.Item && !d.Removed && !d.Implied && d.Folder == folder && strings.EqualFold(d.Name, name) {
			return "", errors.New("there's already something called " + name + " in that folder")
		}
	}
	if data.Type == "folder" && prev != nil && joinPath(folder, name) != joinPath(data.Folder, data.Name) {
		for _, d := range docs {
			if !d.Removed && strings.HasPrefix(d.Folder+"/", joinPath(data.Folder, data.Name)+"/") {
				return "", errors.New("move or remove what's in the folder first")
			}
		}
	}

	body := envelope.Body{Text: strings.TrimSpace(in.Note)}
	if prev != nil {
		body.Attachments = prev.body.Attachments
	}
	if in.File != nil {
		if data.Type != "file" {
			return "", errors.New("folders don't hold a file of their own")
		}
		a, err := e.uploadFile(in.Space, *in.File)
		if err != nil {
			return "", err
		}
		body.Attachments = []envelope.Attachment{*a}
	}
	data.Folder, data.Name = folder, name
	body.Data = jsonData(data)
	if prev == nil {
		it, err := e.post(in.Space, envelope.Draft{Kind: envelope.KindFile, Body: body})
		if err != nil {
			return "", err
		}
		return it.ItemID, nil
	}
	body.Reason = strings.TrimSpace(in.Reason)
	return in.Item, e.reviseKind(in.Space, in.Item, envelope.KindFile, body)
}

// RemoveDocument removes one of your files or (empty) folders. It stays in the record, marked removed, with the
// reason, and its versions can still be opened.
func (e *Engine) RemoveDocument(space, item, reason string) error {
	docs, err := e.Documents(space)
	if err != nil {
		return err
	}
	for _, d := range docs {
		if d.ID == item && d.Type == "folder" {
			for _, c := range docs {
				if !c.Removed && strings.HasPrefix(c.Folder+"/", d.Path+"/") {
					return errors.New("the folder isn't empty")
				}
			}
		}
	}
	return e.reviseKind(space, item, envelope.KindFile, envelope.Body{Retracted: true, Reason: strings.TrimSpace(reason)})
}

// Documents lists every folder and file (removed ones too, marked), with versions and who opened them.
func (e *Engine) Documents(space string) ([]Doc, error) {
	all, err := e.loadDecoded(space)
	if err != nil {
		return nil, err
	}
	names := e.names(all)
	threads, order := threadsOf(all, envelope.KindFile)

	// Who opened which file version: blob downloads in the verified audit log.
	opens := map[string][]DocAccess{}
	e.eachAudit(space, func(en *audit.Entry) {
		if en.Action == audit.BlobDownload {
			opens[en.ItemID] = append(opens[en.ItemID], DocAccess{Person: en.Person, Name: names.of(en.Person), Time: en.Time})
		}
	})

	out := []Doc{}
	folders := map[string]bool{}
	for _, id := range order {
		t := threads[id]
		first := t.revs[0].rec.Item
		cur := t.latest()
		it := &cur.rec.Item
		d := Doc{ID: id, Rev: it.Rev, Author: first.Person, AuthorName: names.of(first.Person), Mine: first.Person == e.person,
			Created: first.Created, Updated: it.Created, History: history(t)}
		var data DocData
		var live *envelope.Body
		for i := len(t.revs) - 1; i >= 0; i-- { // the last readable, non-retracted content
			if b := t.revs[i].body; b != nil && !b.Retracted && json.Unmarshal(b.Data, &data) == nil {
				live = b
				break
			}
		}
		if live == nil || (data.Type != "folder" && data.Type != "file") {
			continue
		}
		d.Type, d.Folder, d.Name, d.Path, d.Note = data.Type, data.Folder, data.Name, joinPath(data.Folder, data.Name), live.Text
		if cur.body != nil {
			d.Removed, d.Reason = cur.body.Retracted, cur.body.Reason
		}
		if d.Type == "file" {
			seen := map[string]bool{}
			for _, r := range t.revs {
				if r.body == nil || r.body.Retracted || len(r.body.Attachments) == 0 {
					continue
				}
				a := r.body.Attachments[0]
				if seen[a.BlobID] {
					continue // a rename or move: same file
				}
				seen[a.BlobID] = true
				av := AttachmentView{BlobID: a.BlobID, Name: a.Name, Mime: a.Mime, Size: a.Size, Width: a.Width, Height: a.Height, Thumb: a.Thumb}
				d.Versions = append(d.Versions, DocVersion{Rev: r.rec.Item.Rev, Time: r.rec.Item.Created, Reason: r.body.Reason, File: av})
				for _, o := range opens[a.BlobID] {
					o.Rev = r.rec.Item.Rev
					d.Opened = append(d.Opened, o)
				}
			}
			if n := len(d.Versions); n > 0 {
				f := d.Versions[n-1].File
				d.File = &f
			}
			sort.Slice(d.Opened, func(i, j int) bool { return d.Opened[i].Time.After(d.Opened[j].Time) })
		} else if !d.Removed {
			folders[d.Path] = true
		}
		out = append(out, d)
	}
	// Folders that only exist because something live is in them (e.g. a folder item that was removed later).
	for _, d := range out {
		if d.Removed {
			continue
		}
		parts := strings.Split(d.Folder, "/")
		for i := range parts {
			p := strings.Join(parts[:i+1], "/")
			if p == "" || folders[p] {
				continue
			}
			folders[p] = true
			parent := strings.Join(parts[:i], "/")
			out = append(out, Doc{ID: "", Type: "folder", Folder: parent, Name: parts[i], Path: p, Implied: true, History: []Revision{}})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "folder"
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	return out, nil
}
