package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"pp/core/envelope"
)

// The file explorer's operations beyond adding and changing one document: moving or renaming a folder with what's
// in it, copying (within the family, or into another family you're in), removing a folder with its contents,
// restoring something removed, and the files shared in messages. Everything stays in the record: a move or rename
// is a new revision (with a reason), a removal is a retraction, and only the person who added something can change it.

const maxBatch = 500 // the most documents one operation touches

// MoveDocument renames and/or moves a file or folder (a folder with everything in it). Only what you added can be
// moved: if someone else has something in the folder, they move theirs first. An implied folder (one that exists
// only because things are in it) is moved by moving its contents.
func (e *Engine) MoveDocument(space, item, folderPath, toFolder, name, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("a reason is required - it's kept with the history")
	}
	docs, err := e.Documents(space)
	if err != nil {
		return err
	}
	d := findDoc(docs, item, folderPath)
	if d == nil || d.Removed {
		return errors.New("that document isn't there any more")
	}
	if d.Type == "file" {
		_, err := e.SaveDocument(DocInput{Space: space, Item: d.ID, Folder: toFolder, Name: name, Note: d.Note, Reason: reason})
		return err
	}
	to, err := cleanFolder(toFolder)
	if err != nil {
		return err
	}
	if name = strings.TrimSpace(name); name == "" {
		name = d.Name
	}
	if n, err := cleanFolder(name); err != nil || n == "" || strings.Contains(n, "/") {
		return errors.New("give the folder a name (no slashes)")
	}
	oldPath, newPath := d.Path, joinPath(to, name)
	if newPath == oldPath {
		return nil
	}
	if strings.HasPrefix(newPath+"/", oldPath+"/") {
		return errors.New("a folder can't go inside itself")
	}
	for _, x := range docs {
		if !x.Removed && x.Folder == to && strings.EqualFold(x.Name, name) && x.Path != oldPath {
			return errors.New("there's already something called " + name + " in that folder")
		}
	}
	inside := descendants(docs, oldPath)
	if len(inside) > maxBatch {
		return fmt.Errorf("that folder holds more than %d things - move them in parts", maxBatch)
	}
	for _, x := range append(inside, *d) {
		if x.ID != "" && !x.Mine {
			who := x.AuthorName
			if who == "" {
				who = "someone else"
			}
			return fmt.Errorf("%s added %q - only they can move it, so move or ask about that first", who, x.Name)
		}
	}
	// The folder itself first (when it's a real item), then everything in it, deepest last.
	if d.ID != "" {
		if err := e.reviseDoc(space, d.ID, to, name, reason); err != nil {
			return err
		}
	}
	for _, x := range inside {
		if x.ID == "" {
			continue
		}
		if err := e.reviseDoc(space, x.ID, newPath+strings.TrimPrefix(x.Folder, oldPath), x.Name, reason); err != nil {
			return err
		}
	}
	return nil
}

// RemoveDocuments removes files and folders (a folder with everything in it) that you added, with one reason.
func (e *Engine) RemoveDocuments(space string, items, folders []string, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("a reason is required - it's kept with the history")
	}
	docs, err := e.Documents(space)
	if err != nil {
		return err
	}
	var todo []Doc
	seen := map[string]bool{}
	add := func(x Doc) {
		if x.ID != "" && !x.Removed && !seen[x.ID] {
			seen[x.ID] = true
			todo = append(todo, x)
		}
	}
	for _, id := range items {
		if d := findDoc(docs, id, ""); d != nil {
			if d.Type == "folder" {
				folders = append(folders, d.Path)
			} else {
				add(*d)
			}
		}
	}
	for _, p := range folders {
		d := findDoc(docs, "", p)
		if d == nil {
			continue
		}
		for _, x := range descendants(docs, p) {
			add(x)
		}
		add(*d)
	}
	if len(todo) > maxBatch {
		return fmt.Errorf("that's more than %d things - remove them in parts", maxBatch)
	}
	for _, x := range todo {
		if !x.Mine {
			return fmt.Errorf("%s added %q - only they can remove it", x.AuthorName, x.Name)
		}
	}
	// Deepest first, so a folder is empty when it goes.
	sort.SliceStable(todo, func(i, j int) bool { return strings.Count(todo[i].Path, "/") > strings.Count(todo[j].Path, "/") })
	for _, x := range todo {
		if err := e.reviseKind(space, x.ID, envelope.KindFile, envelope.Body{Retracted: true, Reason: reason}); err != nil {
			return err
		}
	}
	return nil
}

