package e2etest

import (
	"testing"

	"pp/engine/app"
)

// TestLinkDevice: Alice links a second device with a one-time code. It gets her identity and every family key -
// her private journal included - and Bob sees it act as Alice.
func TestLinkDevice(t *testing.T) {
	ts := startServer(t)
	inv, _, _ := ts.svc.CreateServerInvite()
	alice, ares := setup(t, ts, inv, "Alice", "windows")
	space := ares.SpaceID
	code := call[map[string]any](t, alice, "invite", `{"space":"`+space+`","role":"parent"}`)["code"].(string)
	bob, bres := setup(t, ts, code, "Bob", "android")
	waitFor(t, "approval possible", func() bool {
		alice.SyncAll()
		s, _ := alice.Spaces()
		return len(s) == 1 && len(s[0].Members) == 2
	})
	call[map[string]bool](t, alice, "approve", `{"space":"`+space+`","person":"`+bres.Person+`"}`)
	if _, err := alice.SaveJournal(app.JournalInput{Family: space, Date: "2026-11-01", Text: "A private note"}); err != nil {
		t.Fatal(err)
	}
	if err := alice.Send(app.SendRequest{Space: space, Text: "From Alice's laptop"}); err != nil {
		t.Fatal(err)
	}

	link, err := alice.LinkStart()
	if err != nil {
		t.Fatal(err)
	}
	phone := app.New(t.TempDir(), nil)
	t.Cleanup(phone.Lock)
	p, err := phone.Preview(ts.url)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	var linked *app.SetupResult
	go func() {
		var err error
		linked, err = phone.SetupLinked(app.SetupLinkedRequest{Server: ts.url, CertSHA256: p.CertSHA256, Code: link["code"],
			Passphrase: "another good passphrase", DeviceName: "Alice's phone", DeviceKind: "android"})
		done <- err
	}()
	waitFor(t, "the phone asks to be linked", func() bool {
		select {
		case err := <-done:
			t.Fatalf("the phone gave up: %v", err)
		default:
		}
		r, err := alice.LinkCheck(link["code"])
		return err == nil && r["waiting"] == false && r["name"] == "Alice's phone"
	})
	if _, err := bob.LinkConfirm(link["code"]); err == nil {
		t.Fatal("someone else's device confirmed Alice's link")
	}
	if _, err := alice.LinkConfirm(link["code"]); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if linked.Person != ares.Person {
		t.Fatal("the phone isn't Alice")
	}

	// The phone reads the family's history and Alice's journal, and Bob sees it as Alice.
	waitFor(t, "the phone has the family", func() bool {
		phone.SyncAll()
		ms, _ := phone.Messages(space)
		for _, m := range ms {
			if m.Text == "From Alice's laptop" {
				return true
			}
		}
		return false
	})
	waitFor(t, "the phone has the journal", func() bool {
		phone.SyncAll()
		j, _ := phone.Journal(space)
		return len(j) == 1 && j[0].Text == "A private note"
	})
	if s, _ := phone.Spaces(); len(s) != 1 {
		t.Fatalf("the phone lists %d families", len(s))
	}
	if err := phone.Send(app.SendRequest{Space: space, Text: "From Alice's phone"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob sees it from Alice", func() bool {
		bob.SyncAll()
		ms, _ := bob.Messages(space)
		for _, m := range ms {
			if m.Text == "From Alice's phone" && m.Author == ares.Person {
				return true
			}
		}
		return false
	})
	if _, err := alice.LinkCheck(link["code"]); err == nil {
		r, _ := alice.LinkCheck(link["code"])
		if r["waiting"] != true {
			t.Fatal("a used link code is still open")
		}
	}
}
