package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/budget"
	"finux/src/transaction"
)

const budgetToday = "2026-11-18"

// sampleBudgets returns a budget near its limit, one over its limit with
// rollover, a one-time budget that did not start and one that ended.
func sampleBudgets() []budget.Budget {
	budgets := []budget.Budget{
		{ID: 1, Title: "Food", Description: "market and restaurants", Code: "FOOD", Currency: "BRL", Limit: 80000, StartDate: "2026-09-01", Every: 1, Unit: budget.Month, CategoryIDs: []uint{1}},
		{ID: 2, Title: "Pay cycle", Code: "CYCLE", Currency: "BRL", Limit: 50000, StartDate: "2026-10-05", Every: 1, Unit: budget.Month, Rollover: true},
		{ID: 3, Title: "Trip", Code: "TRIP", Currency: "USD", Limit: 300000, StartDate: "2026-12-01", EndDate: "2026-12-31"},
		{ID: 4, Title: "Party", Code: "PARTY", Currency: "BRL", Limit: 20000, StartDate: "2026-10-01", EndDate: "2026-10-31"},
	}
	spent := []map[string]int64{
		{"2026-09-10": 50000, "2026-10-10": 90000, "2026-11-03": 70000},
		{"2026-10-20": 60000, "2026-11-10": 45000},
		nil,
		{"2026-10-15": 5000},
	}
	for i := range budgets {
		budgets[i].Usages = budgets[i].Fold(budgets[i].Periods(budgetToday), spent[i])
	}
	return budgets
}

func sampleBudgetTransactions() []transaction.View {
	inter := sampleAccounts()[0]
	categories := sampleCategories()
	return []transaction.View{
		{Transaction: transaction.Transaction{ID: 12, Title: "Refund", Amount: 5000, Type: transaction.Income, Date: "2026-11-12"}, Account: inter, Category: categories[0]},
		{Transaction: transaction.Transaction{ID: 9, Title: "Market", Amount: -75000, Type: transaction.Expense, Date: "2026-11-03"}, Account: inter, Category: categories[0]},
	}
}

func TestBudgetEntries(t *testing.T) {
	budgets := sampleBudgets()
	budgets[3].DeletedAt.Valid = true
	entries := BudgetEntries(budgets, budgetToday)
	if len(entries) != 4 || entries[0].Code != "FOOD" || entries[0].Name != "Food" || entries[0].Value != "87% of R$ 800,00" {
		t.Fatalf("entries = %+v", entries)
	}
	for i, want := range []string{"every month  near  market and restaurants", "every month  over", "one time  upcoming", "archived  one time  ended"} {
		if entries[i].Description != want {
			t.Errorf("description %d = %q, want %q", i, entries[i].Description, want)
		}
	}
	// Rollover: October left -R$ 100,00, so November has R$ 400,00.
	if entries[1].Value != "112% of R$ 400,00" || entries[2].Value != "0% of $3,000.00" {
		t.Errorf("values = %q, %q", entries[1].Value, entries[2].Value)
	}
}

func TestBudgetTable(t *testing.T) {
	if got := BudgetTable(nil, budgetToday, true); got != "No budgets yet. Run 'finux bg add'.\n" {
		t.Fatalf("empty table = %q", got)
	}
	budgets := sampleBudgets()
	budgets[3].DeletedAt.Valid = true
	bare := BudgetTable(budgets, budgetToday, true)
	if strings.ContainsAny(bare, "╭│\x1b") {
		t.Fatalf("plain table has borders or colors:\n%s", bare)
	}
	lines := strings.Split(bare, "\n")
	if got := strings.Join(strings.Fields(lines[0]), " "); got != "CODE TITLE PERIOD UNTIL STATUS SPENT % AVAILABLE" {
		t.Fatalf("headers = %q", got)
	}
	for line, wants := range map[int][]string{
		1: {"FOOD", "Food", "every month", "30/11/2026", "near", "R$ 700,00", "87%", "R$ 800,00"},
		2: {"CYCLE", "Pay cycle", "04/12/2026", "over", "R$ 450,00", "112%", "R$ 400,00"},
		3: {"TRIP", "one time", "31/12/2026", "upcoming", "$0.00", "0%", "$3,000.00"},
		4: {"PARTY", "Party (archived)", "31/10/2026", "ended", "R$ 50,00", "25%", "R$ 200,00"},
		5: {"4 budgets"},
	} {
		for _, want := range wants {
			if !strings.Contains(lines[line], want) {
				t.Errorf("line %d lacks %q: %q", line, want, lines[line])
			}
		}
	}
	styled := BudgetTable(budgets, budgetToday, false)
	if !strings.Contains(styled, "╭") || !strings.Contains(plain(styled), "upcoming") {
		t.Fatalf("styled table:\n%s", styled)
	}
	if got := BudgetTable(budgets[:1], budgetToday, true); !strings.HasSuffix(got, "1 budget\n") {
		t.Fatalf("count of one budget: %q", got)
	}
}

