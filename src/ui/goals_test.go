package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/goal"
	"finux/src/transaction"
)

func sampleGoals() []goal.Goal {
	return []goal.Goal{
		{ID: 1, Title: "Trip to Japan", Description: "in 2027", Code: "TRIP", Type: goal.Save, Currency: "BRL", Target: 1000000, DueDate: "2027-06-30", Saved: 250000, Links: 3},
		{ID: 2, Title: "Late Visa bill", Code: "DEBT", Type: goal.PayOff, Currency: "BRL", Target: 80000, DueDate: "2026-11-10", Saved: 30000, Links: 1},
		{ID: 3, Title: "Notebook", Code: "NOTE", Type: goal.Purchase, Currency: "USD", Target: 150000, Saved: 150000, Links: 2},
	}
}

func sampleGoalTransactions() []transaction.View {
	trip := sampleGoals()[0]
	inter := sampleAccounts()[0]
	nubank := sampleAccounts()[1]
	categories := sampleCategories()
	return []transaction.View{
		{Transaction: transaction.Transaction{ID: 9, Title: "To savings", Amount: 100000, Type: transaction.Transfer, Date: "2026-11-15", GoalID: 1, PairID: 8}, Account: nubank, Category: categories[2], Goal: &trip, Other: &inter},
		{Transaction: transaction.Transaction{ID: 8, Title: "To savings", Amount: -100000, Type: transaction.Transfer, Date: "2026-11-15", GoalID: 1, PairID: 9}, Account: inter, Category: categories[2], Goal: &trip, Other: &nubank},
		{Transaction: transaction.Transaction{ID: 7, Title: "Withdrawal", Amount: -50000, Type: transaction.Expense, Date: "2026-11-10", GoalID: 1}, Account: inter, Category: categories[0], Goal: &trip},
		{Transaction: transaction.Transaction{ID: 5, Title: "Bonus", Amount: 200000, Type: transaction.Income, Date: "2026-11-05", GoalID: 1}, Account: inter, Category: categories[1], Goal: &trip},
	}
}

func TestGoalEntries(t *testing.T) {
	goals := sampleGoals()
	goals[2].DeletedAt.Valid = true
	entries := GoalEntries(goals, "2026-11-17")
	if len(entries) != 3 || entries[0].Code != "TRIP" || entries[0].Name != "Trip to Japan" || entries[0].Value != "25% of R$ 10.000,00" {
		t.Fatalf("entries = %+v", entries)
	}
	if !strings.Contains(entries[0].Description, "save") || !strings.Contains(entries[1].Description, "overdue") || !strings.Contains(entries[2].Description, "archived") {
		t.Fatalf("descriptions = %q, %q, %q", entries[0].Description, entries[1].Description, entries[2].Description)
	}
}

func TestGoalTable(t *testing.T) {
	if got := GoalTable(nil, "2026-11-17", true); got != "No goals yet. Run 'finux gl add'.\n" {
		t.Fatalf("empty table = %q", got)
	}
	goals := sampleGoals()
	goals[2].DeletedAt.Valid = true
	bare := GoalTable(goals, "2026-11-17", true)
	if strings.ContainsAny(bare, "╭│\x1b") {
		t.Fatalf("plain table has borders or colors:\n%s", bare)
	}
	lines := strings.Split(bare, "\n")
	if got := strings.Join(strings.Fields(lines[0]), " "); got != "CODE TITLE TYPE DUE STATUS PROGRESS % TARGET" {
		t.Fatalf("headers = %q", got)
	}
	for line, wants := range map[int][]string{
		1: {"TRIP", "Trip to Japan", "save", "30/06/2027", "active", "R$ 2.500,00", "25%", "R$ 10.000,00"},
		2: {"DEBT", "payoff", "10/11/2026", "overdue", "R$ 300,00", "37%", "R$ 800,00"},
		3: {"NOTE", "Notebook (archived)", "purchase", "-", "reached", "$1,500.00", "100%"},
		4: {"3 goals"},
	} {
		for _, want := range wants {
			if !strings.Contains(lines[line], want) {
				t.Errorf("line %d lacks %q: %q", line, want, lines[line])
			}
		}
	}
	styled := GoalTable(goals, "2026-11-17", false)
	if !strings.Contains(styled, "╭") || !strings.Contains(plain(styled), "overdue") {
		t.Fatalf("styled table:\n%s", styled)
	}
	if got := GoalTable(goals[:1], "2026-11-17", true); !strings.HasSuffix(got, "1 goal\n") {
		t.Fatalf("count of one goal: %q", got)
	}
}

