package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// step answers the question that the bot sent with a button. It
// checks the question and the text that replaces it, and returns the
// next message of the bot.
func (f *fakeTelegram) step(question sent, wantQuestion, label, wantAfter string) sent {
	f.t.Helper()
	if question.text != wantQuestion {
		f.t.Fatalf("question = %q, want %q", question.text, wantQuestion)
	}
	f.press(42, question, label)
	if changed := f.next(); changed.method != "editMessageText" || changed.id != question.id || changed.text != wantAfter || len(changed.buttons) != 0 {
		f.t.Fatalf("after the button %q = %+v, want the text %q", label, changed, wantAfter)
	}
	return f.next()
}

func TestBotGuidesANewTransaction(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)

	f.say(42, "/tx add")
	question := f.next()
	if question.text != "<b>New transaction</b>\nTitle?" || !reflect.DeepEqual(question.buttons, []string{"Cancel"}) {
		t.Fatalf("first question = %+v", question)
	}
	// A text with no command is the answer.
	f.say(42, "Lunch")
	if question = f.next(); question.text != "Value?" {
		t.Fatalf("second question = %+v", question)
	}
	f.say(42, "45,50")
	question = f.next()
	if !reflect.DeepEqual(question.buttons, []string{"INTER", "NU", "WISE", "Cancel"}) {
		t.Fatalf("buttons of the accounts = %v", question.buttons)
	}
	question = f.step(question, "Account or credit card?", "INTER", "Account or credit card: <b>INTER</b>")
	if !slices.Contains(question.buttons, "FOOD") || !slices.Contains(question.buttons, "OTHER") {
		t.Fatalf("buttons of the categories = %v", question.buttons)
	}
	question = f.step(question, "Category?", "FOOD", "Category: <b>FOOD</b>")
	if !reflect.DeepEqual(question.buttons, []string{"Today", "Yesterday", "Cancel"}) {
		t.Fatalf("buttons of the date = %v", question.buttons)
	}
	result := f.step(question, "Date? Press a button or send a date as DD/MM/YYYY.", "Yesterday", "Date: <b>Yesterday</b>")
	if result.text != pre("✓ Transaction 1 created") {
		t.Fatalf("result = %+v", result)
	}
	card := f.ask(42, "/tx 1")
	for _, want := range []string{"Lunch", "Type      expense", "-R$ 45,50", "INTER", "FOOD", "Date      16/11/2026"} {
		if !strings.Contains(card, want) {
			t.Errorf("the transaction has no %q:\n%s", want, card)
		}
	}
	// The questions are over: a text is not an answer now.
	if got := f.ask(42, "Dinner"); got != botSorry {
		t.Fatalf("text after the last question = %q", got)
	}
}

func TestBotGuideAsksTheTypeForAGreyCategory(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)
	f.say(42, "/transactions a")
	f.next()
	f.say(42, "Found money")
	f.next()
	f.say(42, "10")
	question := f.next()
	// A person can write the answer of a question that has buttons.
	f.say(42, "nu")
	if question = f.next(); question.text != "Category?" {
		t.Fatalf("question after a written account = %+v", question)
	}
	question = f.step(question, "Category?", "OTHER", "Category: <b>OTHER</b>")
	question = f.step(question, "Expense or income?", "Income", "Expense or income: <b>Income</b>")
	f.say(42, "05/11/2026")
	if result := f.next(); result.text != pre("✓ Transaction 1 created") {
		t.Fatalf("result = %+v", result)
	}
	if card := f.ask(42, "/tx 1"); !strings.Contains(card, "Type      income") || !strings.Contains(card, "Date      05/11/2026") || !strings.Contains(card, "NU") {
		t.Fatalf("transaction:\n%s", card)
	}
}

