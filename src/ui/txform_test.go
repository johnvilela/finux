package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/transaction"
)

func txForm(t *testing.T, model tea.Model) TransactionForm {
	t.Helper()
	f, ok := model.(TransactionForm)
	if !ok {
		t.Fatalf("model is %T, want TransactionForm", model)
	}
	return f
}

func txOptions() TransactionFormOptions {
	return TransactionFormOptions{
		Accounts: []account.Account{
			{ID: 1, Name: "Banco Inter", Code: "INTER", Color: "orange", Currency: "BRL"},
			{ID: 2, Name: "Nubank", Code: "NUBAN", Color: "purple", Currency: "BRL"},
			{ID: 3, Name: "Wise", Code: "WISE", Color: "lime", Currency: "USD"},
		},
		Categories: []category.Category{
			{ID: 1, Name: "Bills", Code: "BILLS", Color: "red"},
			{ID: 2, Name: "Food", Code: "FOOD", Color: "red"},
			{ID: 3, Name: "Other", Code: "OTHER", Color: "grey"},
			{ID: 4, Name: "Salary", Code: "SAL", Color: "green"},
			{ID: 5, Name: "Transfer", Code: "XFER", Color: "grey"},
		},
		Titles: []string{"Market", "Rent", "Marathon ticket"},
		Tags:   []string{"#fixed", "#market", "#weekly"},
		Today:  "2026-11-17",
	}
}

// step sends keys and checks the field that then has the focus.
func step(t *testing.T, model tea.Model, want TxField, keys ...tea.Msg) tea.Model {
	t.Helper()
	model = send(t, model, keys...)
	if f := txForm(t, model); f.Focus() != want {
		t.Fatalf("focus = %v, want %v (error %q)\n%s", f.Focus(), want, f.Error(), plain(f.View().Content))
	}
	return model
}

func TestTransactionFormExpenseFlow(t *testing.T) {
	var model tea.Model = NewTransactionForm(txOptions())
	f := txForm(t, model)
	if f.Focus() != TxType || f.Value(TxType) != transaction.Expense || f.Value(TxDate) != "17/11/2026" {
		t.Fatalf("start: focus %v, type %q, date %q", f.Focus(), f.Value(TxType), f.Value(TxDate))
	}
	view := plain(f.View().Content)
	for _, want := range []string{"New transaction", "Type", "expense", "income", "transfer", "Title", "Value", "Account", "INTER", "Category", "BILLS", "Date", "Tags", "Description"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"To account", "Received"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("expense form shows %q:\n%s", unwanted, view)
		}
	}

	model = step(t, model, TxTitle, keyEnter)
	// The title suggests a title in use. Tab accepts the suggestion and
	// stays in the field; the next tab goes on.
	model = step(t, model, TxTitle, append(text("mar"), keyTab)...)
	if got := txForm(t, model).Value(TxTitle); got != "Market" {
		t.Fatalf("accepted suggestion = %q", got)
	}
	model = step(t, model, TxValue, keyTab)
	model = step(t, model, TxInstallments, append(text("50,5"), keyTab)...)
	model = step(t, model, TxAccount, keyTab)
	model = step(t, model, TxCategory, keyRight, keyTab)
	if got := txForm(t, model).Value(TxAccount); got != "NUBAN" {
		t.Fatalf("account = %q", got)
	}
	// A letter goes to the next choice that starts with it.
	model = step(t, model, TxDate, append(text("f"), keyEnter)...)
	if got := txForm(t, model).Value(TxCategory); got != "FOOD" {
		t.Fatalf("category = %q", got)
	}
	model = step(t, model, TxTags, keyTab)
	// A tag in use completes the word the user types.
	model = step(t, model, TxTags, append(text("#mar"), keyTab)...)
	model = step(t, model, TxTags, append(text(" #WE"), keyTab)...)
	if got := txForm(t, model).Value(TxTags); got != "#market #weekly" {
		t.Fatalf("tags = %q", got)
	}
	model = step(t, model, TxDescription, keyTab)
	model = send(t, model, append(text("fruit"), keyEnter)...)
	f = txForm(t, model)
	if !f.Submitted() || f.Cancelled() {
		t.Fatalf("submitted %v, cancelled %v, error %q", f.Submitted(), f.Cancelled(), f.Error())
	}
	want := transaction.Draft{
		Title: "Market", Description: "fruit", Type: transaction.Expense, Value: 5050,
		AccountID: 2, CategoryID: 2, Date: "2026-11-17", Tags: "#market #weekly",
	}
	if got := f.Draft(); got != want {
		t.Fatalf("draft = %+v\nwant    %+v", got, want)
	}
	if f.View().Content != "" {
		t.Fatal("closed form still has a view")
	}
}