// RestoreDocument brings back something you removed, as it was (its folder must still be free of the same name).
// The removal and the restore both stay in its history.
func (e *Engine) RestoreDocument(space, item, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("a reason is required - it's kept with the history")
	}
	all, err := e.loadDecoded(space)
	if err != nil {
		return err
	}
	threads, _ := threadsOf(all, envelope.KindFile)
	t := threads[item]
	if t == nil || len(t.revs) == 0 {
		return errors.New("that document isn't there any more")
	}
	cur := t.latest()
	if cur.rec.Item.Person != e.person {
		return errors.New("only the person who added it can restore it")
	}
	if cur.body == nil || !cur.body.Retracted {
		return errors.New("it isn't removed")
	}
	var live *envelope.Body
	var data DocData
	for i := len(t.revs) - 1; i >= 0; i-- {
		if b := t.revs[i].body; b != nil && !b.Retracted && json.Unmarshal(b.Data, &data) == nil {
			live = b
			break
		}
	}
	if live == nil {
		return errors.New("there's nothing to restore")
	}
	docs, err := e.Documents(space)
	if err != nil {
		return err
	}
	for _, x := range docs {
		if x.ID != item && !x.Removed && !x.Implied && x.Folder == data.Folder && strings.EqualFold(x.Name, data.Name) {
			return errors.New("there's something else called " + data.Name + " there now - rename it first")
		}
	}
	b := *live
	b.Reason, b.Retracted = reason, false
	_, err = e.post(space, envelope.Draft{Prev: &cur.rec.Item, Body: b})
	return err
}

// CopyDocument copies a file or a folder (with what's in it) into a folder of this family or of another family
// you're in. The copies are yours. Within a family the encrypted files are shared, not uploaded again; into another
// family each file is decrypted on this device and encrypted again with that family's keys.
func (e *Engine) CopyDocument(space, item, folderPath, toSpace, toFolder string) error {
	if toSpace == "" {
		toSpace = space
	}
	to, err := cleanFolder(toFolder)
	if err != nil {
		return err
	}
	docs, err := e.Documents(space)
	if err != nil {
		return err
	}
	d := findDoc(docs, item, folderPath)
	if d == nil {
		return errors.New("that document isn't there any more")
	}
	if d.Type == "folder" && toSpace == space && strings.HasPrefix(to+"/", d.Path+"/") {
		return errors.New("a folder can't be copied into itself")
	}
	target := docs
	if toSpace != space {
		if target, err = e.Documents(toSpace); err != nil {
			return err
		}
	}
	taken := map[string]bool{}
	for _, x := range target {
		if !x.Removed {
			taken[strings.ToLower(x.Path)] = true
		}
	}
	// A free name in the target folder: "Report.pdf", then "Report (copy).pdf", "Report (copy 2).pdf"...
	free := func(folder, name string) string {
		ext := path.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if d.Type == "folder" || ext == name {
			base, ext = name, ""
		}
		n := name
		for i := 1; taken[strings.ToLower(joinPath(folder, n))]; i++ {
			if i == 1 {
				n = base + " (copy)" + ext
			} else {
				n = fmt.Sprintf("%s (copy %d)%s", base, i, ext)
			}
		}
		taken[strings.ToLower(joinPath(folder, n))] = true
		return n
	}
	one := func(x Doc, folder, name string) error {
		if x.Type == "folder" {
			_, err := e.SaveDocument(DocInput{Space: toSpace, Type: "folder", Folder: folder, Name: name, Note: x.Note})
			return err
		}
		if x.File == nil {
			return nil
		}
		if toSpace == space { // the same encrypted file, a new document of yours
			src, err := e.latest(space, x.ID)
			if err != nil {
				return err
			}
			var att []envelope.Attachment
			if src.body != nil && !src.body.Retracted {
				att = src.body.Attachments
			}
			if len(att) == 0 { // removed: copy the last version
				all, _ := e.loadDecoded(space)
				for _, r := range all {
					if r.rec.Item.ItemID == x.ID && r.body != nil && len(r.body.Attachments) > 0 {
						att = r.body.Attachments
					}
				}
			}
			body := envelope.Body{Text: x.Note, Attachments: att, Data: jsonData(DocData{Type: "file", Folder: folder, Name: name})}
			_, err = e.post(space, envelope.Draft{Kind: envelope.KindFile, Body: body})
			return err
		}
		p, err := e.OpenAttachment(space, x.ID, x.File.BlobID)
		if err != nil {
			return err
		}
		_, err = e.SaveDocument(DocInput{Space: toSpace, Type: "file", Folder: folder, Name: name, Note: x.Note,
			File: &FileInput{Path: p, Name: name, Mime: x.File.Mime, Thumb: x.File.Thumb, Width: x.File.Width, Height: x.File.Height}})
		return err
	}
	if d.Type == "file" {
		return one(*d, to, free(to, d.Name))
	}
	inside := descendants(docs, d.Path)
	if len(inside) > maxBatch {
		return fmt.Errorf("that folder holds more than %d things - copy it in parts", maxBatch)
	}
	root := free(to, d.Name)
	newRoot := joinPath(to, root)
	if err := one(*d, to, root); err != nil {
		return err
	}
	// Folders before files, shallow before deep, so each folder exists before what goes in it.
	sort.SliceStable(inside, func(i, j int) bool {
		if inside[i].Type != inside[j].Type {
			return inside[i].Type == "folder"
		}
		return strings.Count(inside[i].Path, "/") < strings.Count(inside[j].Path, "/")
	})
	for _, x := range inside {
		if x.Implied {
			continue // it comes back with what's in it
		}
		if err := one(x, newRoot+strings.TrimPrefix(x.Folder, d.Path), x.Name); err != nil {
			return err
		}
	}
	return nil
}

