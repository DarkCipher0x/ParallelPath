package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"pp/core/envelope"
)

// Expenses are ordinary family items (kind "expense"), end-to-end encrypted, signed and never deleted:
//
//   - an expense (no Ref): what was paid, by whom (its author), how much, and the share asked of the other parents,
//     with receipts as encrypted attachments;
//   - an answer (Ref = the expense): accept or decline (with a reason) exactly one revision; "paid" when a parent
//     pays their share (with how, and a reference); "received" when the payer confirms it arrived.
//
// Editing an expense makes a new revision, so earlier answers no longer count. Once anyone has paid it, it can't be
// edited; it can still be withdrawn (the payments stay in the record).

// ExpenseData is an expense's content.
type ExpenseData struct {
	Title    string   `json:"title"`
	Amount   int64    `json:"amount"`   // minor units (pence, cents)
	Currency string   `json:"currency"` // ISO 4217, e.g. GBP
	Date     string   `json:"date"`     // yyyy-mm-dd, when it was paid
	Category string   `json:"category,omitempty"`
	Children []string `json:"children,omitempty"`
	Share    int      `json:"share"` // percent of Amount asked of each other parent (0-100)
}

// ExpenseAnswerData is a parent's answer to one exact revision of an expense.
type ExpenseAnswerData struct {
	Answer    string `json:"answer"` // accept | decline | paid | received
	Rev       uint64 `json:"rev"`
	Hash      string `json:"hash"`
	Method    string `json:"method,omitempty"`    // paid: bank transfer, cash...
	Reference string `json:"reference,omitempty"` // paid: a payment reference
	Person    string `json:"person,omitempty"`    // received: whose payment arrived
}

var expenseCategories = map[string]bool{"": true, "medical": true, "school": true, "clothing": true, "activities": true,
	"childcare": true, "travel": true, "food": true, "other": true}

