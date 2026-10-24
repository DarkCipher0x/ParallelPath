package e2etest

import (
	"os"
	"strings"
	"testing"

	"pp/engine/app"
)

// The file explorer's operations: folders move and copy with what's in them, copies go to another family (only
// the server's operator lets someone start a second one), several things are removed at once, removed things come
// back, and files shared in messages are listed and filed into the documents.
func TestFileExplorer(t *testing.T) {
	ts := startServer(t)
	inv, _, _ := ts.svc.CreateServerInvite()
	alice, ares := setup(t, ts, inv, "Alice", "windows")
	space := ares.SpaceID
	code := call[map[string]any](t, alice, "invite", `{"space":"`+space+`","role":"parent"}`)["code"].(string)
	bob, bres := setup(t, ts, code, "Bob", "android")
	waitFor(t, "alice sees bob waiting", func() bool {
		alice.SyncAll()
		s, _ := alice.Spaces()
		return len(s) == 1 && len(s[0].Members) == 2
	})
	call[map[string]bool](t, alice, "approve", `{"space":"`+space+`","person":"`+bres.Person+`"}`)
	waitFor(t, "bob has keys", func() bool { bob.SyncAll(); s, _ := bob.Spaces(); return len(s) == 1 && !s[0].Waiting })

	// Neither parent can bring another family onto the server.
	for _, e := range []*app.Engine{alice, bob} {
		if a, err := e.Account(); err != nil || a.CanStartFamily {
			t.Fatalf("may start a family: %+v %v", a, err)
		}
		if _, err := e.CreateSpace("Another family"); err == nil {
			t.Fatal("a parent started a family")
		}
	}

	docs := func(e *app.Engine, sp string) map[string]app.Doc {
		e.SyncAll()
		list, err := e.Documents(sp)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]app.Doc{}
		for _, d := range list {
			if !d.Removed {
				m[d.Path] = d
			}
		}
		return m
	}
	save := func(e *app.Engine, folder, name, text string) string {
		id, err := e.SaveDocument(app.DocInput{Space: space, Type: "file", Folder: folder, File: &app.FileInput{Path: writeFile(t, name, text)}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	if _, err := alice.SaveDocument(app.DocInput{Space: space, Type: "folder", Name: "School"}); err != nil {
		t.Fatal(err)
	}
	report := save(alice, "School/Reports", "report.txt", "reading is excellent")
	save(alice, "School", "letter.txt", "trip on Friday")

	// Rename a folder with everything in it (one reason for all).
	if err := alice.MoveDocument(space, "", "School", "", "Primary school", ""); err == nil {
		t.Fatal("a move without a reason")
	}
	if err := alice.MoveDocument(space, "", "School", "School/Reports", "", "tidy"); err == nil {
		t.Fatal("a folder moved into itself")
	}
	if err := alice.MoveDocument(space, "", "School", "", "Primary school", "New name"); err != nil {
		t.Fatal(err)
	}
	m := docs(alice, space)
	if _, ok := m["Primary school/Reports/report.txt"]; !ok || m["Primary school/letter.txt"].ID == "" {
		t.Fatalf("after the folder rename: %v", paths(m))
	}
	if _, ok := m["School"]; ok {
		t.Fatal("the old folder is still there")
	}

	// Bob adds a file in Alice's folder: now she can't move the folder (his file is his to move).
	save(bob, "Primary school", "bob.txt", "dentist letter")
	waitFor(t, "alice sees bob's file", func() bool { _, ok := docs(alice, space)["Primary school/bob.txt"]; return ok })
	if err := alice.MoveDocument(space, "", "Primary school", "", "School", "rename"); err == nil || !strings.Contains(err.Error(), "Bob") {
		t.Fatalf("moved a folder with someone else's file in it: %v", err)
	}

	// Copy within the family: same encrypted file, a new document that's Bob's (he may then move it).
	if err := bob.CopyDocument(space, report, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := bob.CopyDocument(space, report, "", "", ""); err != nil {
		t.Fatal(err)
	}
	m = docs(bob, space)
	c1, c2 := m["report.txt"], m["report (copy).txt"]
	if !c1.Mine || !c2.Mine || c1.File == nil || c1.File.BlobID != m["Primary school/Reports/report.txt"].File.BlobID {
		t.Fatalf("copies: %v", paths(m))
	}
	if p, err := bob.OpenAttachment(space, c2.ID, c2.File.BlobID); err != nil {
		t.Fatal(err)
	} else if b, _ := os.ReadFile(p); string(b) != "reading is excellent" {
		t.Fatal("copy content")
	}
	// A folder copy brings what's in it.
	if err := alice.CopyDocument(space, "", "Primary school/Reports", "", "Archive"); err != nil {
		t.Fatal(err)
	}
	if _, ok := docs(alice, space)["Archive/Reports/report.txt"]; !ok {
		t.Fatalf("folder copy: %v", paths(docs(alice, space)))
	}

	// Remove several at once (only your own), then restore one.
	if err := alice.RemoveDocuments(space, nil, []string{"Primary school"}, "Cleaning up"); err == nil {
		t.Fatal("removed a folder with bob's file in it")
	}
	if err := bob.RemoveDocuments(space, []string{c1.ID, c2.ID}, nil, "Duplicates"); err != nil {
		t.Fatal(err)
	}
	if err := alice.RemoveDocuments(space, nil, []string{"Archive"}, "Not needed"); err != nil {
		t.Fatal(err)
	}
	m = docs(alice, space)
	for p := range m {
		if strings.HasPrefix(p, "Archive") || p == "report.txt" || p == "report (copy).txt" {
			t.Fatalf("%s still there", p)
		}
	}
	if err := alice.RestoreDocument(space, c1.ID, "Needed it"); err == nil {
		t.Fatal("alice restored bob's file")
	}
	if err := bob.RestoreDocument(space, c1.ID, "Needed it after all"); err != nil {
		t.Fatal(err)
	}
	if _, ok := docs(bob, space)["report.txt"]; !ok {
		t.Fatal("not restored")
	}

	// The operator lets Alice start a second family; she copies a folder into it (encrypted again for that family).
	if err := ts.svc.AdminSetFamiliesLeft(ares.Person, 1, "test"); err != nil {
		t.Fatal(err)
	}
	other, err := alice.CreateSpace("Second family")
	if err != nil {
		t.Fatal(err)
	}
	if err := alice.CopyDocument(space, "", "Primary school/Reports", other, "From the first family"); err != nil {
		t.Fatal(err)
	}
	m = docs(alice, other)
	d, ok := m["From the first family/Reports/report.txt"]
	if !ok || d.File == nil || d.File.BlobID == docs(alice, space)["Primary school/Reports/report.txt"].File.BlobID {
		t.Fatalf("copy to another family: %v", paths(m))
	}
	if p, err := alice.OpenAttachment(other, d.ID, d.File.BlobID); err != nil {
		t.Fatal(err)
	} else if b, _ := os.ReadFile(p); string(b) != "reading is excellent" {
		t.Fatal("cross-family copy content")
	}

	// Files shared in messages are listed and can be filed into the documents.
	if err := bob.Send(app.SendRequest{Space: space, Text: "The new timetable", Files: []app.FileInput{{Path: writeFile(t, "timetable.txt", "Mon: swim")}}}); err != nil {
		t.Fatal(err)
	}
	var shared []app.SharedFile
	waitFor(t, "alice sees the shared file", func() bool {
		alice.SyncAll()
		shared, _ = alice.SharedFiles(space)
		return len(shared) == 1
	})
	if shared[0].AuthorName != "Bob" || shared[0].File.Name != "timetable.txt" || shared[0].Mine {
		t.Fatalf("shared: %+v", shared[0])
	}
	if _, err := alice.SaveSharedFile(space, shared[0].Space, shared[0].Message, shared[0].File.BlobID, "Primary school"); err != nil {
		t.Fatal(err)
	}
	if _, ok := docs(alice, space)["Primary school/timetable.txt"]; !ok {
		t.Fatal("the shared file wasn't filed")
	}
}

func paths(m map[string]app.Doc) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