func TestTransactionFormSuggestionIsOptional(t *testing.T) {
	var model tea.Model = NewTransactionForm(txOptions())
	model = step(t, model, TxTitle, keyEnter)
	model = send(t, model, text("Mar")...)
	if view := plain(txForm(t, model).View().Content); !strings.Contains(view, "Market") || !strings.Contains(view, "tab") {
		t.Fatalf("no suggestion in the view:\n%s", view)
	}
	// Down shows the next title that matches.
	model = step(t, model, TxTitle, keyDown, keyTab)
	if got := txForm(t, model).Value(TxTitle); got != "Marathon ticket" {
		t.Fatalf("second suggestion = %q", got)
	}
	// Enter keeps what the user typed.
	model = send(t, NewTransactionForm(txOptions()), keyEnter)
	model = step(t, model, TxValue, append(text("Mar"), keyEnter)...)
	if got := txForm(t, model).Value(TxTitle); got != "Mar" {
		t.Fatalf("title = %q, want the typed text", got)
	}
	// A title that no transaction has is accepted.
	model = send(t, NewTransactionForm(txOptions()), keyEnter)
	model = step(t, model, TxValue, append(text("Cinema"), keyTab)...)
	// A word without # gets no tag suggestion, and a new tag is accepted.
	options := txOptions()
	options.Preset = []TxField{TxType, TxTitle, TxValue, TxAccount, TxCategory, TxDate}
	model = step(t, NewTransactionForm(options), TxDescription, append(text("mar"), keyTab)...)
	if got := txForm(t, model).Value(TxTags); got != "mar" {
		t.Fatalf("tags = %q", got)
	}
	model = step(t, NewTransactionForm(options), TxTags, text("#fixed #f")...)
	if view := plain(txForm(t, model).View().Content); strings.Contains(view, "#fixed #fixed") {
		t.Fatalf("suggested a tag the field already has:\n%s", view)
	}
}

func TestTransactionFormTransfer(t *testing.T) {
	var model tea.Model = NewTransactionForm(txOptions())
	model = step(t, model, TxTitle, keyRight, keyRight, keyRight, keyEnter)
	f := txForm(t, model)
	if f.Value(TxType) != transaction.Transfer || f.Value(TxCategory) != "XFER" || f.Value(TxTo) != "NUBAN" {
		t.Fatalf("type %q, category %q, to %q", f.Value(TxType), f.Value(TxCategory), f.Value(TxTo))
	}
	view := plain(f.View().Content)
	if !strings.Contains(view, "To account") || strings.Contains(view, "Received") {
		t.Fatalf("transfer in one currency:\n%s", view)
	}
	model = step(t, model, TxValue, append(text("Travel"), keyEnter)...)
	model = step(t, model, TxAccount, append(text("550"), keyEnter)...)
	model = step(t, model, TxTo, keyEnter)
	// The destination list does not have the source account.
	model = step(t, model, TxTo, keyLeft)
	if got := txForm(t, model).Value(TxTo); got != "WISE" {
		t.Fatalf("to after left = %q, want WISE (the list wraps)", got)
	}
	// Another currency: the form asks the received value.
	model = step(t, model, TxReceived, keyEnter)
	model = step(t, model, TxReceived, keyEnter)
	if got := txForm(t, model).Error(); !strings.Contains(got, "required") {
		t.Fatalf("empty received value: error %q", got)
	}
	model = step(t, model, TxCategory, append(text("100.00"), keyEnter)...)
	// Only neutral categories are for transfers.
	model = step(t, model, TxCategory, keyRight, keyRight)
	if got := txForm(t, model).Value(TxCategory); got != "XFER" {
		t.Fatalf("category after two moves = %q, want XFER", got)
	}
	model = send(t, model, keySave)
	f = txForm(t, model)
	if !f.Submitted() {
		t.Fatalf("not submitted: %q", f.Error())
	}
	want := transaction.Draft{
		Title: "Travel", Type: transaction.Transfer, Value: 55000, AccountID: 1, ToAccountID: 3, Received: 10000,
		CategoryID: 5, Date: "2026-11-17",
	}
	if got := f.Draft(); got != want {
		t.Fatalf("draft = %+v\nwant    %+v", got, want)
	}

	// When the source becomes the destination, the destination changes.
	options := txOptions()
	options.Preset = []TxField{TxTitle, TxValue}
	model = step(t, NewTransactionForm(options), TxAccount, keyRight, keyRight, keyEnter, keyRight)
	if f := txForm(t, model); f.Value(TxAccount) != "NUBAN" || f.Value(TxTo) != "INTER" {
		t.Fatalf("account %q, to %q", f.Value(TxAccount), f.Value(TxTo))
	}

	// One account is not enough for a transfer.
	options = txOptions()
	options.Accounts = options.Accounts[:1]
	if view := plain(NewTransactionForm(options).View().Content); strings.Contains(view, "transfer") {
		t.Fatalf("form with one account offers a transfer:\n%s", view)
	}
}

