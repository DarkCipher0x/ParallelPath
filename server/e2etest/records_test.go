package e2etest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pp/core/audit"
	"pp/engine/app"
)

func writeFile(t *testing.T, name, text string) string {
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func expenseOf(t *testing.T, e *app.Engine, space, id string) (*app.ExpensesView, app.Expense) {
	t.Helper()
	e.SyncAll()
	v, err := e.Expenses(space)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range v.Expenses {
		if x.ID == id {
			return v, x
		}
	}
	return v, app.Expense{}
}

func TestExpensesEndToEnd(t *testing.T) {
	alice, bob, space, _, bobID := family(t)

	// Alice paid for school shoes and asks Bob for half, with the receipt attached.
	receipt := writeFile(t, "receipt.txt", "Shoe shop - school shoes - £48.00")
	id, err := alice.SaveExpense(app.ExpenseInput{Space: space, Note: "Mia's feet grew again", Receipts: []app.FileInput{{Path: receipt}},
		Data: app.ExpenseData{Title: "School shoes", Amount: 4800, Currency: "gbp", Date: "2026-11-02", Category: "clothing", Share: 50}})
	if err != nil {
		t.Fatal(err)
	}
	var x app.Expense
	var v *app.ExpensesView
	waitFor(t, "bob is asked", func() bool { v, x = expenseOf(t, bob, space, id); return x.NeedsMe })
	if x.State != "pending" || x.Owed != 2400 || x.Data.Currency != "GBP" || len(x.Receipts) != 1 || len(v.Balances) != 0 {
		t.Fatalf("pending expense: %+v %+v", x, v.Balances)
	}
	if path, err := bob.OpenAttachment(space, id, x.Receipts[0].BlobID); err != nil {
		t.Fatal(err)
	} else if b, _ := os.ReadFile(path); !strings.Contains(string(b), "£48.00") {
		t.Fatal("receipt content")
	}
	if bob.AnswerExpense(space, id, false, "") == nil {
		t.Fatal("a decline without a reason")
	}
	if alice.AnswerExpense(space, id, true, "") == nil {
		t.Fatal("alice answered her own expense")
	}
	if bob.PayExpense(space, id, "Bank transfer", "", "") == nil {
		t.Fatal("paid before agreeing")
	}

	// Bob agrees; now he owes half. He pays; Alice confirms it arrived: settled, nothing owed.
	if err := bob.AnswerExpense(space, id, true, "Fair enough"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "agreed for alice", func() bool { v, x = expenseOf(t, alice, space, id); return x.State == "agreed" })
	if len(v.Balances) != 1 || v.Balances[0].From != bobID || v.Balances[0].Amount != 2400 {
		t.Fatalf("balance: %+v", v.Balances)
	}
	if _, err := alice.SaveExpense(app.ExpenseInput{Space: space, Item: id, Data: x.Data}); err == nil {
		t.Fatal("an edit without a reason")
	}
	_, x = expenseOf(t, bob, space, id)
	if !x.CanPay {
		t.Fatalf("bob can pay: %+v", x)
	}
	if err := bob.PayExpense(space, id, "Bank transfer", "MIA SHOES", ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "alice sees the payment", func() bool { v, x = expenseOf(t, alice, space, id); return x.State == "paid" && x.NeedsMe })
	if len(v.Balances) != 0 {
		t.Fatalf("still owed after paying: %+v", v.Balances)
	}
	if _, err := alice.SaveExpense(app.ExpenseInput{Space: space, Item: id, Reason: "typo", Data: x.Data}); err == nil {
		t.Fatal("a paid expense was edited")
	}
	if err := alice.ConfirmPayment(space, id, bobID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "settled for bob", func() bool { _, x = expenseOf(t, bob, space, id); return x.State == "settled" })

	// A second expense: Alice changes it (new revision: Bob's earlier yes no longer counts), then withdraws it.
	id2, err := alice.SaveExpense(app.ExpenseInput{Space: space, Data: app.ExpenseData{Title: "Swimming lessons", Amount: 6000, Currency: "GBP",
		Date: "2026-11-03", Category: "activities", Share: 50}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob sees the second", func() bool { _, x = expenseOf(t, bob, space, id2); return x.NeedsMe })
	if err := bob.AnswerExpense(space, id2, true, ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "agreed", func() bool { _, x = expenseOf(t, alice, space, id2); return x.State == "agreed" })
	x.Data.Amount = 7200
	if _, err := alice.SaveExpense(app.ExpenseInput{Space: space, Item: id2, Reason: "Term price went up", Data: x.Data}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "asked again", func() bool {
		_, x = expenseOf(t, bob, space, id2)
		return x.State == "pending" && x.NeedsMe && x.Owed == 3600
	})
	if err := alice.WithdrawExpense(space, id2, "Club closed"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "withdrawn", func() bool { _, x = expenseOf(t, bob, space, id2); return x.State == "withdrawn" && !x.NeedsMe })
}

func TestDocumentsEndToEnd(t *testing.T) {
	alice, bob, space, _, bobID := family(t)
	docs := func(e *app.Engine) map[string]app.Doc {
		e.SyncAll()
		list, err := e.Documents(space)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]app.Doc{}
		for _, d := range list {
			m[d.Path] = d
		}
		return m
	}
	if _, err := alice.SaveDocument(app.DocInput{Space: space, Type: "folder", Name: "School"}); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.SaveDocument(app.DocInput{Space: space, Type: "folder", Name: "school"}); err == nil {
		t.Fatal("two folders with the same name")
	}
	report := writeFile(t, "report.txt", "Autumn report: reading is excellent")
	id, err := alice.SaveDocument(app.DocInput{Space: space, Type: "file", Folder: "School/Reports", File: &app.FileInput{Path: report},
		Note: "From the class teacher"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]app.Doc
	waitFor(t, "bob sees the file", func() bool { m = docs(bob); _, ok := m["School/Reports/report.txt"]; return ok })
	if !m["School/Reports"].Implied || m["School"].Implied || m["School"].Type != "folder" {
		t.Fatalf("folders: %+v %+v", m["School/Reports"], m["School"])
	}
	d := m["School/Reports/report.txt"]
	p, err := bob.OpenAttachment(space, id, d.File.BlobID)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "Autumn report: reading is excellent" {
		t.Fatal("content")
	}

	// A new version needs a reason; both versions stay; Bob's opening is in the access log.
	v2 := writeFile(t, "report-v2.txt", "Autumn report (corrected): reading and maths are excellent")
	if _, err := alice.SaveDocument(app.DocInput{Space: space, Item: id, Folder: "School/Reports", File: &app.FileInput{Path: v2}}); err == nil {
		t.Fatal("a new version without a reason")
	}
	if _, err := bob.SaveDocument(app.DocInput{Space: space, Item: id, Folder: "School/Reports", Reason: "mine", File: &app.FileInput{Path: v2}}); err == nil {
		t.Fatal("bob replaced alice's file")
	}
	if _, err := alice.SaveDocument(app.DocInput{Space: space, Item: id, Folder: "School/Reports", Reason: "School sent a corrected copy",
		File: &app.FileInput{Path: v2}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "two versions, bob's open logged", func() bool {
		d = docs(alice)["School/Reports/report.txt"]
		return len(d.Versions) == 2 && len(d.Opened) == 1 && d.Opened[0].Person == bobID && d.Opened[0].Rev == 0
	})
	if d.Versions[1].Reason != "School sent a corrected copy" || d.File.BlobID != d.Versions[1].File.BlobID {
		t.Fatalf("versions: %+v", d.Versions)
	}

	// Move (same file, not a new version), then remove: it stays in the record, marked, and can still be opened.
	if _, err := alice.SaveDocument(app.DocInput{Space: space, Item: id, Folder: "School", Name: "Autumn report.txt", Reason: "Tidying up"}); err != nil {
		t.Fatal(err)
	}
	if err := alice.RemoveDocument(space, id, "Superseded by the spring report"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "removed", func() bool { d = docs(bob)["School/Autumn report.txt"]; return d.Removed })
	if len(d.Versions) != 2 || d.Reason != "Superseded by the spring report" {
		t.Fatalf("removed doc: %+v", d)
	}
	if _, err := bob.OpenAttachment(space, id, d.Versions[0].File.BlobID); err != nil {
		t.Fatal(err)
	}
	if _, ok := docs(bob)["School/Reports"]; ok {
		t.Fatal("an empty implied folder is still shown")
	}
}

func TestInfoBankEndToEnd(t *testing.T) {
	alice, bob, space, _, _ := family(t)
	info := func(e *app.Engine) *app.InfoView {
		e.SyncAll()
		v, err := e.InfoBank(space)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	mia, err := alice.SaveInfo(app.InfoInput{Space: space, Type: "child", Label: "Mia", Value: "2018-05-14"})
	if err != nil {
		t.Fatal(err)
	}
	gp, err := alice.SaveInfo(app.InfoInput{Space: space, Type: "info", Child: mia, Section: "medical", Label: "GP", Value: "Dr Patel, High Street Surgery"})
	if err != nil {
		t.Fatal(err)
	}
	var v *app.InfoView
	waitFor(t, "bob sees mia's GP", func() bool { v = info(bob); return len(v.Children) == 1 && len(v.Entries) == 1 })
	if v.Entries[0].Child != v.Children[0].Chain || v.Entries[0].ID != gp {
		t.Fatalf("entry: %+v", v.Entries[0])
	}
	// Bob updates Alice's entry (a reason is required); the history shows both people.
	if _, err := bob.SaveInfo(app.InfoInput{Space: space, Replaces: gp, Value: "Dr Jones"}); err == nil {
		t.Fatal("an update without a reason")
	}
	gp2, err := bob.SaveInfo(app.InfoInput{Space: space, Replaces: gp, Why: "Surgery changed the GP", Type: "info", Child: mia, Section: "medical",
		Label: "GP", Value: "Dr Jones, High Street Surgery"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "alice sees the update", func() bool { v = info(alice); return len(v.Entries) == 1 && v.Entries[0].ID == gp2 })
	e := v.Entries[0]
	if len(e.History) != 2 || e.History[0].Name != "Alice" || e.History[1].Why != "Surgery changed the GP" || e.Chain != gp || e.Conflict {
		t.Fatalf("history: %+v", e)
	}
	// Removing keeps it in the record.
	if _, err := alice.SaveInfo(app.InfoInput{Space: space, Replaces: gp2, Why: "Registered elsewhere now", Remove: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "removed", func() bool { v = info(bob); return len(v.Entries) == 0 && len(v.Removed) == 1 })
	if len(v.Removed[0].History) != 3 || !v.Removed[0].History[2].Removed {
		t.Fatalf("removed: %+v", v.Removed[0])
	}
}

func TestJournalIsPrivate(t *testing.T) {
	alice, bob, space, _, _ := family(t)
	photo := samplePNG(t, t.TempDir())
	id, err := alice.SaveJournal(app.JournalInput{Family: space, Date: "2026-11-07", Mood: "hard", Tags: []string{"handover"},
		Text: "Handover was 40 minutes late again. Mia was upset.", Files: []app.FileInput{{Path: photo}}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := alice.Journal(space)
	if err != nil || len(entries) != 1 || entries[0].ID != id || len(entries[0].Attachments) != 1 {
		t.Fatalf("journal: %v %+v", err, entries)
	}
	alice.SyncAll()
	if s, _ := alice.Spaces(); len(s) != 1 || s[0].ID != space {
		t.Fatalf("the journal shows as a family: %+v", s)
	}
	// Nothing about it reaches the family: not the items, not even the audit log.
	bob.SyncAll()
	if s, _ := bob.Spaces(); len(s) != 1 {
		t.Fatalf("bob's spaces: %+v", s)
	}
	if j, _ := bob.Journal(space); len(j) != 0 {
		t.Fatal("bob sees alice's journal")
	}
	log, err := bob.Audit(space)
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range log.Rows {
		if strings.Contains(en.What, "journal") || en.Action == audit.BlobUpload { // nothing has been uploaded to the family yet
			t.Fatalf("journal leaked into the family log: %+v", en)
		}
	}
	// Changing needs a reason; sharing posts a copy (with the photo re-encrypted for the family).
	if _, err := alice.SaveJournal(app.JournalInput{Item: id, Family: space, Date: "2026-11-07", Text: "edited"}); err == nil {
		t.Fatal("an edit without a reason")
	}
	if err := alice.ShareJournal(space, id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob gets the shared copy", func() bool {
		bob.SyncAll()
		ms, _ := bob.Messages(space)
		for _, m := range ms {
			if strings.Contains(m.Text, "40 minutes late") && len(m.Attachments) == 1 {
				return true
			}
		}
		return false
	})
	if err := alice.RemoveJournal(id, "Wrote it in anger"); err != nil {
		t.Fatal(err)
	}
	if entries, _ = alice.Journal(space); !entries[0].Removed || len(entries[0].History) != 2 {
		t.Fatalf("removed entry: %+v", entries[0])
	}
}