func TestBotGuideStops(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)
	if got := f.ask(42, "/cancel"); got != "There is nothing to cancel." {
		t.Fatalf("/cancel with no question = %q", got)
	}
	f.ask(42, "/tx add")
	if got := f.ask(42, "/cancel"); got != "Cancelled." {
		t.Fatalf("/cancel = %q", got)
	}
	if got := f.ask(42, "Lunch"); got != botSorry {
		t.Fatalf("text after /cancel = %q", got)
	}
	// The button Cancel does the same.
	f.say(42, "/tx add")
	question := f.next()
	f.press(42, question, "Cancel")
	if changed := f.next(); changed.method != "editMessageText" || changed.text != "Cancelled." {
		t.Fatalf("after Cancel = %+v", changed)
	}
	if got := f.ask(42, "Lunch"); got != botSorry {
		t.Fatalf("text after Cancel = %q", got)
	}
	// Another command stops the questions.
	f.ask(42, "/tx add")
	if got := f.ask(42, "/ac"); !strings.Contains(got, "INTER") {
		t.Fatalf("/ac in the middle of the questions = %q", got)
	}
	if got := f.ask(42, "Lunch"); got != botSorry {
		t.Fatalf("text after another command = %q", got)
	}
	// An old button does nothing.
	f.press(42, question, "Cancel")
	f.quiet()
	if got := succeed(t, home, "tx"); !strings.Contains(got, "No transactions yet") {
		t.Fatalf("questions that stopped made a transaction:\n%s", got)
	}
}

func TestBotGuideSkipsAnOptionalValue(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)
	f.say(42, "/ac add")
	if question := f.next(); question.text != "<b>New account</b>\nName?" {
		t.Fatalf("first question = %+v", question)
	}
	f.say(42, "Savings box")
	question := f.next()
	if !slices.Contains(question.buttons, "BRL") || !slices.Contains(question.buttons, "USD") {
		t.Fatalf("buttons of the currency = %v", question.buttons)
	}
	question = f.step(question, "Currency?", "BRL", "Currency: <b>BRL</b>")
	if !reflect.DeepEqual(question.buttons, []string{"Skip", "Cancel"}) {
		t.Fatalf("buttons of an optional value = %v", question.buttons)
	}
	result := f.step(question, "Opening balance? It is optional.", "Skip", "Opening balance: <b>skipped</b>")
	if !strings.HasPrefix(result.text, "<pre>✓ Account ") || !strings.HasSuffix(result.text, " created</pre>") {
		t.Fatalf("result = %+v", result)
	}
	if got := f.ask(42, "/ac"); !strings.Contains(got, "Savings box") {
		t.Fatalf("/ac = %q", got)
	}
	// A wrong value comes back as the error of the command line.
	f.ask(42, "/ct add")
	f.say(42, "Pets")
	question = f.next()
	if result = f.step(question, "Color? Red is for expenses, green for incomes, grey for all.", "red", "Color: <b>red</b>"); !strings.Contains(result.text, "✓ Category") {
		t.Fatalf("result of /ct add = %+v", result)
	}
}

func TestBotShowsTheItemsWhenACommandHasNoReference(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)
	f.ask(42, "/tx add --title Lunch --value 45 --account INTER --category FOOD")
	f.ask(42, "/tx add --title Dinner --value 60 --account INTER --category FOOD")

	f.say(42, "/tx show")
	question := f.next()
	if !reflect.DeepEqual(question.buttons, []string{"2 Dinner", "1 Lunch", "Cancel"}) {
		t.Fatalf("buttons of the transactions = %v", question.buttons)
	}
	card := f.step(question, "Which transaction?\n<code>/tx show</code>", "1 Lunch", "<code>/tx show 1</code>")
	if !strings.Contains(card.text, "Lunch") || !strings.Contains(card.text, "ID        1") {
		t.Fatalf("card = %+v", card)
	}

	// A delete asks the item, then asks again before it deletes.
	f.say(42, "/ac delete")
	question = f.next()
	if !reflect.DeepEqual(question.buttons, []string{"INTER", "NU", "WISE", "Cancel"}) {
		t.Fatalf("buttons of the accounts = %v", question.buttons)
	}
	confirm := f.step(question, "Which account?\n<code>/ac delete</code>", "NU", "<code>/ac delete NU</code>")
	if !strings.HasPrefix(confirm.text, "This deletes data") || !reflect.DeepEqual(confirm.buttons, []string{"Confirm", "Cancel"}) {
		t.Fatalf("confirmation = %+v", confirm)
	}
	f.press(42, confirm, "Confirm")
	f.next()
	if result := f.next(); !strings.Contains(result.text, "✓ Account NU") {
		t.Fatalf("result = %+v", result)
	}
	// Restore shows the archived items.
	f.say(42, "/ac restore")
	if question = f.next(); !reflect.DeepEqual(question.buttons, []string{"NU", "Cancel"}) {
		t.Fatalf("buttons of restore = %v", question.buttons)
	}
	f.press(42, question, "Cancel")
	if changed := f.next(); changed.text != "Cancelled:\n<code>/ac restore</code>" {
		t.Fatalf("after Cancel = %+v", changed)
	}
	// With no item, the error of the command line comes back.
	if got := f.ask(42, "/gl show"); !strings.Contains(got, "a goal code or id is necessary") {
		t.Fatalf("/gl show with no goals = %q", got)
	}
}

