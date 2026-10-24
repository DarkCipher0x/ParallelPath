// ppdemo seeds a running ParallelPath server with a realistic, entirely fictional family conversation, using two
// real client engines. It's for testing and screenshots only: every name and picture is made up.
//
//	ppdemo --server https://127.0.0.1:8443 --invite S.xxxx --sam DIR --alex DIR
//	ppdemo link --dir DIR --out FILE    links another device (a phone or emulator) to that demo person: writes a
//	                                    one-time link code to FILE, then confirms the device that uses it
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"pp/engine/app"
	"pp/engine/calendar"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "link" {
		link(os.Args[2:])
		return
	}
	server := flag.String("server", "", "server URL")
	invite := flag.String("invite", "", "server invite code (S.)")
	samDir := flag.String("sam", "", "data folder for Sam's engine")
	alexDir := flag.String("alex", "", "data folder for Alex's engine")
	pass := flag.String("pass", "demo-passphrase", "passphrase for both demo vaults")
	flag.Parse()
	if *server == "" || *invite == "" || *samDir == "" || *alexDir == "" {
		flag.Usage()
		os.Exit(2)
	}

	sam := app.New(*samDir, nil)
	p, err := sam.Preview(*server)
	must(err)
	fmt.Printf("server %s, key exchange %s, cert %x\n", p.Name, p.KeyExchange, p.CertSHA256[:8])
	sres, err := sam.Setup(app.SetupRequest{Server: *server, CertSHA256: p.CertSHA256, Invite: *invite, Passphrase: *pass,
		YourName: "Sam", DeviceName: "Sam's laptop", DeviceKind: "windows", FamilyName: "Mia & Leo"})
	must(err)
	space := sres.SpaceID
	waitFor("family named", func() bool { s, _ := sam.Spaces(); return len(s) == 1 && s[0].Name == "Mia & Leo" })

	code, _, err := sam.Invite(space, "parent", time.Time{}, time.Time{}, nil)
	must(err)
	alex := app.New(*alexDir, nil)
	ares, err := alex.Setup(app.SetupRequest{Server: *server, CertSHA256: p.CertSHA256, Invite: code, Passphrase: *pass,
		YourName: "Alex", DeviceName: "Alex's phone", DeviceKind: "android"})
	must(err)
	waitFor("Alex waiting", func() bool {
		sam.SyncAll()
		s, _ := sam.Spaces()
		for _, m := range s[0].Members {
			if m.Person == ares.Person && m.NeedsKeys {
				return true
			}
		}
		return false
	})
	n1, _ := sam.SafetyNumber(ares.Person)
	n2, _ := alex.SafetyNumber(sres.Person)
	if n1 != n2 {
		log.Fatal("safety numbers differ")
	}
	must(sam.Approve(space, ares.Person))
	waitFor("Alex approved", func() bool { alex.SyncAll(); s, _ := alex.Spaces(); return len(s) == 1 && !s[0].Waiting })
	waitFor("Alex's name", func() bool { sam.SyncAll(); s, _ := sam.Spaces(); return names(s[0]) })

	say := func(e *app.Engine, text string) string {
		must(e.Send(app.SendRequest{Space: space, Text: text}))
		return last(e, space).ID
	}
	say(sam, "Hi Alex - I've set up ParallelPath for us. Everything's end-to-end encrypted 🙂")
	m2 := say(alex, "Thanks. Can we swap this weekend? I've got the school trip on Saturday.")
	say(sam, "Yes, that works. I'll have them Saturday and you take Sunday?")
	m4 := say(alex, "Perfect 👍 I'll pick them up at 10:00 on Sunday.")

	photo := filepath.Join(os.TempDir(), "pp-demo-beach.png")
	must(writePhoto(photo))
	must(sam.Send(app.SendRequest{Space: space, Text: "Mia's drawing from art club 🎨", Files: []app.FileInput{{Path: photo, Name: "art-club.png"}}}))
	art := last(sam, space).ID

	sync(alex, space)
	must(alex.MarkRead(space, art))
	must(alex.React(space, art, "❤️", false))
	must(alex.React(space, art, "😊", false))
	must(sam.React(space, m4, "👍", false))
	sync(sam, space)
	must(sam.MarkRead(space, m2))
	must(sam.MarkRead(space, m4))

	m7 := say(alex, "Also - parents' evening is on the 14th at 17:30.")
	sync(alex, space)
	must(alex.Edit(space, m7, "Also - parents' evening is on the 15th at 17:30.", "wrong date, the school changed it"))
	m8 := say(sam, "Can you send me the dentist's number?")
	sync(sam, space)
	must(sam.Retract(space, m8, "found it in the info bank"))
	say(sam, "Great, see you Sunday. Leo says hi 👋")
	sync(alex, space)
	sync(sam, space)
	calendarDemo(sam, alex, space, sres.Person, ares.Person)
	recordsDemo(sam, alex, space, photo)

	// A private conversation between the parents (only their devices hold its keys).
	conv, err := sam.StartConversation(space, []string{ares.Person})
	must(err)
	must(sam.Send(app.SendRequest{Space: conv, Text: "Can we agree the summer plan between us before the mediation on Thursday?"}))
	waitFor("Alex has the conversation", func() bool { alex.SyncAll(); l, _ := alex.Conversations(space); return len(l) == 1 })
	must(alex.Send(app.SendRequest{Space: conv, Text: "Yes - I'll send dates tonight."}))
	for _, e := range []*app.Engine{sam, alex} {
		l, err := e.Audit(space)
		must(err)
		if !l.Verified {
			log.Fatalf("audit not verified: %s", l.Problem)
		}
	}
	sam.Lock()
	alex.Lock()
	fmt.Println("demo family ready:", space)
}

