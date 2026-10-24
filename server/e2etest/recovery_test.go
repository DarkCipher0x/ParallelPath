package e2etest

import (
	"testing"

	"pp/engine/app"
)

// TestRecovery: Alice loses her only device. With her recovery code a new device gets her identity back (same
// safety number, same journal), her old device is revoked, and Bob approves the new one to restore the family.
func TestRecovery(t *testing.T) {
	alice, bob, space, aliceID, _ := family(t)
	ts := serverOf(t, alice)
	if _, err := alice.SaveJournal(app.JournalInput{Family: space, Date: "2026-11-01", Text: "Kept safe"}); err != nil {
		t.Fatal(err)
	}
	if err := alice.Send(app.SendRequest{Space: space, Text: "Before the phone was lost"}); err != nil {
		t.Fatal(err)
	}
	kit, err := alice.RecoveryCreate()
	if err != nil {
		t.Fatal(err)
	}
	numberBefore, _ := bob.SafetyNumber(aliceID)

	fresh := app.New(t.TempDir(), nil)
	t.Cleanup(fresh.Lock)
	p, err := fresh.Preview(ts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.SetupRecover(app.SetupRecoverRequest{Server: ts, CertSHA256: p.CertSHA256, Code: "R-AAAA-BBBB-CCCC-DDDD-EEEE-FFFF-GGGG-HHHH-IIII-JJJJ",
		Passphrase: "new device passphrase", DeviceName: "New phone", DeviceKind: "android"}); err == nil {
		t.Fatal("a wrong recovery code worked")
	}
	fresh = app.New(t.TempDir(), nil)
	t.Cleanup(fresh.Lock)
	res, err := fresh.SetupRecover(app.SetupRecoverRequest{Server: ts, CertSHA256: p.CertSHA256, Code: kit["code"],
		Passphrase: "new device passphrase", DeviceName: "New phone", DeviceKind: "android"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Person != aliceID {
		t.Fatal("recovered as someone else")
	}
	// The journal came back with the kit.
	waitFor(t, "the journal is back", func() bool {
		fresh.SyncAll()
		j, _ := fresh.Journal(space)
		return len(j) == 1 && j[0].Text == "Kept safe"
	})
	// The old device is revoked: it can't sign in any more.
	if err := alice.Send(app.SendRequest{Space: space, Text: "from the lost phone"}); err == nil {
		t.Fatal("the lost device can still post")
	}
	// Bob sees Alice waiting, the safety number is unchanged, and approving restores her history.
	waitFor(t, "bob sees alice needs approval", func() bool {
		bob.SyncAll()
		s, _ := bob.Spaces()
		for _, m := range s[0].Members {
			if m.Person == aliceID && m.NeedsKeys {
				return true
			}
		}
		return false
	})
	if n, _ := bob.SafetyNumber(aliceID); n != numberBefore {
		t.Fatal("the safety number changed")
	}
	// The lost phone held the family's current key, so Bob's device replaced it on its own.
	waitFor(t, "bob rotated the keys away from the lost phone", func() bool { return countAction(t, bob, space, "epoch.rotate") == 1 })
	call[map[string]bool](t, bob, "approve", `{"space":"`+space+`","person":"`+aliceID+`"}`)
	waitFor(t, "the new phone reads the history", func() bool {
		fresh.SyncAll()
		ms, _ := fresh.Messages(space)
		for _, m := range ms {
			if m.Text == "Before the phone was lost" {
				return true
			}
		}
		return false
	})
}
