package app

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"time"
)

// reportCSV is the report as one spreadsheet (for solicitors and accountants who want to sort and total it): one
// row per message, event, check-in, expense, document and journal entry. It's a convenience copy - the signed JSON
// stays the evidence.
func reportCSV(d *ReportData, loc *time.Location) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xef, 0xbb, 0xbf}) // a UTF-8 byte order mark, so spreadsheet apps read it as UTF-8
	w := csv.NewWriter(&buf)
	w.UseCRLF = true
	row := func(cells ...string) {
		for i, c := range cells {
			cells[i] = safeCell(c)
		}
		_ = w.Write(cells)
	}
	day := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.In(loc).Format("2006-01-02")
	}
	clock := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.In(loc).Format("15:04:05")
	}
	row("Section", "Date", "Time", "Who", "What", "Details", "Amount", "Currency", "Status", "Reason", "Item")
	for _, m := range d.Messages {
		what, status := m.Text, ""
		switch {
		case m.Call != nil:
			what = "Call"
		case m.Retracted:
			status = "withdrawn"
		case m.Edited:
			status = "edited"
		}
		var files []string
		for _, a := range m.Attachments {
			files = append(files, a.Name)
		}
		var read []string
		for _, r := range m.ReadBy {
			read = append(read, "read by "+r.Name+" "+r.Time.In(loc).Format("2006-01-02 15:04"))
		}
		row("Message", day(m.Sent), clock(m.Sent), m.AuthorName, what, strings.Join(append(files, read...), "; "), "", "", status, m.Reason, m.ID)
	}
	for _, c := range d.Conversations {
		for _, m := range c.Messages {
			what := m.Text
			if m.Call != nil {
				what = "Call"
			}
			row("Private message (with "+c.Title+")", day(m.Sent), clock(m.Sent), m.AuthorName, what, "", "", "", "", m.Reason, m.ID)
		}
	}
	if c := d.Calendar; c != nil {
		for _, ev := range c.Events {
			status := ""
			if ev.Cancelled {
				status = "cancelled"
			} else if ev.Edited {
				status = "edited"
			}
			details := ev.Data.Category
			if ev.Notes != "" {
				details = strings.TrimSpace(details + "; " + ev.Notes)
			}
			row("Event", day(ev.Start), clock(ev.Start), ev.AuthorName, ev.Data.Title, details, "", "", status, ev.Reason, ev.ID)
		}
		for _, ci := range c.CheckIns {
			row("Check-in", day(ci.Data.At), clock(ci.Data.At), ci.Name, ci.Data.Place, ci.Note, "", "", "", "", ci.ID)
		}
		for _, p := range append(append([]Proposal{}, c.Schedules...), c.Changes...) {
			row("Calendar request", day(p.Created), clock(p.Created), p.AuthorName, p.Kind, p.Note, "", "", p.State, p.Reason, p.ID)
		}
	}
	if x := d.Expenses; x != nil {
		for _, ex := range x.Expenses {
			row("Expense", ex.Data.Date, "", ex.AuthorName, ex.Data.Title, strings.TrimSpace(ex.Data.Category+"; "+ex.Note),
				minorUnits(ex.Data.Amount), ex.Data.Currency, ex.State, ex.Reason, ex.ID)
			for _, a := range ex.Answers {
				row("Expense answer", day(a.Time), clock(a.Time), a.Name, a.Answer, strings.TrimSpace(a.Method+" "+a.Reference+" "+a.For), "", "", "",
					a.Comment, ex.ID)
			}
		}
		for _, b := range x.Balances {
			row("Balance", "", "", b.FromName, "owes "+b.ToName, "", minorUnits(b.Amount), b.Currency, "", "", "")
		}
	}
	for _, doc := range d.Documents {
		status := ""
		if doc.Removed {
			status = "removed"
		}
		row("Document", day(doc.Updated), clock(doc.Updated), doc.AuthorName, doc.Path, fmt.Sprintf("%d version(s)", len(doc.Versions)), "", "",
			status, doc.Reason, doc.ID)
	}
	for _, j := range d.Journal {
		row("Journal", j.Data.Date, "", d.By.Name, j.Text, j.Data.Mood, "", "", "", j.Reason, j.ID)
	}
	w.Flush()
	return buf.Bytes()
}

// safeCell stops a cell being run as a formula when the file is opened in a spreadsheet (CSV injection): text
// that starts like a formula gets a leading apostrophe, which spreadsheets show as plain text.
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func minorUnits(n int64) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}