// calendarDemo fills this month's calendar: an agreed alternating-weeks schedule, an agreed weekend swap, a
// request still waiting for Sam, a few events (one moved, one cancelled) and a check-in at a handover. Dates are
// relative to today so screenshots always show a busy month.
func calendarDemo(sam, alex *app.Engine, space, samID, alexID string) {
	london, err := time.LoadLocation("Europe/London")
	must(err)
	now := time.Now().In(london)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, london)
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, london)
	day := func(t time.Time, hh, mm int) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), hh, mm, 0, 0, london)
	}
	weekday := func(base time.Time, wd time.Weekday) time.Time { return base.AddDate(0, 0, (int(wd)+6)%7) }

	sched, err := sam.ProposeSchedule(app.ScheduleInput{Space: space, Template: "alternating-weeks", ParentA: samID, ParentB: alexID,
		Start: weekday(first.AddDate(0, 0, -((int(first.Weekday())+6)%7)), time.Friday).Format("2006-01-02"), Exchange: "18:00",
		Zone: "Europe/London", Note: "Alternating weeks with Friday handovers, as we agreed at mediation."})
	must(err)
	sync(alex, space)
	must(alex.Answer(space, sched, true, "Agreed - thanks for setting it up."))
	sync(sam, space)

	ask := func(e *app.Engine, from, to time.Time, parent, note string) string {
		id, err := e.RequestChange(app.SwapInput{Space: space, Note: note, Changes: []app.Change{{From: from, To: to, Parent: parent}}})
		must(err)
		sync(e, space)
		return id
	}
	// Alex asks for two of Sam's Saturdays: the first is agreed, the second still waits for Sam.
	var sats []time.Time
	cv, err := sam.Calendar(space, monday.Format("2006-01-02"), monday.AddDate(0, 0, 41).Format("2006-01-02"), "Europe/London")
	must(err)
	for _, d := range cv.Days {
		t, _ := time.ParseInLocation("2006-01-02", d.Date, london)
		if t.Weekday() == time.Saturday && d.Start == samID && d.End == samID {
			sats = append(sats, t)
		}
	}
	if len(sats) < 2 {
		log.Fatal("calendar demo: no weekends with Sam to swap")
	}
	swap := ask(alex, day(sats[0], 10, 0), day(sats[0].AddDate(0, 0, 1), 17, 0), alexID, "Grandma's birthday lunch - could I have them this weekend?")
	sync(sam, space)
	must(sam.Answer(space, swap, true, "Of course, have a lovely time."))
	ask(alex, day(sats[1], 9, 0), day(sats[1], 19, 0), alexID, "The cinema has a kids' morning - happy to swap for an evening that week?")

	event := func(e *app.Engine, title, cat string, start, end time.Time, place string, every string, until time.Time, notes string) string {
		r := calendar.Repeat{Every: every}
		if every != "" {
			r.Until = until.Format("2006-01-02")
		}
		id, err := e.SaveEvent(app.EventInput{Space: space, Notes: notes, Data: app.EventData{Title: title, Category: cat, Start: start, End: end,
			Zone: "Europe/London", Location: place, Children: []string{"Mia"}, Repeat: r, AllDay: cat == "holiday"}})
		must(err)
		return id
	}
	wed := weekday(monday, time.Wednesday)
	event(sam, "Football club", "activity", day(wed, 16, 0), day(wed, 17, 0), "Rec ground", "weekly", first.AddDate(0, 2, 0), "Bring shin pads")
	pe := event(alex, "Parents' evening", "school", day(weekday(monday, time.Thursday).AddDate(0, 0, 7), 17, 30),
		day(weekday(monday, time.Thursday).AddDate(0, 0, 7), 18, 0), "Oakfield Primary", "", time.Time{}, "")
	dentist := event(sam, "Dentist check-up", "medical", day(weekday(monday, time.Tuesday).AddDate(0, 0, 7), 9, 0),
		day(weekday(monday, time.Tuesday).AddDate(0, 0, 7), 9, 30), "High Street Dental", "", time.Time{}, "Leo too")
	event(alex, "Half-term", "holiday", first.AddDate(0, 0, 24), first.AddDate(0, 0, 29), "", "", time.Time{}, "")
	sync(alex, space)
	sync(sam, space)
	must(alex.CancelEvent(space, pe, "The school moved it online - link to follow"))
	t1, t2 := day(weekday(monday, time.Tuesday).AddDate(0, 0, 7), 10, 0), day(weekday(monday, time.Tuesday).AddDate(0, 0, 7), 10, 30)
	_, err = sam.SaveEvent(app.EventInput{Space: space, Item: dentist, Reason: "They had a cancellation later in the morning", Notes: "Leo too",
		Data: app.EventData{Title: "Dentist check-up", Category: "medical", Start: t1, End: t2, Zone: "Europe/London", Location: "High Street Dental",
			Children: []string{"Mia", "Leo"}}})
	must(err)

	// A check-in at a (made-up, public) place: the location is a well-known landmark, not anyone's home.
	lat, lon := 51.5007, -0.1246
	must(alex.CheckIn(app.CheckInInput{Space: space, Place: "School gate", Note: "Dropped both off, Leo has his PE kit", Lat: &lat, Lon: &lon, Accuracy: 15}))
	sync(alex, space)
	sync(sam, space)
	v, err := sam.Calendar(space, first.Format("2006-01-02"), first.AddDate(0, 1, -1).Format("2006-01-02"), "Europe/London")
	must(err)
	if v.NeedsMe != 1 {
		log.Fatalf("calendar demo: %d requests wait on Sam, want 1", v.NeedsMe)
	}
}

