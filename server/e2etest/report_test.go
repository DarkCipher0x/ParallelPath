package e2etest

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"pp/engine/app"
)

// TestReport: a court-ready report is signed by its author, says so to anyone who checks it, matches the family's
// audit chain on the other parent's device, and any change to it is detected.
func TestReport(t *testing.T) {
	alice, bob, space, aliceID, _ := family(t)
	if err := alice.Send(app.SendRequest{Space: space, Text: "Pick-up is at 4pm on Friday."}); err != nil {
		t.Fatal(err)
	}
	if err := alice.Send(app.SendRequest{Space: space, Text: `=HYPERLINK("http://x.invalid","click")`}); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.SaveExpense(app.ExpenseInput{Space: space, Data: app.ExpenseData{Title: "School trip", Amount: 3500, Currency: "GBP",
		Date: time.Now().Format("2006-01-02"), Category: "school", Share: 50}}); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	files, err := alice.Report(app.ReportInput{Space: space, From: today, To: today, Zone: "Europe/London",
		Sections: []string{"messages", "calendar", "expenses", "documents", "info", "audit"}})
	if err != nil {
		t.Fatal(err)
	}
	pdf, _ := os.ReadFile(files.PDF)
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.Contains(pdf, []byte("Pick-up is at 4pm on Friday.")) || !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatal("the PDF is missing or incomplete")
	}
	if files.Signer != aliceID {
		t.Fatal("the report should be signed by its author")
	}
	// The spreadsheet copy: every row there, and nothing a spreadsheet would run as a formula.
	csvText, _ := os.ReadFile(files.CSV)
	for _, want := range []string{"Section,Date,Time", "Pick-up is at 4pm on Friday.", "School trip", "35.00,GBP", `'=HYPERLINK(`} {
		if !bytes.Contains(csvText, []byte(want)) {
			t.Fatalf("the CSV is missing %q:\n%s", want, csvText)
		}
	}
	if bytes.Contains(csvText, []byte(`,=HYPERLINK`)) || bytes.Contains(csvText, []byte(`"=HYPERLINK`)) {
		t.Fatal("the CSV has a formula cell")
	}

	// Bob checks it on his own device: valid, signed by Alice, and the quoted chain head matches his copy.
	bob.SyncAll()
	waitFor(t, "bob has the report's audit entry", func() bool { bob.SyncAll(); c, _ := bob.VerifyReport(files.JSON); return c.ChainMatch != "" })
	c, err := bob.VerifyReport(files.JSON)
	if err != nil || !c.Valid || c.Signer != aliceID || c.ByName != "Alice" || !strings.HasPrefix(c.ChainMatch, "matches") {
		t.Fatalf("verify: %v %+v", err, c)
	}

	// One changed character anywhere breaks it.
	raw, _ := os.ReadFile(files.JSON)
	bad := bytes.Replace(raw, []byte("4pm"), []byte("5pm"), 1)
	if bytes.Equal(bad, raw) {
		t.Fatal("test setup: text not found in the JSON")
	}
	if c := app.VerifyReportJSON(bad); c.Valid {
		t.Fatal("a changed report verified")
	}
	if c := app.VerifyReportJSON(raw); !c.Valid {
		t.Fatalf("offline verify: %+v", c)
	}

	// Making the report is in the family's audit log.
	waitFor(t, "the report is in the audit log", func() bool {
		bob.SyncAll()
		log, _ := bob.Audit(space)
		for _, r := range log.Rows {
			if r.Action == "report.export" {
				return true
			}
		}
		return false
	})
}