func TestTransactionFormCategoriesFollowType(t *testing.T) {
	codes := func(model tea.Model) string {
		var seen []string
		for range 6 {
			code := txForm(t, model).Value(TxCategory)
			if len(seen) > 0 && code == seen[0] {
				break
			}
			seen = append(seen, code)
			model = send(t, model, keyRight)
		}
		return strings.Join(seen, " ")
	}
	options := txOptions()
	options.Preset = []TxField{TxTitle, TxValue, TxAccount}
	var model tea.Model = NewTransactionForm(options)
	if got := codes(step(t, model, TxCategory, keyEnter)); got != "BILLS FOOD OTHER XFER" {
		t.Fatalf("expense categories = %s", got)
	}
	if got := codes(step(t, model, TxCategory, keyRight, keyEnter)); got != "OTHER SAL XFER" {
		t.Fatalf("income categories = %s", got)
	}
	// A category that the new type accepts stays selected.
	model = step(t, model, TxCategory, keyEnter)
	model = step(t, model, TxType, keyRight, keyRight, keyShiftTab)
	model = step(t, model, TxCategory, keyRight, keyEnter)
	if got := txForm(t, model).Value(TxCategory); got != "OTHER" {
		t.Fatalf("category after the type changed = %q, want OTHER", got)
	}
}

func TestTransactionFormValidation(t *testing.T) {
	var model tea.Model = NewTransactionForm(txOptions())
	model = step(t, model, TxTitle, keyEnter)
	model = step(t, model, TxTitle, keyEnter)
	if got := txForm(t, model).Error(); got != "Title is required" {
		t.Fatalf("error = %q", got)
	}
	model = step(t, model, TxValue, append(text("Market"), keyEnter)...)
	if txForm(t, model).Error() != "" {
		t.Fatal("error stays after the field is valid")
	}
	for input, want := range map[string]string{"": "required", "abc": "number", "0": "more than zero", "-5": "more than zero", "1,234": "decimal"} {
		next := step(t, model, TxValue, append(text(input), keyEnter)...)
		if got := txForm(t, next).Error(); !strings.Contains(got, want) {
			t.Errorf("value %q: error %q, want %q", input, got, want)
		}
	}
	model = step(t, model, TxDate, append(text("10"), keyEnter, keyEnter, keyEnter, keyEnter)...)
	for range 10 {
		model = send(t, model, keyBack)
	}
	model = step(t, model, TxDate, append(text("31/02/2026"), keyEnter)...)
	if got := txForm(t, model).Error(); !strings.Contains(got, "DD/MM/YYYY") {
		t.Fatalf("date error = %q", got)
	}
	for range 10 {
		model = send(t, model, keyBack)
	}
	model = step(t, model, TxTags, append(text("2026/12/01"), keyEnter)...)
	model = step(t, model, TxTags, append(text("#a-b"), keyEnter)...)
	if got := txForm(t, model).Error(); !strings.Contains(got, "letters, digits") {
		t.Fatalf("tag error = %q", got)
	}
	// Save goes to the first field that is not valid.
	model = step(t, model, TxTags, keySave)
	if txForm(t, model).Submitted() {
		t.Fatal("form with an invalid tag was saved")
	}
	model = send(t, model, keyEscape)
	if f := txForm(t, model); !f.Cancelled() || f.Submitted() {
		t.Fatal("escape did not cancel")
	}
}

func TestTransactionFormEdit(t *testing.T) {
	options := txOptions()
	options.Edit, options.Title = true, "Edit #7"
	options.Initial = transaction.Draft{
		Title: "Market", Description: "fruit", Type: transaction.Expense, AccountID: 3, CategoryID: 2,
		Date: "2026-10-01", Tags: "#market",
	}
	options.Value = "50.00"
	var model tea.Model = NewTransactionForm(options)
	f := txForm(t, model)
	view := plain(f.View().Content)
	if f.Value(TxTitle) != "Market" || f.Value(TxValue) != "50.00" || f.Value(TxAccount) != "WISE" || f.Value(TxCategory) != "FOOD" ||
		f.Value(TxDate) != "01/10/2026" || f.Value(TxTags) != "#market" || f.Value(TxDescription) != "fruit" || !strings.Contains(view, "Edit #7") {
		t.Fatalf("edit form does not start with the transaction:\n%s", view)
	}
	// An expense can become an income, not a transfer.
	if strings.Contains(view, "transfer") {
		t.Fatalf("edit of an expense offers a transfer:\n%s", view)
	}
	model = send(t, model, keySave)
	want := options.Initial
	want.Value = 5000
	if got := txForm(t, model).Draft(); got != want {
		t.Fatalf("draft without changes = %+v\nwant %+v", got, want)
	}

	// A transfer keeps its type and its accounts.
	options.Initial.Type, options.Initial.CategoryID = transaction.Transfer, 5
	f = NewTransactionForm(options)
	view = plain(f.View().Content)
	if f.Focus() != TxTitle || strings.Contains(view, "Account") || strings.Contains(view, "Type") || strings.Contains(view, "Received") {
		t.Fatalf("edit of a transfer:\n%s", view)
	}
	model = send(t, f, keySave)
	if got := txForm(t, model).Draft(); got.Type != transaction.Transfer || got.AccountID != 3 || got.CategoryID != 5 || got.Value != 5000 {
		t.Fatalf("transfer draft = %+v", got)
	}
}