func TestBotGuideAsksTheVirtualCardForACreditCard(t *testing.T) {
	home := botHome(t)
	succeed(t, home, "cc", "add", "--name", "Visa Gold", "--currency", "BRL", "--limit", "5000", "--closing", "25", "--due", "5", "--code", "VISA")
	f := newFakeTelegram(t)
	startDaemon(t, home, f)

	f.say(42, "/tx add")
	f.next()
	f.say(42, "Streaming")
	f.next()
	f.say(42, "40")
	question := f.next()
	// Only a credit card has the question of the virtual card.
	question = f.step(question, "Account or credit card?", "VISA", "Account or credit card: <b>VISA</b>")
	if !reflect.DeepEqual(question.buttons, []string{"No", "Yes", "Cancel"}) {
		t.Fatalf("buttons of the virtual card = %v", question.buttons)
	}
	question = f.step(question, "Paid with a virtual card?", "Yes", "Virtual card: <b>Yes</b>")
	// The question of the limit comes after it.
	if !reflect.DeepEqual(question.buttons, []string{"No", "Yes", "Cancel"}) {
		t.Fatalf("buttons of the limit = %v", question.buttons)
	}
	question = f.step(question, "Outside the limit of the card?", "Yes", "Off limit: <b>Yes</b>")
	question = f.step(question, "Category?", "FOOD", "Category: <b>FOOD</b>")
	result := f.step(question, "Date? Press a button or send a date as DD/MM/YYYY.", "Today", "Date: <b>Today</b>")
	if result.text != pre("✓ Transaction 1 created") {
		t.Fatalf("result = %+v", result)
	}
	if card := f.ask(42, "/tx 1"); !strings.Contains(card, "Virtual   yes") || !strings.Contains(card, "Off limit yes") || !strings.Contains(card, "VISA") {
		t.Fatalf("the transaction has no mark:\n%s", card)
	}
}

func TestBotGuidesANewNoteAndMarksItDone(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)

	f.say(42, "/nt add")
	question := f.next()
	if question.text != "<b>New note</b>\nTitle?" {
		t.Fatalf("first question = %+v", question)
	}
	f.say(42, "Ask about the fee")
	if question = f.next(); question.text != "Text? What the note tells, in a few words. It is optional." || !slices.Contains(question.buttons, "Skip") {
		t.Fatalf("second question = %+v", question)
	}
	f.say(42, "The bank took R$ 12 in November.")
	if question = f.next(); question.text != "Reminder date, as DD/MM/YYYY? It is optional." {
		t.Fatalf("third question = %+v", question)
	}
	f.say(42, "20/11/2026")
	if question = f.next(); question.text != "Tags? It is optional." {
		t.Fatalf("fourth question = %+v", question)
	}
	f.say(42, "bank")
	if result := f.next(); result.text != pre("✓ Note 1 created") {
		t.Fatalf("result = %+v", result)
	}
	card := f.ask(42, "/nt 1")
	for _, want := range []string{"Ask about the fee", "The bank took R$ 12 in November.", "Remind    20/11/2026", "Tags      #bank", "Source    user"} {
		if !strings.Contains(card, want) {
			t.Errorf("the note has no %q:\n%s", want, card)
		}
	}

	// A command without an id shows the open notes.
	f.say(42, "/nt done")
	question = f.next()
	if !reflect.DeepEqual(question.buttons, []string{"1 Ask about the fee", "Cancel"}) {
		t.Fatalf("buttons of the notes = %v", question.buttons)
	}
	if result := f.step(question, "Which note?\n<code>/nt done</code>", "1 Ask about the fee", "<code>/nt done 1</code>"); result.text != pre("✓ Note 1 done") {
		t.Fatalf("result of done = %+v", result)
	}
}