// recordsDemo adds the records content: expenses in every state, documents in folders (one with two versions,
// opened by the other parent), the info bank for both children, and Sam's private journal.
func recordsDemo(sam, alex *app.Engine, space, photo string) {
	day := func(d int) string { return time.Now().AddDate(0, 0, -d).Format("2006-01-02") }
	text := func(name, body string) string {
		p := filepath.Join(os.TempDir(), name)
		must(os.WriteFile(p, []byte(body), 0o600))
		return p
	}
	exp := func(e *app.Engine, title, cat string, amount int64, d int, note string, receipts ...app.FileInput) string {
		id, err := e.SaveExpense(app.ExpenseInput{Space: space, Note: note, Receipts: receipts,
			Data: app.ExpenseData{Title: title, Amount: amount, Currency: "GBP", Date: day(d), Category: cat, Children: []string{"Mia"}, Share: 50}})
		must(err)
		return id
	}
	// Settled: school shoes (Alex agreed, paid, Sam confirmed).
	shoes := exp(sam, "School shoes", "clothing", 4800, 20, "Mia's feet grew again", app.FileInput{Path: text("shoe-receipt.txt", "Shoe shop - school shoes - £48.00")})
	sync(alex, space)
	must(alex.AnswerExpense(space, shoes, true, ""))
	sync(alex, space)
	must(alex.PayExpense(space, shoes, "Bank transfer", "MIA SHOES", ""))
	sync(sam, space)
	must(sam.ConfirmPayment(space, shoes, alexID(sam, space)))
	// Agreed, Alex still to pay: swimming. Declined: a games console. Waiting for Sam: Alex's school trip.
	swim := exp(sam, "Swimming lessons (autumn term)", "activities", 7200, 9, "Thursdays at the leisure centre")
	console := exp(sam, "Games console", "other", 25000, 6, "Birthday present from both of us?")
	sync(alex, space)
	must(alex.AnswerExpense(space, swim, true, "Yes, she loves it"))
	must(alex.AnswerExpense(space, console, false, "Too much - let's get something smaller together"))
	exp(alex, "School trip to the science museum", "school", 3500, 2, "Deposit due Friday", app.FileInput{Path: text("trip-letter.txt", "Year 3 trip - £35 per child")})
	sync(sam, space)

	// Documents: folders, a school report with a corrected second version (opened by Alex), a medical letter.
	for _, f := range []string{"School", "Medical", "Court and legal"} {
		_, err := sam.SaveDocument(app.DocInput{Space: space, Type: "folder", Name: f})
		must(err)
	}
	report, err := sam.SaveDocument(app.DocInput{Space: space, Type: "file", Folder: "School", Note: "From Mrs Okafor",
		File: &app.FileInput{Path: text("Autumn report.txt", "Autumn report: reading is excellent.")}})
	must(err)
	sync(alex, space)
	if docs, err := alex.Documents(space); err == nil {
		for _, d := range docs {
			if d.ID == report && d.File != nil {
				_, _ = alex.OpenAttachment(space, report, d.File.BlobID)
			}
		}
	}
	_, err = sam.SaveDocument(app.DocInput{Space: space, Item: report, Folder: "School", Reason: "School sent a corrected copy",
		File: &app.FileInput{Path: text("Autumn report.txt", "Autumn report (corrected): reading and maths are excellent.")}})
	must(err)
	_, err = alex.SaveDocument(app.DocInput{Space: space, Type: "file", Folder: "Medical", File: &app.FileInput{Path: text("Dentist letter.txt", "Check-up booked.")}})
	must(err)
	_, err = alex.SaveDocument(app.DocInput{Space: space, Type: "file", Folder: "School", File: &app.FileInput{Path: photo, Name: "Art club drawing.png"}})
	must(err)

	// Info bank.
	child := func(name, born string) string {
		id, err := sam.SaveInfo(app.InfoInput{Space: space, Type: "child", Label: name, Value: born})
		must(err)
		return id
	}
	mia, leo := child("Mia", "2018-05-14"), child("Leo", "2021-09-02")
	info := func(e *app.Engine, c, section, label, value string) string {
		id, err := e.SaveInfo(app.InfoInput{Space: space, Type: "info", Child: c, Section: section, Label: label, Value: value})
		must(err)
		return id
	}
	gp := info(sam, mia, "medical", "GP", "Dr Patel, High Street Surgery")
	info(sam, mia, "medical", "Allergies", "Penicillin (rash)")
	info(alex, mia, "school", "School", "Oakfield Primary, Year 3 (Mrs Okafor)")
	info(alex, mia, "clothing", "Shoe size", "13")
	info(sam, leo, "medical", "GP", "Dr Patel, High Street Surgery")
	info(alex, leo, "routines", "Bedtime", "7pm, two stories, the blue rabbit")
	info(sam, "", "contacts", "Emergency contact", "Grandma Jo - 01234 567890")
	sync(alex, space)
	_, err = alex.SaveInfo(app.InfoInput{Space: space, Replaces: gp, Why: "The surgery moved Mia to a new GP", Type: "info", Child: mia,
		Section: "medical", Label: "GP", Value: "Dr Jones, High Street Surgery"})
	must(err)

	// Sam's private journal.
	journal := func(d int, mood, body string, tags ...string) {
		_, err := sam.SaveJournal(app.JournalInput{Family: space, Date: day(d), Mood: mood, Tags: tags, Text: body})
		must(err)
	}
	journal(12, "hard", "Handover was 40 minutes late. Mia was upset in the car but settled after tea.", "handover")
	journal(5, "good", "Parents' evening went well - both of us there. Reading is ahead of her year.", "school")
	journal(1, "okay", "Leo has a cough. Gave Calpol at 6pm, sleeping now.", "health")
	sync(sam, space)
	sync(alex, space)
}