func TestTransactionFormPreset(t *testing.T) {
	options := txOptions()
	options.Initial = transaction.Draft{Title: "Rent", Type: transaction.Income, AccountID: 2}
	options.Value = "10"
	options.Preset = []TxField{TxType, TxTitle, TxValue, TxAccount}
	f := NewTransactionForm(options)
	view := plain(f.View().Content)
	if f.Focus() != TxCategory || f.Value(TxCategory) != "OTHER" || strings.Contains(view, "Title") || strings.Contains(view, "Value") {
		t.Fatalf("preset fields are in the form:\n%s", view)
	}
	model := send(t, f, keySave)
	want := transaction.Draft{Title: "Rent", Type: transaction.Income, Value: 1000, AccountID: 2, CategoryID: 3, Date: "2026-11-17"}
	if got := txForm(t, model).Draft(); got != want {
		t.Fatalf("draft = %+v\nwant    %+v", got, want)
	}
}

func TestTransactionFormVirtualCard(t *testing.T) {
	options := txOptions()
	options.Cards = []card.Card{{ID: 7, Name: "Visa Gold", Code: "VISA", Color: "blue", Currency: "BRL"}}
	var model tea.Model = NewTransactionForm(options)
	// An account has no virtual card.
	if view := plain(txForm(t, model).View().Content); strings.Contains(view, "Virtual card") {
		t.Fatalf("the form of an account shows the field:\n%s", view)
	}
	model = step(t, model, TxTitle, keyEnter)
	model = step(t, model, TxValue, append(text("Shop"), keyTab)...)
	model = step(t, model, TxInstallments, append(text("10"), keyTab)...)
	model = step(t, model, TxAccount, keyTab)
	// The field comes after a credit card. It starts with no.
	model = step(t, model, TxVirtual, append(text("v"), keyTab)...)
	f := txForm(t, model)
	if view := plain(f.View().Content); f.Value(TxAccount) != "VISA" || f.Value(TxVirtual) != "no" || !strings.Contains(view, "Virtual card") {
		t.Fatalf("account %q, virtual %q:\n%s", f.Value(TxAccount), f.Value(TxVirtual), view)
	}
	model = step(t, model, TxCategory, keyRight, keyTab)
	model = send(t, model, keySave)
	f = txForm(t, model)
	if got := f.Draft(); !f.Submitted() || !got.Virtual || got.CardID != 7 || got.AccountID != 0 {
		t.Fatalf("submitted %v, error %q, draft = %+v", f.Submitted(), f.Error(), got)
	}

	// An edit starts with the mark of the transaction. A change to an
	// account takes the mark away.
	options.Edit = true
	options.Initial = transaction.Draft{Title: "Shop", Type: transaction.Expense, CardID: 7, CategoryID: 2, Date: "2026-11-17", Virtual: true}
	options.Value = "10"
	model = NewTransactionForm(options)
	if got := txForm(t, model).Value(TxVirtual); got != "yes" {
		t.Fatalf("virtual of the edit = %q", got)
	}
	model = step(t, model, TxTitle, keyEnter)
	model = step(t, model, TxValue, keyTab)
	model = step(t, model, TxAccount, keyTab)
	model = step(t, model, TxCategory, append(text("i"), keyTab)...)
	model = send(t, model, keySave)
	f = txForm(t, model)
	if got := f.Draft(); !f.Submitted() || got.Virtual || got.AccountID != 1 || got.CardID != 0 {
		t.Fatalf("submitted %v, error %q, draft = %+v", f.Submitted(), f.Error(), got)
	}
}

func TestTransactionFormUsesTheTerminalWidth(t *testing.T) {
	var model tea.Model = NewTransactionForm(txOptions())
	for _, width := range []int{60, 120, 200} {
		view := txForm(t, send(t, model, tea.WindowSizeMsg{Width: width, Height: 40})).View().Content
		if narrowest, widest := boxWidths(view); narrowest != width || widest != width {
			t.Fatalf("width %d: the lines have %d to %d columns:\n%s", width, narrowest, widest, plain(view))
		}
	}
}