func TestBudgetDetailsView(t *testing.T) {
	food := sampleBudgets()[0]
	usage, _ := food.Current(budgetToday)
	details := BudgetDetails{
		Budget: food, Usage: &usage, Categories: sampleCategories()[:1], Transactions: sampleBudgetTransactions(),
		History: []budget.Usage{food.Usages[1], food.Usages[0]}, Today: budgetToday,
	}
	bare := BudgetDetailsView(details, true)
	if strings.ContainsAny(bare, "╭│\x1b") {
		t.Fatalf("plain details have borders or colors:\n%s", bare)
	}
	for _, want := range []string{
		"FOOD  Food", "market and restaurants", "Currency    BRL", "Limit       R$ 800,00", "Period      every month",
		"Start       01/09/2026", "Rollover    no", "Categories  FOOD", "Status      near",
		"Current period: 01/11/2026 to 30/11/2026", "Available   R$ 800,00", "Spent       R$ 700,00 (87%)", "Remaining   R$ 100,00",
		"ID  DATE", "AMOUNT", "Refund", "-R$ 50,00", "Market", "R$ 750,00",
		"History", "PERIOD", "01/10/2026 to 31/10/2026", "R$ 900,00", "112%", "over", "01/09/2026 to 30/09/2026", "62%", "ok",
	} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain details lack %q:\n%s", want, bare)
		}
	}
	if strings.Contains(bare, "Carried") || strings.Contains(bare, "End ") {
		t.Errorf("details without rollover or end date show them:\n%s", bare)
	}

	styled := plain(BudgetDetailsView(details, false))
	for _, want := range []string{"FOOD", "Food", "EVERY MONTH", "NEAR", "87%", "R$ 700,00", "of R$ 800,00", "R$ 100,00 left", "01/11/2026 to 30/11/2026", "TRANSACTIONS OF THE PERIOD", "Market", "HISTORY", "01/10/2026"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled details lack %q:\n%s", want, styled)
		}
	}

	// Rollover, over the limit, all categories, no transactions.
	cycle := sampleBudgets()[1]
	usage, _ = cycle.Current(budgetToday)
	rolled := BudgetDetailsView(BudgetDetails{Budget: cycle, Usage: &usage, Today: budgetToday}, true)
	for _, want := range []string{
		"Rollover    yes", "Categories  all", "Status      over", "Current period: 05/11/2026 to 04/12/2026",
		"Carried     -R$ 100,00", "Available   R$ 400,00", "Spent       R$ 450,00 (112%)", "Over        R$ 50,00",
		"No transactions count in this period.",
	} {
		if !strings.Contains(rolled, want) {
			t.Errorf("rollover details lack %q:\n%s", want, rolled)
		}
	}
	if strings.Contains(rolled, "History") || strings.Contains(rolled, "Remaining") {
		t.Errorf("details without history show it:\n%s", rolled)
	}
	if over := plain(BudgetDetailsView(BudgetDetails{Budget: cycle, Usage: &usage, Today: budgetToday}, false)); !strings.Contains(over, "R$ 50,00 over") {
		t.Errorf("styled details over the limit:\n%s", over)
	}

	// A budget that did not start has no period.
	trip := sampleBudgets()[2]
	upcoming := BudgetDetailsView(BudgetDetails{Budget: trip, Today: budgetToday}, true)
	for _, want := range []string{"Period      one time", "Start       01/12/2026", "End         31/12/2026", "Status      upcoming", "The budget starts on 01/12/2026."} {
		if !strings.Contains(upcoming, want) {
			t.Errorf("upcoming details lack %q:\n%s", want, upcoming)
		}
	}
	if styled := plain(BudgetDetailsView(BudgetDetails{Budget: trip, Today: budgetToday}, false)); !strings.Contains(styled, "UPCOMING") || !strings.Contains(styled, "The budget starts on 01/12/2026.") {
		t.Errorf("styled upcoming details:\n%s", styled)
	}

	// A period that is not the one of today has another heading.
	party := sampleBudgets()[3]
	party.DeletedAt.Valid = true
	last, _ := party.Shown(budgetToday)
	ended := BudgetDetailsView(BudgetDetails{Budget: party, Usage: &last, Today: budgetToday}, true)
	for _, want := range []string{"Status      ended, archived", "Period: 01/10/2026 to 31/10/2026", "Spent       R$ 50,00 (25%)"} {
		if !strings.Contains(ended, want) {
			t.Errorf("ended details lack %q:\n%s", want, ended)
		}
	}
	if strings.Contains(ended, "Current period") {
		t.Errorf("an ended budget has a current period:\n%s", ended)
	}
}