func (d *ExpenseData) check() error {
	d.Title = strings.TrimSpace(d.Title)
	d.Currency = strings.ToUpper(strings.TrimSpace(d.Currency))
	switch {
	case d.Title == "" || utf8.RuneCountInString(d.Title) > 120:
		return errors.New("say what it was for (up to 120 characters)")
	case d.Amount <= 0 || d.Amount > 100_000_000_00:
		return errors.New("enter the amount paid")
	case len(d.Currency) != 3 || strings.Trim(d.Currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "":
		return errors.New("choose a currency")
	case d.Share < 0 || d.Share > 100:
		return errors.New("the share must be between 0 and 100%")
	case !expenseCategories[d.Category]:
		return errors.New("unknown category")
	case len(d.Children) > 20:
		return errors.New("check the expense's details")
	}
	if _, err := time.Parse("2006-01-02", d.Date); err != nil {
		return errors.New("choose the date it was paid")
	}
	return nil
}

// ExpenseAnswer is an answer or payment, as shown.
type ExpenseAnswer struct {
	Person    string    `json:"person"`
	Name      string    `json:"name"`
	Answer    string    `json:"answer"`
	Comment   string    `json:"comment,omitempty"`
	Method    string    `json:"method,omitempty"`
	Reference string    `json:"reference,omitempty"`
	For       string    `json:"for,omitempty"` // received: whose payment (name)
	Time      time.Time `json:"time"`
	Current   bool      `json:"current"` // about the current revision (older ones are kept for the record)
}

// Expense is an expense and where it stands.
type Expense struct {
	ID         string           `json:"id"`
	Rev        uint64           `json:"rev"`
	Author     string           `json:"author"` // who paid it
	AuthorName string           `json:"author_name"`
	Mine       bool             `json:"mine"`
	Created    time.Time        `json:"created"`
	Updated    time.Time        `json:"updated"`
	Data       ExpenseData      `json:"data"`
	Note       string           `json:"note,omitempty"`
	Owed       int64            `json:"owed"`  // asked of each other parent
	State      string           `json:"state"` // pending | agreed | declined | withdrawn | paid | settled
	Reason     string           `json:"reason,omitempty"`
	Waiting    []string         `json:"waiting,omitempty"` // names: parents yet to answer, or (agreed) to pay
	NeedsMe    bool             `json:"needs_me"`          // waiting for my answer, or for me to confirm a payment
	CanPay     bool             `json:"can_pay"`           // agreed, and my share isn't paid yet
	ToConfirm  []string         `json:"to_confirm,omitempty"`
	Answers    []ExpenseAnswer  `json:"answers"`
	History    []Revision       `json:"history"`
	Receipts   []AttachmentView `json:"receipts,omitempty"`
	Problem    string           `json:"problem,omitempty"`
}

// Owing is what one parent owes another, netted over agreed, unpaid expenses (per currency).
type Owing struct {
	From     string `json:"from"`
	FromName string `json:"from_name"`
	To       string `json:"to"`
	ToName   string `json:"to_name"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// ExpensesView is everything the Expenses screen shows.
type ExpensesView struct {
	Parents  []Person  `json:"parents"`
	Expenses []Expense `json:"expenses"` // newest first
	Balances []Owing   `json:"balances"`
	NeedsMe  int       `json:"needs_me"`
}

// ExpenseInput is a new or edited expense. Receipts are files to encrypt and attach (kept on edits).
type ExpenseInput struct {
	Space    string      `json:"space"`
	Item     string      `json:"item,omitempty"`
	Reason   string      `json:"reason,omitempty"`
	Data     ExpenseData `json:"data"`
	Note     string      `json:"note,omitempty"`
	Receipts []FileInput `json:"receipts,omitempty"`
}

// SaveExpense records an expense you paid, or (with Item and a reason) changes one of yours that nobody has paid.
func (e *Engine) SaveExpense(in ExpenseInput) (string, error) {
	if _, err := e.requireParent(in.Space); err != nil {
		return "", err
	}
	if err := in.Data.check(); err != nil {
		return "", err
	}
	if utf8.RuneCountInString(in.Note) > 4000 {
		return "", errors.New("the note is too long")
	}
	var keep []envelope.Attachment
	if in.Item != "" {
		v, err := e.Expenses(in.Space)
		if err != nil {
			return "", err
		}
		for _, x := range v.Expenses {
			if x.ID == in.Item && (x.State == "paid" || x.State == "settled" || len(x.paidBy()) > 0) {
				return "", errors.New("it's been paid, so it can't be changed - withdraw it and add a new one if it was wrong")
			}
		}
		cur, err := e.latest(in.Space, in.Item)
		if err != nil {
			return "", err
		}
		if cur.body != nil {
			keep = cur.body.Attachments
		}
	}
	atts := keep
	for _, f := range in.Receipts {
		a, err := e.uploadFile(in.Space, f)
		if err != nil {
			return "", err
		}
		atts = append(atts, *a)
	}
	body := envelope.Body{Text: strings.TrimSpace(in.Note), Data: jsonData(in.Data), Attachments: atts}
	if in.Item == "" {
		it, err := e.post(in.Space, envelope.Draft{Kind: envelope.KindExpense, Body: body})
		if err != nil {
			return "", err
		}
		return it.ItemID, nil
	}
	body.Reason = strings.TrimSpace(in.Reason)
	return in.Item, e.reviseKind(in.Space, in.Item, envelope.KindExpense, body)
}

func (x *Expense) paidBy() []string {
	var out []string
	for _, a := range x.Answers {
		if a.Current && a.Answer == "paid" {
			out = append(out, a.Person)
		}
	}
	return out
}

// WithdrawExpense withdraws one of your expenses (kept in the record with the reason).
func (e *Engine) WithdrawExpense(space, item, reason string) error {
	return e.reviseKind(space, item, envelope.KindExpense, envelope.Body{Retracted: true, Reason: strings.TrimSpace(reason)})
}

// AnswerExpense accepts or declines exactly the current version of another parent's expense.
func (e *Engine) AnswerExpense(space, item string, accept bool, comment string) error {
	comment = strings.TrimSpace(comment)
	if !accept && comment == "" {
		return errors.New("say why you're declining - it's kept with the expense")
	}
	x, it, err := e.expenseFor(space, item)
	if err != nil {
		return err
	}
	if x.Mine {
		return errors.New("you can't answer your own expense")
	}
	if x.State == "withdrawn" {
		return errors.New("it was withdrawn")
	}
	if len(x.paidBy()) > 0 {
		return errors.New("it's already been paid")
	}
	a := "decline"
	if accept {
		a = "accept"
	}
	return e.postExpenseAnswer(space, it, ExpenseAnswerData{Answer: a}, comment)
}

// PayExpense records that you paid your share of an agreed expense (how, and an optional reference).
func (e *Engine) PayExpense(space, item, method, reference, note string) error {
	x, it, err := e.expenseFor(space, item)
	if err != nil {
		return err
	}
	if !x.CanPay {
		return errors.New("there's nothing for you to pay on this expense")
	}
	method, reference = strings.TrimSpace(method), strings.TrimSpace(reference)
	if method == "" || utf8.RuneCountInString(method) > 60 || utf8.RuneCountInString(reference) > 120 {
		return errors.New("say how you paid (and a reference if there is one)")
	}
	return e.postExpenseAnswer(space, it, ExpenseAnswerData{Answer: "paid", Method: method, Reference: reference}, strings.TrimSpace(note))
}

// ConfirmPayment confirms (as the person who paid the expense) that another parent's payment arrived.
func (e *Engine) ConfirmPayment(space, item, person string) error {
	x, it, err := e.expenseFor(space, item)
	if err != nil {
		return err
	}
	if !x.Mine {
		return errors.New("only the person who paid the expense confirms payments")
	}
	paid := false
	for _, p := range x.paidBy() {
		paid = paid || p == person
	}
	if !paid {
		return errors.New("they haven't recorded a payment for this expense")
	}
	for _, a := range x.Answers {
		if a.Current && a.Answer == "received" && a.For == person {
			return nil
		}
	}
	return e.postExpenseAnswer(space, it, ExpenseAnswerData{Answer: "received", Person: person}, "")
}

func (e *Engine) expenseFor(space, item string) (*Expense, *envelope.Item, error) {
	if _, err := e.requireParent(space); err != nil {
		return nil, nil, err
	}
	cur, err := e.latest(space, item)
	if err != nil {
		return nil, nil, err
	}
	if cur.rec.Item.Kind != envelope.KindExpense || cur.rec.Item.Ref != "" {
		return nil, nil, errors.New("that isn't an expense")
	}
	v, err := e.Expenses(space)
	if err != nil {
		return nil, nil, err
	}
	for i := range v.Expenses {
		if v.Expenses[i].ID == item {
			return &v.Expenses[i], &cur.rec.Item, nil
		}
	}
	return nil, nil, errors.New("that expense isn't on this device yet - try again in a moment")
}

func (e *Engine) postExpenseAnswer(space string, it *envelope.Item, ad ExpenseAnswerData, comment string) error {
	if utf8.RuneCountInString(comment) > envelope.MaxReason {
		return errors.New("the comment is too long")
	}
	h := it.Hash()
	ad.Rev, ad.Hash = it.Rev, base64.StdEncoding.EncodeToString(h[:])
	_, err := e.post(space, envelope.Draft{Kind: envelope.KindExpense, Ref: it.ItemID, Body: envelope.Body{Text: comment, Data: jsonData(ad)}})
	return err
}

// Expenses lists the family's expenses with their answers and payments, and who owes whom.
func (e *Engine) Expenses(space string) (*ExpensesView, error) {
	sv, err := e.spaceView(space)
	if err != nil {
		return nil, err
	}
	all, err := e.loadDecoded(space)
	if err != nil {
		return nil, err
	}
	names := e.names(all)
	parents := map[string]bool{}
	view := &ExpensesView{Parents: []Person{}, Expenses: []Expense{}, Balances: []Owing{}}
	for _, m := range sv.Members {
		if m.Role == "parent" && m.Status == "active" {
			parents[m.Person] = true
			view.Parents = append(view.Parents, Person{ID: m.Person, Name: names.of(m.Person), Me: m.Person == e.person})
		}
	}
	sort.Slice(view.Parents, func(i, j int) bool { return view.Parents[i].Name < view.Parents[j].Name })
	threads, order := threadsOf(all, envelope.KindExpense)
	for _, id := range order {
		view.Expenses = append(view.Expenses, e.expense(threads[id], names, parents))
	}
	sort.SliceStable(view.Expenses, func(i, j int) bool {
		if view.Expenses[i].Data.Date != view.Expenses[j].Data.Date {
			return view.Expenses[i].Data.Date > view.Expenses[j].Data.Date
		}
		return view.Expenses[i].Created.After(view.Expenses[j].Created)
	})

	// Net what each pair owes, per currency, over agreed shares not yet paid.
	type key struct{ a, b, cur string }
	net := map[key]int64{}
	for _, x := range view.Expenses {
		if x.State != "agreed" {
			continue
		}
		paid := map[string]bool{}
		for _, p := range x.paidBy() {
			paid[p] = true
		}
		for p := range parents {
			if p == x.Author || paid[p] || x.Owed == 0 {
				continue
			}
			a, b, amt := p, x.Author, x.Owed // a owes b
			if a > b {
				a, b, amt = b, a, -amt
			}
			net[key{a, b, x.Data.Currency}] += amt
		}
	}
	for k, amt := range net {
		if amt == 0 {
			continue
		}
		from, to := k.a, k.b
		if amt < 0 {
			from, to, amt = to, from, -amt
		}
		view.Balances = append(view.Balances, Owing{From: from, FromName: names.of(from), To: to, ToName: names.of(to), Amount: amt, Currency: k.cur})
	}
	sort.Slice(view.Balances, func(i, j int) bool {
		if view.Balances[i].Currency != view.Balances[j].Currency {
			return view.Balances[i].Currency < view.Balances[j].Currency
		}
		return view.Balances[i].FromName < view.Balances[j].FromName
	})
	for _, x := range view.Expenses {
		if x.NeedsMe {
			view.NeedsMe++
		}
	}
	return view, nil
}

// threadsOf groups the revisions of each item of a kind, and the answers (Ref'd items of that kind) to each.
func threadsOf(all []*decoded, kind string) (map[string]*thread, []string) {
	threads := map[string]*thread{}
	var order []string
	get := func(id string) *thread {
		t := threads[id]
		if t == nil {
			t = &thread{}
			threads[id] = t
		}
		return t
	}
	for _, d := range all {
		it := &d.rec.Item
		if it.Kind != kind {
			continue
		}
		if it.Ref != "" {
			t := get(it.Ref)
			t.answers = append(t.answers, d)
			continue
		}
		t := get(it.ItemID)
		if len(t.revs) == 0 {
			order = append(order, it.ItemID)
		}
		t.revs = append(t.revs, d)
	}
	var kept []string
	for _, id := range order {
		if len(threads[id].revs) > 0 {
			kept = append(kept, id)
		}
	}
	return threads, kept
}

func (e *Engine) expense(t *thread, names nameBook, parents map[string]bool) Expense {
	first := t.revs[0].rec.Item
	cur := t.latest()
	it := &cur.rec.Item
	x := Expense{ID: it.ItemID, Rev: it.Rev, Author: first.Person, AuthorName: names.of(first.Person), Mine: first.Person == e.person,
		Created: first.Created, Updated: it.Created, History: history(t), Answers: []ExpenseAnswer{}}
	if cur.body == nil {
		x.State, x.Problem = "pending", "can't be read on this device"
		return x
	}
	x.Note, x.Reason = cur.body.Text, cur.body.Reason
	body := cur.body
	if body.Retracted { // keep showing what it was
		for i := len(t.revs) - 1; i >= 0; i-- {
			if b := t.revs[i].body; b != nil && !b.Retracted {
				body = b
				x.Note = b.Text
				break
			}
		}
	}
	if json.Unmarshal(body.Data, &x.Data) != nil {
		x.State, x.Problem = "declined", "unreadable expense"
		return x
	}
	x.Owed = x.Data.Amount * int64(x.Data.Share) / 100
	for _, a := range body.Attachments {
		x.Receipts = append(x.Receipts, AttachmentView{BlobID: a.BlobID, Name: a.Name, Mime: a.Mime, Size: a.Size, Width: a.Width,
			Height: a.Height, Thumb: a.Thumb})
	}
	if !parents[first.Person] {
		x.State, x.Problem = "declined", "added by someone who isn't a parent in this family"
		return x
	}

	h := it.Hash()
	want := base64.StdEncoding.EncodeToString(h[:])
	decision := map[string]*ExpenseAnswer{} // each other parent's latest accept/decline of this revision
	paid := map[string]bool{}
	received := map[string]bool{}
	sort.SliceStable(t.answers, func(i, j int) bool { return t.answers[i].rec.Item.Created.Before(t.answers[j].rec.Item.Created) })
	for _, d := range t.answers {
		a := &d.rec.Item
		if d.body == nil || d.body.Retracted || !parents[a.Person] {
			continue
		}
		var ad ExpenseAnswerData
		if json.Unmarshal(d.body.Data, &ad) != nil {
			continue
		}
		byAuthor := a.Person == first.Person
		switch ad.Answer {
		case "accept", "decline", "paid":
			if byAuthor {
				continue
			}
		case "received":
			if !byAuthor || !parents[ad.Person] {
				continue
			}
		default:
			continue
		}
		v := ExpenseAnswer{Person: a.Person, Name: names.of(a.Person), Answer: ad.Answer, Comment: d.body.Text, Method: ad.Method,
			Reference: ad.Reference, Time: a.Created, Current: ad.Rev == it.Rev && ad.Hash == want}
		if ad.Answer == "received" {
			v.For = names.of(ad.Person)
		}
		x.Answers = append(x.Answers, v)
		if !v.Current {
			continue
		}
		switch ad.Answer {
		case "accept", "decline":
			vv := v
			decision[a.Person] = &vv
		case "paid":
			paid[a.Person] = true
		case "received":
			if paid[ad.Person] {
				received[ad.Person] = true
			}
		}
	}
	if cur.body.Retracted {
		x.State = "withdrawn"
		return x
	}
	x.State = "agreed"
	for p := range parents {
		if p == first.Person {
			continue
		}
		if paid[p] { // paying is agreeing
			continue
		}
		d := decision[p]
		switch {
		case d == nil:
			x.State = "pending"
			x.Waiting = append(x.Waiting, names.of(p))
			if p == e.person {
				x.NeedsMe = true
			}
		case d.Answer == "decline":
			x.State = "declined"
		}
	}
	switch x.State {
	case "declined":
		x.Waiting, x.NeedsMe = nil, false
	case "agreed":
		x.Waiting = nil
		allPaid, allReceived, others := true, true, 0
		for p := range parents {
			if p == first.Person {
				continue
			}
			others++
			if !paid[p] {
				allPaid = false
				x.Waiting = append(x.Waiting, names.of(p))
				if p == e.person && x.Owed > 0 {
					x.CanPay = true
				}
			} else if !received[p] {
				allReceived = false
				x.ToConfirm = append(x.ToConfirm, p)
				if x.Mine {
					x.NeedsMe = true
				}
			}
		}
		switch {
		case others == 0 || x.Owed == 0:
		case allPaid && allReceived:
			x.State = "settled"
		case allPaid:
			x.State = "paid"
		}
	}
	sort.Strings(x.Waiting)
	sort.Strings(x.ToConfirm)
	return x
}