func TestGoalDetailsView(t *testing.T) {
	details := GoalDetails{Goal: sampleGoals()[0], Transactions: sampleGoalTransactions(), Today: "2026-11-17"}
	bare := GoalDetailsView(details, true)
	if strings.ContainsAny(bare, "╭│\x1b") {
		t.Fatalf("plain details have borders or colors:\n%s", bare)
	}
	for _, want := range []string{
		"TRIP  Trip to Japan", "in 2027", "Type       save", "Currency   BRL", "Target     R$ 10.000,00",
		"Progress   R$ 2.500,00 (25%)", "Remaining  R$ 7.500,00", "Due        30/06/2027", "Status     active",
		"ID  DATE", "EFFECT", "To savings", "+R$ 1.000,00", "Withdrawal", "-R$ 500,00", "Bonus", "+R$ 2.000,00",
	} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain details lack %q:\n%s", want, bare)
		}
	}
	// A transfer shows one time: the side that leaves its account.
	if strings.Count(bare, "To savings") != 1 || !strings.Contains(bare, "8   15/11/2026") {
		t.Errorf("transfer rows:\n%s", bare)
	}

	styled := plain(GoalDetailsView(details, false))
	for _, want := range []string{"TRIP", "Trip to Japan", "SAVE", "25%", "R$ 2.500,00", "of R$ 10.000,00", "R$ 7.500,00 to go", "until 30/06/2027", "LINKED TRANSACTIONS", "Withdrawal", "+R$ 2.000,00"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled details lack %q:\n%s", want, styled)
		}
	}

	empty := GoalDetailsView(GoalDetails{Goal: sampleGoals()[1], Today: "2026-11-17"}, true)
	for _, want := range []string{"Status     overdue", "No transactions linked to this goal."} {
		if !strings.Contains(empty, want) {
			t.Errorf("empty details lack %q:\n%s", want, empty)
		}
	}
	noDate := GoalDetailsView(GoalDetails{Goal: sampleGoals()[2], Today: "2026-11-17"}, true)
	if strings.Contains(noDate, "Due ") || !strings.Contains(noDate, "Status     reached") {
		t.Errorf("details without a final date:\n%s", noDate)
	}
}

func TestTransactionCardOfAGoal(t *testing.T) {
	view := sampleGoalTransactions()[3]
	bare := TransactionCard(view, true)
	if !strings.Contains(bare, "Goal      TRIP  Trip to Japan") {
		t.Fatalf("plain card lacks the goal:\n%s", bare)
	}
	if styled := plain(TransactionCard(view, false)); !strings.Contains(styled, "Goal") || !strings.Contains(styled, "TRIP  Trip to Japan") {
		t.Fatalf("styled card lacks the goal:\n%s", styled)
	}
	view.Goal = nil
	if strings.Contains(TransactionCard(view, true), "Goal") {
		t.Fatal("a transaction without a goal shows a goal")
	}
}