var keySpace = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}

func TestBudgetFormCreateFlow(t *testing.T) {
	var model tea.Model = NewBudgetForm(BudgetFormOptions{
		Taken: free, Initial: budget.Budget{Currency: "BRL"}, Categories: sampleCategories(), Today: budgetToday,
	})
	view := plain(form(t, model).View().Content)
	for _, want := range []string{"New budget", "Title", "Code", "Description", "Currency", "Limit", "Categories", "FOOD", "SAL", "XFER", "all categories", "Period", "month", "Start date", "End date", "Rollover", "no", "yes"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for _, other := range []string{"Opening balance", "Closing day", "Color", "Name", "Target", "Type", "Final date"} {
		if strings.Contains(view, other) {
			t.Errorf("budget form shows %q:\n%s", other, view)
		}
	}
	erase := func(count int) []tea.Msg {
		keys := make([]tea.Msg, count)
		for i := range keys {
			keys[i] = keyBack
		}
		return keys
	}
	model = send(t, model, text("Food")...)
	steps := []struct {
		keys  []tea.Msg
		focus Field
		err   string
	}{
		{[]tea.Msg{keyTab}, FieldCode, ""},
		{[]tea.Msg{keyTab}, FieldDescription, ""},
		{[]tea.Msg{keyTab}, FieldCurrency, ""},
		{[]tea.Msg{keyTab}, FieldLimit, ""},
		{[]tea.Msg{keyTab}, FieldLimit, "Limit is required"},
		{append(text("0"), keyTab), FieldLimit, "Limit must be more than zero"},
		{append([]tea.Msg{keyBack}, append(text("800"), keyTab)...), FieldCategories, ""},
		// FOOD and XFER; SAL is picked, then not.
		{[]tea.Msg{keySpace, keyRight, keySpace, keySpace, keyRight, keySpace, keyRight, keyTab}, FieldEvery, ""},
		{append(erase(5), append(text("fortnight"), keyTab)...), FieldEvery, `Use for example "month", "2 weeks", "15 days" or "none"`},
		{append(erase(9), append(text("2 weeks"), keyTab)...), FieldStart, ""},
		{append(text("soon"), keyTab), FieldStart, "Use DD/MM/YYYY or YYYY/MM/DD"},
		{append(erase(4), keyTab), FieldDate, ""},
		{append(text("01/11/2026"), keyTab), FieldDate, "End date is before the start date"},
		{append(erase(10), keyTab), FieldRollover, ""},
	}
	for index, step := range steps {
		model = send(t, model, step.keys...)
		if f := form(t, model); f.Focus() != step.focus || f.Error() != step.err {
			t.Fatalf("step %d: focus = %v, error %q; want %v, %q", index, f.Focus(), f.Error(), step.focus, step.err)
		}
	}
	if view := plain(form(t, model).View().Content); strings.Contains(view, "all categories") || !strings.Contains(view, "every 2 weeks · R$ 800,00 · FOOD, XFER") {
		t.Errorf("view after the choices:\n%s", view)
	}
	model = send(t, model, keyRight, keyEnter)
	f := form(t, model)
	if !f.Submitted() {
		t.Fatalf("form not submitted: error %q", f.Error())
	}
	got := f.Budget()
	if got.Title != "Food" || got.Code != "FOOD" || got.Currency != "BRL" || got.Limit != 80000 || !reflect.DeepEqual(got.CategoryIDs, []uint{1, 3}) ||
		got.Every != 2 || got.Unit != budget.Week || got.StartDate != budgetToday || got.EndDate != "" || !got.Rollover {
		t.Fatalf("budget = %+v", got)
	}
}

func TestBudgetFormOneTime(t *testing.T) {
	trip := budget.Budget{Title: "Trip", Code: "TRIP", Currency: "USD", StartDate: "2026-12-01", EndDate: "2026-12-31"}
	options := BudgetFormOptions{Taken: free, Initial: trip, Limit: "3000", Categories: sampleCategories(), Today: budgetToday}
	// A budget with an end date and no period does not get a period.
	var model tea.Model = NewBudgetForm(options)
	if f := form(t, model); f.Value(FieldEvery) != "" || f.Value(FieldStart) != "01/12/2026" || f.Value(FieldDate) != "31/12/2026" {
		t.Fatalf("start values = %q, %q, %q", f.Value(FieldEvery), f.Value(FieldStart), f.Value(FieldDate))
	}
	model = send(t, model, keySave)
	f := form(t, model)
	if got := f.Budget(); !f.Submitted() || got.Every != 0 || got.Unit != "" || got.Limit != 300000 || got.StartDate != "2026-12-01" || got.EndDate != "2026-12-31" || got.Rollover || len(got.CategoryIDs) != 0 {
		t.Fatalf("submitted %v, budget = %+v, error %q", f.Submitted(), got, f.Error())
	}
	// Without a period, the end date is necessary.
	options.Initial.EndDate = ""
	options.Preset = []Field{FieldName, FieldCode, FieldDescription, FieldCurrency, FieldLimit, FieldCategories, FieldStart, FieldRollover}
	model = NewBudgetForm(options)
	if f := form(t, model); f.Focus() != FieldEvery || f.Value(FieldEvery) != "month" {
		t.Fatalf("focus = %v, period = %q", f.Focus(), f.Value(FieldEvery))
	}
	model = send(t, model, keyBack, keyBack, keyBack, keyBack, keyBack, keyTab, keyTab)
	if f := form(t, model); f.Focus() != FieldDate || f.Error() != "End date is necessary without a period" {
		t.Fatalf("focus = %v, error %q", f.Focus(), f.Error())
	}
	// A new monthly budget without a start date starts on day 1.
	options.Initial.StartDate = ""
	options.Preset = nil
	model = send(t, NewBudgetForm(options), keySave)
	if got := form(t, model).Budget(); got.StartDate != "2026-11-01" || got.Every != 1 || got.Unit != budget.Month {
		t.Fatalf("budget without a start date = %+v", got)
	}
}

func TestBudgetFormEditMode(t *testing.T) {
	cycle := sampleBudgets()[1]
	f := NewBudgetForm(BudgetFormOptions{Title: "Edit CYCLE", Initial: cycle, Limit: "500,00", Categories: sampleCategories(), Today: budgetToday, Edit: true, Taken: free})
	view := plain(f.View().Content)
	if strings.Contains(view, "Currency") || !strings.Contains(view, "500,00") || !strings.Contains(view, "05/10/2026") ||
		!strings.Contains(view, "every month · R$ 500,00 · all categories") {
		t.Fatalf("edit view:\n%s", view)
	}
	var model tea.Model = f
	model = send(t, model, keySave)
	got := form(t, model).Budget()
	if got.ID != 2 || got.Currency != "BRL" || got.Limit != 50000 || got.Every != 1 || got.Unit != budget.Month || got.StartDate != "2026-10-05" || !got.Rollover || len(got.CategoryIDs) != 0 {
		t.Fatalf("budget = %+v", got)
	}
	// The categories of the budget start picked. A hidden field keeps them.
	food := sampleBudgets()[0]
	for _, preset := range [][]Field{nil, {FieldCategories}} {
		model = send(t, NewBudgetForm(BudgetFormOptions{Initial: food, Limit: "800", Categories: sampleCategories(), Today: budgetToday, Edit: true, Preset: preset, Taken: free}), keySave)
		if got := form(t, model).Budget(); !reflect.DeepEqual(got.CategoryIDs, []uint{1}) || got.StartDate != "2026-09-01" {
			t.Fatalf("preset %v: budget = %+v", preset, got)
		}
	}
}

func TestOtherFormsHaveNoBudgetFields(t *testing.T) {
	for name, view := range map[string]string{
		"account":  plain(NewForm(FormOptions{Taken: free}).View().Content),
		"card":     plain(NewCardForm(CardFormOptions{Taken: free}).View().Content),
		"category": plain(NewCategoryForm(CategoryFormOptions{Taken: free}).View().Content),
		"goal":     plain(NewGoalForm(GoalFormOptions{Taken: free}).View().Content),
	} {
		for _, other := range []string{"Categories", "Period", "Start date", "Rollover"} {
			if strings.Contains(view, other) {
				t.Errorf("%s form shows %q:\n%s", name, other, view)
			}
		}
	}
}