func alexID(sam *app.Engine, space string) string {
	s, err := sam.Spaces()
	must(err)
	for _, m := range s[0].Members {
		if !m.Me {
			return m.Person
		}
	}
	log.Fatal("no other parent")
	return ""
}

func names(s app.SpaceSummary) bool {
	for _, m := range s.Members {
		if m.Name == "Member "+m.Person[:6] {
			return false
		}
	}
	return len(s.Members) == 2
}

func last(e *app.Engine, space string) app.Message {
	ms, err := e.Messages(space)
	must(err)
	return ms[len(ms)-1]
}

func sync(e *app.Engine, space string) {
	e.SyncAll()
	time.Sleep(300 * time.Millisecond)
}

func waitFor(what string, cond func() bool) {
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Fatalf("timed out: %s", what)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// writePhoto draws a cheerful child's-drawing style picture (sun, hills, house) so screenshots show a real image.
func writePhoto(path string) error {
	const w, h = 900, 640
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{uint8(150 + y/12), uint8(205 + y/30), 245, 255}
			if float64(y) > 430+40*math.Sin(float64(x)/90) {
				c = color.RGBA{uint8(90 + x/40), 180, 90, 255}
			}
			if dx, dy := float64(x-720), float64(y-130); dx*dx+dy*dy < 70*70 {
				c = color.RGBA{255, 205, 60, 255}
			}
			if x > 260 && x < 470 && y > 300 && y < 470 {
				c = color.RGBA{230, 110, 90, 255}
			}
			if y > 200 && y <= 300 && x > 260+(300-y) && x < 470-(300-y) {
				c = color.RGBA{120, 70, 60, 255}
			}
			if x > 345 && x < 395 && y > 390 && y < 470 {
				c = color.RGBA{90, 60, 40, 255}
			}
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// link makes a link code for a seeded demo person and confirms the device that asks with it (ten minutes at most).
func link(args []string) {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	dir := fs.String("dir", "", "the demo person's engine folder (--sam or --alex from the seeding run)")
	out := fs.String("out", "", "file to write the link code to")
	pass := fs.String("pass", "demo-passphrase", "the demo vault's passphrase")
	fs.Parse(args)
	if *dir == "" || *out == "" {
		fs.Usage()
		os.Exit(2)
	}
	e := app.New(*dir, nil)
	must(e.Unlock([]byte(*pass), nil))
	defer e.Lock()
	r, err := e.LinkStart()
	must(err)
	must(os.WriteFile(*out, []byte(r["code"]), 0o600))
	fmt.Println("link code written; waiting for the device")
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		c, err := e.LinkCheck(r["code"])
		must(err)
		if c["waiting"] == false {
			fmt.Printf("linking %v (%v)\n", c["name"], c["kind"])
			_, err := e.LinkConfirm(r["code"])
			must(err)
			fmt.Println("linked")
			return
		}
		time.Sleep(time.Second)
	}
	log.Fatal("no device asked to link within ten minutes")
}
