package e2etest

import (
	"strings"
	"testing"
	"time"

	"pp/engine/app"
)

func approveJoiner(t *testing.T, parent, joiner *app.Engine, space, person string) {
	t.Helper()
	call[map[string]bool](t, parent, "approve", `{"space":"`+space+`","person":"`+person+`"}`)
	waitFor(t, "the new member has keys", func() bool {
		joiner.SyncAll()
		s, _ := joiner.Spaces()
		return len(s) == 1 && !s[0].Waiting
	})
}

// TestPermissionsAndConversations: what someone may see, add, message and call follows the permissions the parent who
// invited them chose; private conversations and calls in them reach only the people in them.
func TestPermissionsAndConversations(t *testing.T) {
	var ai, bi, ci inbox
	alice, bob, space, aliceID, bobID := familyWith(t, ai.listener(), bi.listener())

	// Alice invites Sam as a child with the defaults: sees messages and the calendar; messages and calls the parents.
	code := call[map[string]any](t, alice, "invite", `{"space":"`+space+`","role":"child"}`)["code"].(string)
	sam, sres := setupWith(t, testServerOf(t, alice), code, "Sam", "android", ci.listener())
	waitFor(t, "alice sees sam waiting", func() bool {
		alice.SyncAll()
		s, _ := alice.Spaces()
		for _, m := range s[0].Members {
			if m.Person == sres.Person && m.NeedsKeys {
				return true
			}
		}
		return false
	})
	approveJoiner(t, alice, sam, space, sres.Person)

	// A section that isn't shared doesn't reach Sam's device at all.
	if _, err := alice.SaveExpense(app.ExpenseInput{Space: space, Data: app.ExpenseData{Title: "Braces", Amount: 120000, Currency: "GBP",
		Date: time.Now().Format("2006-01-02"), Share: 50}}); err != nil {
		t.Fatal(err)
	}
	if err := alice.Send(app.SendRequest{Space: space, Text: "Family news"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "sam sees the family chat", func() bool { return texts(t, sam, space)["Family news"] })
	if x, _ := sam.Expenses(space); x != nil && len(x.Expenses) > 0 {
		t.Fatal("an expense reached a child it wasn't shared with")
	}
	if err := sam.Send(app.SendRequest{Space: space, Text: "child writing in the family chat"}); err == nil {
		t.Fatal("a child wrote in the family chat without permission")
	}

	// Sam may contact the parents, and only them.
	cs, err := sam.Contacts(space)
	if err != nil || len(cs) != 2 {
		t.Fatalf("sam's contacts: %v %+v", err, cs)
	}
	conv, err := sam.StartConversation(space, []string{aliceID})
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := sam.StartConversation(space, []string{aliceID}); again != conv {
		t.Fatal("the same people got a second conversation")
	}
	if err := sam.Send(app.SendRequest{Space: conv, Text: "Hi Mum, can I stay at Leo's?"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "alice reads sam privately", func() bool {
		alice.SyncAll()
		l, _ := alice.Conversations(space)
		if len(l) != 1 || l[0].Title != "Sam" {
			return false
		}
		return texts(t, alice, conv)["Hi Mum, can I stay at Leo's?"]
	})
	bob.SyncAll()
	if l, _ := bob.Conversations(space); len(l) != 0 {
		t.Fatal("bob can see alice and sam's private conversation")
	}
	if _, err := bob.Messages(conv); err == nil {
		if ms, _ := bob.Messages(conv); len(ms) > 0 {
			t.Fatal("bob read a conversation he isn't in")
		}
	}

	// Calls: Sam calls Mum in their conversation - only Alice's devices ring; the whole family can't be rung.
	if _, err := sam.CallStart(space, true); err == nil {
		t.Fatal("a child rang the whole family")
	}
	started, err := sam.CallStart(conv, true)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "alice's phone rings", func() bool { r := ai.find("ring"); return r != nil && r["call"] == started["call"] })
	time.Sleep(300 * time.Millisecond)
	if r := bi.find("ring"); r != nil {
		t.Fatal("bob's phone rang for a call that wasn't for him")
	}

	// Only the parent who invited Sam may change Sam's permissions.
	var none = app.Perms{See: []string{"messages", "calendar"}, Message: "parents", Call: "nobody"}
	if err := bob.SetPermissions(space, sres.Person, none); err == nil {
		t.Fatal("a parent changed the permissions of someone the other parent invited")
	}
	if err := alice.SetPermissions(space, sres.Person, none); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "sam's device knows", func() bool {
		sam.SyncAll()
		_, err := sam.CallStart(conv, false)
		return err != nil && strings.Contains(err.Error(), "allow")
	})
	if err := sam.Send(app.SendRequest{Space: conv, Text: "still allowed to message"}); err != nil {
		t.Fatalf("messaging stopped although only calls were taken away: %v", err)
	}

	// A section shared later arrives in full (the device fetches again from the start).
	if err := alice.SetPermissions(space, sres.Person, app.Perms{See: []string{"messages", "calendar", "expenses"}, Message: "parents", Call: "parents"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "sam now sees the expense", func() bool {
		sam.SyncAll()
		x, _ := sam.Expenses(space)
		return x != nil && len(x.Expenses) == 1
	})

	// Someone a child may not message can't be put in a conversation with them.
	code = call[map[string]any](t, bob, "invite", `{"space":"`+space+`","role":"family"}`)["code"].(string)
	gran, gres := setup(t, testServerOf(t, alice), code, "Gran", "windows")
	waitFor(t, "bob sees gran waiting", func() bool {
		bob.SyncAll()
		s, _ := bob.Spaces()
		for _, m := range s[0].Members {
			if m.Person == gres.Person && m.NeedsKeys {
				return true
			}
		}
		return false
	})
	approveJoiner(t, bob, gran, space, gres.Person)
	gran.SyncAll()
	if _, err := gran.StartConversation(space, []string{sres.Person}); err == nil {
		t.Fatal("a conversation was made with a child who may only message the parents")
	}
	if _, err := gran.StartConversation(space, []string{bobID, aliceID}); err != nil {
		t.Fatalf("a family member couldn't message the parents: %v", err)
	}

	// Removing Sam from the family ends the conversation for Sam, and Alice's device rotates its keys.
	call[map[string]bool](t, alice, "remove", `{"space":"`+space+`","person":"`+sres.Person+`"}`)
	waitFor(t, "the conversation's keys rotate", func() bool { return countAction(t, alice, conv, "epoch.rotate") == 1 })
	if err := sam.Send(app.SendRequest{Space: conv, Text: "after removal"}); err == nil {
		t.Fatal("a removed member still wrote in the conversation")
	}
}