func TestGoalFormCreateFlow(t *testing.T) {
	var model tea.Model = NewGoalForm(GoalFormOptions{Taken: free})
	view := plain(form(t, model).View().Content)
	for _, want := range []string{"New goal", "Title", "Code", "Description", "Type", "save", "pay off", "purchase", "Currency", "Target", "Final date"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for _, other := range []string{"Opening balance", "Limit", "Closing day", "Color", "Name"} {
		if strings.Contains(view, other) {
			t.Errorf("goal form shows %q:\n%s", other, view)
		}
	}
	model = send(t, model, text("Trip to Japan")...)
	steps := []struct {
		keys  []tea.Msg
		focus Field
		err   string
	}{
		{[]tea.Msg{keyTab}, FieldCode, ""},
		{[]tea.Msg{keyTab}, FieldDescription, ""},
		{[]tea.Msg{keyTab}, FieldKind, ""},
		{[]tea.Msg{keyRight, keyTab}, FieldCurrency, ""},
		{[]tea.Msg{keyRight, keyRight, keyTab}, FieldTarget, ""},
		{[]tea.Msg{keyTab}, FieldTarget, "Target is required"},
		{append(text("0"), keyTab), FieldTarget, "Target must be more than zero"},
		{append([]tea.Msg{keyBack}, append(text("8000"), keyTab)...), FieldDate, ""},
		{append(text("soon"), keyTab), FieldDate, "Use DD/MM/YYYY or YYYY/MM/DD"},
	}
	for index, step := range steps {
		model = send(t, model, step.keys...)
		if f := form(t, model); f.Focus() != step.focus || f.Error() != step.err {
			t.Fatalf("step %d: focus = %v, error %q; want %v, %q", index, f.Focus(), f.Error(), step.focus, step.err)
		}
	}
	model = send(t, model, keyBack, keyBack, keyBack, keyBack)
	model = send(t, model, append(text("30/06/2027"), keyEnter)...)
	f := form(t, model)
	if !f.Submitted() {
		t.Fatalf("form not submitted: error %q", f.Error())
	}
	got := f.Goal()
	if got.Title != "Trip to Japan" || got.Code != "TTJAP" || got.Type != goal.PayOff || got.Currency != "BRL" || got.Target != 800000 || got.DueDate != "2027-06-30" {
		t.Fatalf("goal = %+v", got)
	}
}

func TestGoalFormWithoutDate(t *testing.T) {
	var model tea.Model = NewGoalForm(GoalFormOptions{Taken: free, Initial: goal.Goal{Title: "Car", Code: "CAR", Type: goal.Purchase, Currency: "USD"}, Target: "100"})
	model = send(t, model, keySave)
	f := form(t, model)
	if got := f.Goal(); !f.Submitted() || got.Type != goal.Purchase || got.Currency != "USD" || got.Target != 10000 || got.DueDate != "" {
		t.Fatalf("submitted %v, goal = %+v, error %q", f.Submitted(), got, f.Error())
	}
}

func TestGoalFormEditMode(t *testing.T) {
	trip := sampleGoals()[0]
	f := NewGoalForm(GoalFormOptions{Title: "Edit TRIP", Initial: trip, Target: "10000,00", Edit: true, Taken: free})
	view := plain(f.View().Content)
	if strings.Contains(view, "Currency") || !strings.Contains(view, "10000,00") || !strings.Contains(view, "30/06/2027") ||
		!strings.Contains(view, "save · R$ 10.000,00 · until 30/06/2027") {
		t.Fatalf("edit view:\n%s", view)
	}
	var model tea.Model = f
	model = send(t, model, keySave)
	got := form(t, model).Goal()
	if got.ID != 1 || got.Currency != "BRL" || got.Target != 1000000 || got.DueDate != "2027-06-30" || got.Saved != trip.Saved || got.Type != goal.Save {
		t.Fatalf("goal = %+v", got)
	}
}

func TestTransactionFormGoal(t *testing.T) {
	options := txOptions()
	// Without goals, the form has no goal field.
	if view := plain(NewTransactionForm(options).View().Content); strings.Contains(view, "Goal") {
		t.Fatalf("form without goals shows the field:\n%s", view)
	}
	options.Goals = sampleGoals()
	var model tea.Model = NewTransactionForm(options)
	f := txForm(t, model)
	if view := plain(f.View().Content); !strings.Contains(view, "Goal") || !strings.Contains(view, "No goal") {
		t.Fatalf("form with goals lacks the field:\n%s", view)
	}
	if f.Value(TxGoal) != "" || f.Draft().GoalID != 0 {
		t.Fatalf("start goal = %q, %d", f.Value(TxGoal), f.Draft().GoalID)
	}
	// type, title, value, account (INTER, in BRL), category, goal
	model = step(t, model, TxTitle, keyTab)
	model = step(t, model, TxValue, append(text("Bonus"), keyTab)...)
	model = step(t, model, TxAccount, append(text("100"), keyTab)...)
	model = step(t, model, TxCategory, keyTab)
	model = step(t, model, TxGoal, keyTab)
	// The goals in BRL, then no goal again. NOTE is in USD.
	model = send(t, model, keyRight)
	if got := txForm(t, model).Value(TxGoal); got != "TRIP" {
		t.Fatalf("after right: goal %q, want TRIP", got)
	}
	model = send(t, model, keyRight)
	if got := txForm(t, model).Value(TxGoal); got != "DEBT" {
		t.Fatalf("after right: goal %q, want DEBT", got)
	}
	model = send(t, model, keyRight)
	if got := txForm(t, model).Value(TxGoal); got != "" {
		t.Fatalf("after the last goal: goal %q, want none", got)
	}
	model = send(t, model, tea.KeyPressMsg{Code: 't', Text: "t"})
	f = txForm(t, model)
	if f.Value(TxGoal) != "TRIP" || f.Draft().GoalID != 1 {
		t.Fatalf("after t: goal %q, id %d", f.Value(TxGoal), f.Draft().GoalID)
	}
	// A change to an account in USD drops a goal in BRL.
	model = step(t, model, TxCategory, keyShiftTab)
	model = step(t, model, TxAccount, keyShiftTab)
	model = send(t, model, tea.KeyPressMsg{Code: 'w', Text: "w"})
	f = txForm(t, model)
	if f.Value(TxAccount) != "WISE" || f.Draft().GoalID != 0 {
		t.Fatalf("after WISE: account %q, goal id %d", f.Value(TxAccount), f.Draft().GoalID)
	}

	// An edit starts with the goal of the transaction.
	options.Initial = transaction.Draft{Title: "Bonus", Type: transaction.Income, AccountID: 1, CategoryID: 4, GoalID: 2, Date: "2026-11-17"}
	options.Edit, options.Value = true, "100"
	edit := NewTransactionForm(options)
	if edit.Value(TxGoal) != "DEBT" || edit.Draft().GoalID != 2 {
		t.Fatalf("edit goal = %q, %d", edit.Value(TxGoal), edit.Draft().GoalID)
	}
}
