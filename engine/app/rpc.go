package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"pp/engine/help"
	"time"

	"pp/engine/calendar"
)

// Call is the single entry point the apps use: a method name and JSON arguments in, a JSON result out.
// The same table serves the Windows engine process, the Android library and the web client, so every client
// has exactly the same capabilities and the same checks.
func (e *Engine) Call(method string, args json.RawMessage) (any, error) {
	h, ok := methods[method]
	if !ok {
		return nil, fmt.Errorf("unknown method %q", method)
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	return h(e, args)
}

type handler func(*Engine, json.RawMessage) (any, error)

func bind[T any](fn func(*Engine, T) (any, error)) handler {
	return func(e *Engine, raw json.RawMessage) (any, error) {
		var a T
		dec := json.NewDecoder(bytesReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&a); err != nil {
			return nil, errors.New("invalid arguments")
		}
		return fn(e, a)
	}
}

type none struct{}
type spaceArg struct {
	Space string `json:"space"`
}
type itemArg struct {
	Space string `json:"space"`
	Item  string `json:"item"`
}

var methods = map[string]handler{
	"status": bind(func(e *Engine, _ none) (any, error) { return e.Status(), nil }),
	"preview": bind(func(e *Engine, a struct {
		Server string `json:"server"`
	}) (any, error) {
		return e.Preview(a.Server)
	}),
	"setup": bind(func(e *Engine, a SetupRequest) (any, error) { return e.Setup(a) }),
	"unlock": bind(func(e *Engine, a struct {
		Passphrase   string `json:"passphrase"`
		DeviceSecret []byte `json:"device_secret,omitempty"`
	}) (any, error) {
		return ok(e.Unlock([]byte(a.Passphrase), a.DeviceSecret))
	}),
	"lock": bind(func(e *Engine, _ none) (any, error) { e.Lock(); return ok(nil) }),
	"sync": bind(func(e *Engine, _ none) (any, error) { go e.SyncAll(); return ok(nil) }),

	"spaces":  bind(func(e *Engine, _ none) (any, error) { return e.Spaces() }),
	"account": bind(func(e *Engine, _ none) (any, error) { return e.Account() }),
	"create_space": bind(func(e *Engine, a struct {
		Name string `json:"name"`
	}) (any, error) {
		id, err := e.CreateSpace(a.Name)
		return map[string]string{"space": id}, err
	}),
	"rename_space": bind(func(e *Engine, a struct {
		Space string `json:"space"`
		Name  string `json:"name"`
	}) (any, error) {
		return ok(e.RenameSpace(a.Space, a.Name))
	}),
	"set_my_name": bind(func(e *Engine, a struct {
		Space string `json:"space"`
		Name  string `json:"name"`
	}) (any, error) {
		return ok(e.SetMyName(a.Space, a.Name))
	}),
	"invite": bind(func(e *Engine, a struct {
		Space       string    `json:"space"`
		Role        string    `json:"role"`
		NotAfter    time.Time `json:"not_after"`
		HistoryFrom time.Time `json:"history_from"`
		Perms       *Perms    `json:"perms"`
	}) (any, error) {
		code, exp, err := e.Invite(a.Space, a.Role, a.NotAfter, a.HistoryFrom, a.Perms)
		return map[string]any{"code": code, "expires": exp}, err
	}),
	"set_permissions": bind(func(e *Engine, a struct {
		Space  string `json:"space"`
		Person string `json:"person"`
		Perms  Perms  `json:"perms"`
	}) (any, error) {
		return ok(e.SetPermissions(a.Space, a.Person, a.Perms))
	}),
	"contacts":      bind(func(e *Engine, a spaceArg) (any, error) { return e.Contacts(a.Space) }),
	"conversations": bind(func(e *Engine, a spaceArg) (any, error) { return e.Conversations(a.Space) }),
	"start_conversation": bind(func(e *Engine, a struct {
		Space  string   `json:"space"`
		People []string `json:"people"`
	}) (any, error) {
		id, err := e.StartConversation(a.Space, a.People)
		return map[string]string{"space": id}, err
	}),
	"help_ask": bind(func(e *Engine, a struct {
		Question string `json:"question"`
	}) (any, error) {
		return help.Ask(a.Question, 4), nil
	}),
	"check_tone": bind(func(e *Engine, a struct {
		Text string `json:"text"`
	}) (any, error) {
		return CheckTone(a.Text), nil
	}),
	"set_time_zone": bind(func(e *Engine, a struct {
		Zone string `json:"zone"`
	}) (any, error) {
		return ok(e.SetTimeZone(a.Zone))
	}),
	"set_children_chat": bind(func(e *Engine, a struct {
		Space string `json:"space"`
		On    bool   `json:"on"`
	}) (any, error) {
		return ok(e.SetChildrenChat(a.Space, a.On))
	}),
	"join": bind(func(e *Engine, a struct {
		Invite string `json:"invite"`
	}) (any, error) {
		return e.JoinFamily(a.Invite)
	}),
	"safety_number": bind(func(e *Engine, a struct {
		Person string `json:"person"`
	}) (any, error) {
		n, err := e.SafetyNumber(a.Person)
		return map[string]string{"number": n}, err
	}),
	"approve": bind(func(e *Engine, a struct {
		Space  string `json:"space"`
		Person string `json:"person"`
	}) (any, error) {
		return ok(e.Approve(a.Space, a.Person))
	}),
	"remove": bind(func(e *Engine, a struct {
		Space  string `json:"space"`
		Person string `json:"person"`
	}) (any, error) {
		return ok(e.Remove(a.Space, a.Person))
	}),

	"messages": bind(func(e *Engine, a spaceArg) (any, error) { return e.Messages(a.Space) }),
	"send":     bind(func(e *Engine, a SendRequest) (any, error) { return ok(e.Send(a)) }),
	"react": bind(func(e *Engine, a struct {
		Space  string `json:"space"`
		Item   string `json:"item"`
		Emoji  string `json:"emoji"`
		Remove bool   `json:"remove"`
	}) (any, error) {
		return ok(e.React(a.Space, a.Item, a.Emoji, a.Remove))
	}),
	"edit": bind(func(e *Engine, a struct {
		Space  string `json:"space"`
		Item   string `json:"item"`
		Text   string `json:"text"`
		Reason string `json:"reason"`
	}) (any, error) {
		return ok(e.Edit(a.Space, a.Item, a.Text, a.Reason))
	}),
	"retract": bind(func(e *Engine, a struct {
		Space  string `json:"space"`
		Item   string `json:"item"`
		Reason string `json:"reason"`
	}) (any, error) {
		return ok(e.Retract(a.Space, a.Item, a.Reason))
	}),
	"mark_read": bind(func(e *Engine, a itemArg) (any, error) { return ok(e.MarkRead(a.Space, a.Item)) }),
	"open_attachment": bind(func(e *Engine, a struct {
		Space string `json:"space"`
		Item  string `json:"item"`
		Blob  string `json:"blob"`
	}) (any, error) {
		p, err := e.OpenAttachment(a.Space, a.Item, a.Blob)
		return map[string]string{"path": p}, err
	}),
	"audit": bind(func(e *Engine, a spaceArg) (any, error) { return e.Audit(a.Space) }),

	// Calendar: events, parenting-time schedules, change requests, answers and check-ins.
	"calendar": bind(func(e *Engine, a struct {
		Space string `json:"space"`
		From  string `json:"from"`
		To    string `json:"to"`
		Zone  string `json:"zone"`
	}) (any, error) {
		return e.Calendar(a.Space, a.From, a.To, a.Zone)
	}),
	"schedule_templates": bind(func(e *Engine, _ none) (any, error) { return calendar.Templates, nil }),
	"save_event": bind(func(e *Engine, a EventInput) (any, error) {
		id, err := e.SaveEvent(a)
		return map[string]string{"item": id}, err
	}),
	"cancel_event": bind(func(e *Engine, a reasonArg) (any, error) { return ok(e.CancelEvent(a.Space, a.Item, a.Reason)) }),
	"propose_schedule": bind(func(e *Engine, a ScheduleInput) (any, error) {
		id, err := e.ProposeSchedule(a)
		return map[string]string{"item": id}, err
	}),
	"request_change": bind(func(e *Engine, a SwapInput) (any, error) {
		id, err := e.RequestChange(a)
		return map[string]string{"item": id}, err
	}),
	"withdraw": bind(func(e *Engine, a reasonArg) (any, error) { return ok(e.Withdraw(a.Space, a.Item, a.Reason)) }),
	"answer": bind(func(e *Engine, a struct {
		Space   string `json:"space"`
		Item    string `json:"item"`
		Accept  bool   `json:"accept"`
		Comment string `json:"comment"`
	}) (any, error) {
		return ok(e.Answer(a.Space, a.Item, a.Accept, a.Comment))
	}),
	"check_in": bind(func(e *Engine, a CheckInInput) (any, error) { return ok(e.CheckIn(a)) }),

	// Expenses: what was paid and the share asked, answers, payments and who owes whom.
	"expenses": bind(func(e *Engine, a spaceArg) (any, error) { return e.Expenses(a.Space) }),
	"save_expense": bind(func(e *Engine, a ExpenseInput) (any, error) {
		id, err := e.SaveExpense(a)
		return map[string]string{"item": id}, err
	}),
	"withdraw_expense": bind(func(e *Engine, a reasonArg) (any, error) { return ok(e.WithdrawExpense(a.Space, a.Item, a.Reason)) }),
	"answer_expense": bind(func(e *Engine, a struct {
		Space   string `json:"space"`
		Item    string `json:"item"`
		Accept  bool   `json:"accept"`
		Comment string `json:"comment"`
	}) (any, error) {
		return ok(e.AnswerExpense(a.Space, a.Item, a.Accept, a.Comment))
	}),
	"pay_expense": bind(func(e *Engine, a struct {
		Space     string `json:"space"`
		Item      string `json:"item"`
		Method    string `json:"method"`
		Reference string `json:"reference"`
		Note      string `json:"note"`
	}) (any, error) {
		return ok(e.PayExpense(a.Space, a.Item, a.Method, a.Reference, a.Note))
	}),
	"confirm_payment": bind(func(e *Engine, a struct {
		Space  string `json:"space"`
		Item   string `json:"item"`
		Person string `json:"person"`
	}) (any, error) {
		return ok(e.ConfirmPayment(a.Space, a.Item, a.Person))
	}),

	// Documents: folders and versioned files; opening one uses open_attachment (and is logged).
	"documents": bind(func(e *Engine, a spaceArg) (any, error) { return e.Documents(a.Space) }),
	"save_document": bind(func(e *Engine, a DocInput) (any, error) {
		id, err := e.SaveDocument(a)
		return map[string]string{"item": id}, err
	}),
	"remove_document": bind(func(e *Engine, a reasonArg) (any, error) { return ok(e.RemoveDocument(a.Space, a.Item, a.Reason)) }),
	// The file explorer (docops.go): folders move and copy with what's in them; copies can go to another family.
	"move_document": bind(func(e *Engine, a struct {
		Space    string `json:"space"`
		Item     string `json:"item"`
		Path     string `json:"path"` // a folder without an item of its own
		ToFolder string `json:"to_folder"`
		Name     string `json:"name"`
		Reason   string `json:"reason"`
	}) (any, error) {
		return ok(e.MoveDocument(a.Space, a.Item, a.Path, a.ToFolder, a.Name, a.Reason))
	}),
	"remove_documents": bind(func(e *Engine, a struct {
		Space   string   `json:"space"`
		Items   []string `json:"items"`
		Folders []string `json:"folders"`
		Reason  string   `json:"reason"`
	}) (any, error) {
		return ok(e.RemoveDocuments(a.Space, a.Items, a.Folders, a.Reason))
	}),
	"restore_document": bind(func(e *Engine, a reasonArg) (any, error) { return ok(e.RestoreDocument(a.Space, a.Item, a.Reason)) }),
	"copy_document": bind(func(e *Engine, a struct {
		Space    string `json:"space"`
		Item     string `json:"item"`
		Path     string `json:"path"`
		ToSpace  string `json:"to_space"`
		ToFolder string `json:"to_folder"`
	}) (any, error) {
		return ok(e.CopyDocument(a.Space, a.Item, a.Path, a.ToSpace, a.ToFolder))
	}),
	"shared_files": bind(func(e *Engine, a spaceArg) (any, error) { return e.SharedFiles(a.Space) }),
	"save_shared_file": bind(func(e *Engine, a struct {
		Space    string `json:"space"` // the family
		From     string `json:"from"`  // the family or a conversation in it
		Message  string `json:"message"`
		BlobID   string `json:"blob_id"`
		ToFolder string `json:"to_folder"`
	}) (any, error) {
		id, err := e.SaveSharedFile(a.Space, a.From, a.Message, a.BlobID, a.ToFolder)
		return map[string]string{"item": id}, err
	}),

	// Info bank: children and the facts about them; either parent updates, history kept.
	"info_bank": bind(func(e *Engine, a spaceArg) (any, error) { return e.InfoBank(a.Space) }),
	"save_info": bind(func(e *Engine, a InfoInput) (any, error) {
		id, err := e.SaveInfo(a)
		return map[string]string{"item": id}, err
	}),

	// Journal: private to this person (a space of their own), optionally shared into a family's messages.
	"journal": bind(func(e *Engine, a struct {
		Family string `json:"family"`
	}) (any, error) {
		return e.Journal(a.Family)
	}),
	"save_journal": bind(func(e *Engine, a JournalInput) (any, error) {
		id, err := e.SaveJournal(a)
		return map[string]string{"item": id}, err
	}),
	"remove_journal": bind(func(e *Engine, a struct {
		Item   string `json:"item"`
		Reason string `json:"reason"`
	}) (any, error) {
		return ok(e.RemoveJournal(a.Item, a.Reason))
	}),
	"open_journal_file": bind(func(e *Engine, a struct {
		Item string `json:"item"`
		Blob string `json:"blob"`
	}) (any, error) {
		p, err := e.OpenJournalFile(a.Item, a.Blob)
		return map[string]string{"path": p}, err
	}),
	"share_journal": bind(func(e *Engine, a struct {
		Family string `json:"family"`
		Item   string `json:"item"`
	}) (any, error) {
		return ok(e.ShareJournal(a.Family, a.Item))
	}),

	// Calls: per-call media keys and call-setup messages, end-to-end encrypted under the family's keys.
	"call_start": bind(func(e *Engine, a struct {
		Space string `json:"space"`
		Video bool   `json:"video"`
	}) (any, error) {
		return e.CallStart(a.Space, a.Video)
	}),
	"call_signal":  bind(func(e *Engine, a CallSignalInput) (any, error) { return ok(e.CallSignal(a)) }),
	"call_session": bind(func(e *Engine, a itemCall) (any, error) { return e.CallSession(a.Space, a.Call) }),
	"call_end": bind(func(e *Engine, a struct {
		Space   string `json:"space"`
		Call    string `json:"call"`
		Seconds int    `json:"seconds"`
		Missed  bool   `json:"missed"`
	}) (any, error) {
		return ok(e.CallEnd(a.Space, a.Call, a.Seconds, a.Missed))
	}),
	"call_decline": bind(func(e *Engine, a itemCall) (any, error) { return ok(e.CallDecline(a.Space, a.Call)) }),
	"call_turn":    bind(func(e *Engine, _ none) (any, error) { return e.CallTurn() }),
	"call_bridge": bind(func(e *Engine, _ none) (any, error) {
		u, err := e.CallBridge()
		return map[string]string{"url": u}, err
	}),
}

type reasonArg struct {
	Space  string `json:"space"`
	Item   string `json:"item"`
	Reason string `json:"reason"`
}

func ok(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type itemCall struct {
	Space string `json:"space"`
	Call  string `json:"call"`
}

func init() {
	// Reports: court-ready, signed, made on this device.
	methods["report"] = bind(func(e *Engine, a ReportInput) (any, error) { return e.Report(a) })
	methods["verify_report"] = bind(func(e *Engine, a struct {
		Path string `json:"path"`
	}) (any, error) {
		return e.VerifyReport(a.Path)
	})
}

func init() {
	// Linking another device of the same person.
	methods["link_start"] = bind(func(e *Engine, _ none) (any, error) { return e.LinkStart() })
	methods["link_check"] = bind(func(e *Engine, a struct {
		Code string `json:"code"`
	}) (any, error) {
		return e.LinkCheck(a.Code)
	})
	methods["link_confirm"] = bind(func(e *Engine, a struct {
		Code string `json:"code"`
	}) (any, error) {
		return e.LinkConfirm(a.Code)
	})
	methods["setup_linked"] = bind(func(e *Engine, a SetupLinkedRequest) (any, error) { return e.SetupLinked(a) })
}

func init() {
	// Recovery kits.
	methods["recovery_create"] = bind(func(e *Engine, _ none) (any, error) { return e.RecoveryCreate() })
	methods["recovery_status"] = bind(func(e *Engine, _ none) (any, error) { return e.RecoveryStatus() })
	methods["setup_recover"] = bind(func(e *Engine, a SetupRecoverRequest) (any, error) { return e.SetupRecover(a) })
}
