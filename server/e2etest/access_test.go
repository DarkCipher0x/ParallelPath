package e2etest

import (
	"testing"
	"time"

	"pp/engine/app"
)

func joinAs(t *testing.T, parent *app.Engine, space, args, name string) (*app.Engine, string) {
	t.Helper()
	code := call[map[string]any](t, parent, "invite", args)["code"].(string)
	e, res := setup(t, &testServer{url: serverOf(t, parent)}, code, name, "windows")
	waitFor(t, name+" is waiting for approval", func() bool {
		parent.SyncAll()
		s, _ := parent.Spaces()
		for _, sp := range s {
			for _, m := range sp.Members {
				if sp.ID == space && m.Person == res.Person && m.NeedsKeys {
					return true
				}
			}
		}
		return false
	})
	return e, res.Person
}

func texts(t *testing.T, e *app.Engine, space string) map[string]bool {
	t.Helper()
	e.SyncAll()
	ms, err := e.Messages(space)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, m := range ms {
		out[m.Text] = true
	}
	return out
}

func countAction(t *testing.T, e *app.Engine, space, action string) int {
	t.Helper()
	e.SyncAll()
	log, err := e.Audit(space)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range log.Rows {
		if r.Action == action {
			n++
		}
	}
	return n
}

// TestProfessionalHistoryWindow: a mediator invited for "from today" sees only what was written from then on. The
// server hides the older items, and they never get the keys of epochs that only hold older history.
func TestProfessionalHistoryWindow(t *testing.T) {
	alice, _, space, _, bobID := family(t)
	waitFor(t, "bob's name is in", func() bool {
		alice.SyncAll()
		s, _ := alice.Spaces()
		for _, m := range s[0].Members {
			if m.Person == bobID && m.Name == "Bob" {
				return true
			}
		}
		return false
	})
	if err := alice.Send(app.SendRequest{Space: space, Text: "Before the mediation"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	from := time.Now()
	time.Sleep(20 * time.Millisecond)
	if err := alice.Rotate(space); err != nil { // epoch 2: everything in epoch 1 is from before the window
		t.Fatal(err)
	}
	if err := alice.Send(app.SendRequest{Space: space, Text: "During the mediation"}); err != nil {
		t.Fatal(err)
	}
	until := time.Now().AddDate(0, 2, 0)
	mediator, mid := joinAs(t, alice, space, `{"space":"`+space+`","role":"professional","not_after":"`+until.Format(time.RFC3339)+
		`","history_from":"`+from.Format(time.RFC3339Nano)+`"}`, "Mediator")
	grantsBefore := countAction(t, alice, space, "keys.grant")
	call[map[string]bool](t, alice, "approve", `{"space":"`+space+`","person":"`+mid+`"}`)
	waitFor(t, "the mediator reads the window", func() bool { return texts(t, mediator, space)["During the mediation"] })
	waitFor(t, "alice's audit log shows the grant", func() bool { return countAction(t, alice, space, "keys.grant") > grantsBefore })
	if n := countAction(t, alice, space, "keys.grant") - grantsBefore; n != 1 {
		t.Fatalf("the mediator got %d epoch keys, want only the one their window needs", n)
	}
	if texts(t, mediator, space)["Before the mediation"] {
		t.Fatal("the mediator can see history from before their window")
	}
	// They still see who's who: the parents' devices post their names again inside the window.
	waitFor(t, "the mediator sees the parents' names", func() bool {
		alice.SyncAll()
		mediator.SyncAll()
		s, _ := mediator.Spaces()
		if len(s) != 1 {
			return false
		}
		named := 0
		for _, m := range s[0].Members {
			if m.Name == "Alice" || m.Name == "Bob" {
				named++
			}
		}
		return named == 2
	})
	if err := mediator.Send(app.SendRequest{Space: space, Text: "a professional writing"}); err == nil {
		t.Fatal("a professional could post")
	}
	if !texts(t, alice, space)["Before the mediation"] {
		t.Fatal("the parents lost their own history")
	}
	// A window that doesn't start before the access ends is refused.
	if _, err := alice.Call("invite", []byte(`{"space":"`+space+`","role":"professional","not_after":"`+until.Format(time.RFC3339)+
		`","history_from":"`+until.AddDate(0, 1, 0).Format(time.RFC3339)+`"}`)); err == nil {
		t.Fatal("a history window after the end date was accepted")
	}
	// Only professionals take a history window.
	if _, err := alice.Call("invite", []byte(`{"space":"`+space+`","role":"family","history_from":"`+from.Format(time.RFC3339)+`"}`)); err == nil {
		t.Fatal("a history window for a family member was accepted")
	}
}

// TestChildrenChat: children are read-only until a parent lets them chat, and only chat (no other kinds).
func TestChildrenChat(t *testing.T) {
	alice, bob, space, _, _ := family(t)
	child, cid := joinAs(t, alice, space, `{"space":"`+space+`","role":"child"}`, "Sam")
	call[map[string]bool](t, alice, "approve", `{"space":"`+space+`","person":"`+cid+`"}`)
	waitFor(t, "the child has keys", func() bool {
		child.SyncAll()
		s, _ := child.Spaces()
		return len(s) == 1 && !s[0].Waiting
	})
	if err := child.Send(app.SendRequest{Space: space, Text: "hi"}); err == nil {
		t.Fatal("a child could chat before a parent allowed it")
	}
	if _, err := bob.Call("set_children_chat", []byte(`{"space":"`+space+`","on":true}`)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the child's app knows it may chat", func() bool {
		child.SyncAll()
		s, _ := child.Spaces()
		return s[0].CanChat
	})
	if err := child.Send(app.SendRequest{Space: space, Text: "Hi from Sam"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the parents see the child's message", func() bool { return texts(t, alice, space)["Hi from Sam"] })
	if _, err := child.SaveInfo(app.InfoInput{Space: space, Type: "child", Label: "Name", Value: "x"}); err == nil {
		t.Fatal("a child could add to the info bank")
	}
	if _, err := child.Call("set_children_chat", []byte(`{"space":"`+space+`","on":false}`)); err == nil {
		t.Fatal("a child changed the family's settings")
	}
	if _, err := alice.Call("set_children_chat", []byte(`{"space":"`+space+`","on":false}`)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the child's app knows chat is off", func() bool {
		child.SyncAll()
		s, _ := child.Spaces()
		return !s[0].CanChat
	})
	if err := child.Send(app.SendRequest{Space: space, Text: "again"}); err == nil {
		t.Fatal("a child could still chat after it was switched off")
	}
}