// SharedFile is a file attached to a message in the family or one of your private conversations in it.
type SharedFile struct {
	Space        string         `json:"space"`
	Conversation string         `json:"conversation,omitempty"` // its title, when it's in a private conversation
	Message      string         `json:"message"`
	Author       string         `json:"author"`
	AuthorName   string         `json:"author_name"`
	Mine         bool           `json:"mine"`
	Time         time.Time      `json:"time"`
	Text         string         `json:"text,omitempty"` // the message's text, shortened
	File         AttachmentView `json:"file"`
}

// SharedFiles lists the files shared in the family's messages and your private conversations there, newest first
// (withdrawn messages are left out).
func (e *Engine) SharedFiles(family string) ([]SharedFile, error) {
	out := []SharedFile{}
	spaces := []struct{ id, title string }{{family, ""}}
	if cs, err := e.Conversations(family); err == nil {
		for _, c := range cs {
			if !c.Waiting {
				spaces = append(spaces, struct{ id, title string }{c.ID, c.Title})
			}
		}
	}
	for _, sp := range spaces {
		all, err := e.loadDecoded(sp.id)
		if err != nil {
			if sp.id == family {
				return nil, err
			}
			continue
		}
		names := e.names(all)
		latest := map[string]*decoded{}
		first := map[string]*decoded{}
		for _, d := range all {
			it := &d.rec.Item
			if it.Kind != envelope.KindMessage || it.Ref != "" {
				continue
			}
			if first[it.ItemID] == nil {
				first[it.ItemID] = d
			}
			latest[it.ItemID] = d
		}
		for id, cur := range latest {
			if cur.body == nil || cur.body.Retracted {
				continue
			}
			f := first[id]
			text := cur.body.Text
			if r := []rune(text); len(r) > 120 {
				text = string(r[:120]) + "…"
			}
			for _, a := range cur.body.Attachments {
				out = append(out, SharedFile{Space: sp.id, Conversation: sp.title, Message: id, Author: f.rec.Item.Person,
					AuthorName: names.of(f.rec.Item.Person), Mine: f.rec.Item.Person == e.person, Time: f.rec.Item.Created, Text: text,
					File: AttachmentView{BlobID: a.BlobID, Name: a.Name, Mime: a.Mime, Size: a.Size, Width: a.Width, Height: a.Height,
						Millis: a.Millis, Thumb: a.Thumb}})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// SaveSharedFile files a copy of a file shared in a message into the family's documents.
func (e *Engine) SaveSharedFile(family, space, message, blobID, folder string) (string, error) {
	p, err := e.OpenAttachment(space, message, blobID)
	if err != nil {
		return "", err
	}
	files, err := e.SharedFiles(family)
	if err != nil {
		return "", err
	}
	in := FileInput{Path: p}
	for _, f := range files {
		if f.Space == space && f.Message == message && f.File.BlobID == blobID {
			in.Name, in.Mime, in.Thumb, in.Width, in.Height = f.File.Name, f.File.Mime, f.File.Thumb, f.File.Width, f.File.Height
		}
	}
	return e.SaveDocument(DocInput{Space: family, Type: "file", Folder: folder, File: &in})
}

// reviseDoc moves/renames one of your documents, keeping its file and note.
func (e *Engine) reviseDoc(space, item, folder, name, reason string) error {
	cur, err := e.latest(space, item)
	if err != nil {
		return err
	}
	var data DocData
	if cur.body == nil || json.Unmarshal(cur.body.Data, &data) != nil {
		return errors.New("that isn't a document")
	}
	data.Folder, data.Name = folder, name
	b := *cur.body
	b.Data, b.Reason = jsonData(data), reason
	return e.reviseKind(space, item, envelope.KindFile, b)
}

// findDoc finds a document by item ID, or a folder (real or implied) by its path.
func findDoc(docs []Doc, item, folderPath string) *Doc {
	for i := range docs {
		d := &docs[i]
		if item != "" && d.ID == item {
			return d
		}
	}
	if folderPath == "" {
		return nil
	}
	var implied *Doc
	for i := range docs {
		d := &docs[i]
		if d.Type == "folder" && d.Path == folderPath {
			if !d.Removed && !d.Implied {
				return d
			}
			if d.Implied {
				implied = d
			}
		}
	}
	return implied
}

// descendants are the live things inside a folder path, at any depth.
func descendants(docs []Doc, folderPath string) []Doc {
	var out []Doc
	for _, x := range docs {
		if !x.Removed && x.Path != folderPath && strings.HasPrefix(x.Folder+"/", folderPath+"/") {
			out = append(out, x)
		}
	}
	return out
}
