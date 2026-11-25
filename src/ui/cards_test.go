package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/account"
	"finux/src/card"
	"finux/src/transaction"
)

func sampleCards() []card.Card {
	return []card.Card{
		{ID: 1, Name: "Visa Gold", Description: "main card", Code: "VISA", Color: "blue", Currency: "BRL", Limit: 500000, ClosingDay: 25, DueDay: 5, Used: 125000},
		{ID: 2, Name: "Amex", Code: "AMEX", Color: "teal", Currency: "USD", Limit: 200000, ClosingDay: 3, DueDay: 10},
	}
}

func sampleBills() []card.Bill {
	return []card.Bill{
		{ID: 3, CardID: 1, Period: "2026-12", ClosesOn: "2026-11-25", DueOn: "2026-12-05", Carried: 4000, Spent: 7500},
		{ID: 2, CardID: 1, Period: "2026-11", ClosesOn: "2026-10-25", DueOn: "2026-11-05", Spent: 85000},
		{ID: 1, CardID: 1, Period: "2026-10", ClosesOn: "2026-09-25", DueOn: "2026-10-05", Spent: 10000, PaidOn: "2026-10-03", Paid: 6000, Remaining: 4000, PaymentID: 7},
	}
}

func samplePurchases() []transaction.View {
	visa := sampleCards()[0]
	shown := account.Account{Name: visa.Name, Code: visa.Code, Color: visa.Color, Currency: visa.Currency}
	categories := sampleCategories()
	bill := sampleBills()[0]
	return []transaction.View{
		{
			Transaction: transaction.Transaction{ID: 21, Title: "Refund", Amount: 2500, Type: transaction.Income, Date: "2026-11-20", CardID: 1, BillID: 3},
			Account:     shown, Category: categories[1], Card: &visa, Bill: &bill,
		},
		{
			Transaction: transaction.Transaction{ID: 20, Title: "Market", Amount: -10000, Type: transaction.Expense, Date: "2026-11-17", Tags: "#market", CardID: 1, BillID: 3},
			Account:     shown, Category: categories[0], Card: &visa, Bill: &bill,
		},
	}
}

func TestCardEntries(t *testing.T) {
	cards := sampleCards()
	cards[1].DeletedAt.Valid = true
	entries := CardEntries(cards)
	if len(entries) != 2 || entries[0].Code != "VISA" || entries[0].Name != "Visa Gold" || entries[0].Value != "R$ 3.750,00" || entries[0].Color != "#3B82F6" {
		t.Fatalf("entries = %+v", entries)
	}
	if !strings.Contains(entries[0].Description, "main card") || !strings.Contains(entries[1].Description, "archived") {
		t.Fatalf("descriptions = %q, %q", entries[0].Description, entries[1].Description)
	}
}

func TestCardTable(t *testing.T) {
	if got := CardTable(nil, true); got != "No credit cards yet. Run 'finux cc add'.\n" {
		t.Fatalf("empty table = %q", got)
	}
	cards := sampleCards()
	cards[1].DeletedAt.Valid = true
	bare := CardTable(cards, true)
	if strings.ContainsAny(bare, "╭│\x1b") {
		t.Fatalf("plain table has borders or colors:\n%s", bare)
	}
	lines := strings.Split(bare, "\n")
	if got := strings.Fields(lines[0]); strings.Join(got, " ") != "CODE NAME CLOSES DUE LIMIT USED AVAILABLE" {
		t.Fatalf("headers = %v", got)
	}
	for _, want := range []string{"VISA", "Visa Gold", "25", "R$ 5.000,00", "R$ 1.250,00", "R$ 3.750,00", "Amex (archived)", "$2,000.00", "2 credit cards\n"} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain table lacks %q:\n%s", want, bare)
		}
	}
	if !strings.HasSuffix(lines[1], "R$ 3.750,00") {
		t.Errorf("available is not at the right edge: %q", lines[1])
	}
	styled := CardTable(cards[:1], false)
	if !strings.Contains(styled, "╭") || !strings.Contains(plain(styled), "1 credit card") {
		t.Fatalf("styled table:\n%s", styled)
	}
}

func TestBillTable(t *testing.T) {
	visa := sampleCards()[0]
	if got := BillTable(visa, nil, "2026-11-17", true); got != "No bills yet for VISA.\n" {
		t.Fatalf("empty table = %q", got)
	}
	bare := BillTable(visa, sampleBills(), "2026-11-17", true)
	lines := strings.Split(strings.TrimRight(bare, "\n"), "\n")
	if strings.Join(strings.Fields(lines[0]), " ") != "BILL CLOSES DUE STATUS PAID TOTAL" {
		t.Fatalf("headers = %q", lines[0])
	}
	for index, want := range [][]string{
		{"12/2026", "25/11/2026", "05/12/2026", "open", "R$ 115,00"},
		{"11/2026", "25/10/2026", "05/11/2026", "late", "R$ 850,00"},
		{"10/2026", "25/09/2026", "05/10/2026", "paid", "R$ 60,00", "R$ 100,00"},
	} {
		for _, cell := range want {
			if !strings.Contains(lines[index+1], cell) {
				t.Errorf("line %d lacks %q: %q", index+1, cell, lines[index+1])
			}
		}
	}
	if lines[len(lines)-1] != "3 bills" {
		t.Errorf("count line = %q", lines[len(lines)-1])
	}
	styled := BillTable(visa, sampleBills(), "2026-11-17", false)
	if !strings.Contains(styled, "╭") || !strings.Contains(styled, "\x1b[") {
		t.Fatalf("styled table:\n%s", styled)
	}
}

func sampleDetails() CardDetails {
	bills := sampleBills()
	return CardDetails{Card: sampleCards()[0], Bill: bills[0], Transactions: samplePurchases(), Pending: bills[1:2], Today: "2026-11-17"}
}

func TestCardDetailsPlain(t *testing.T) {
	bare := CardDetailsView(sampleDetails(), true)
	if strings.ContainsAny(bare, "╭│\x1b") {
		t.Fatalf("plain details have borders or colors:\n%s", bare)
	}
	order := []string{
		"VISA  Visa Gold\n", "main card\n", "ID         1\n", "Currency   BRL\n", "Limit      R$ 5.000,00\n", "Used       R$ 1.250,00\n",
		"Available  R$ 3.750,00\n", "Closing    day 25\n", "Due        day 5\n", "Color      blue\n", "Created    ", "Updated    ",
		"Bill 11/2026 is late: R$ 850,00, due 05/11/2026\n",
		"Open bill 12/2026: closes 25/11/2026, due 05/12/2026\n",
		"ID", "DATE", "TITLE", "CATEGORY", "VALUE",
		"21", "20/11/2026", "Refund", "R$ 25,00",
		"20", "17/11/2026", "Market", "-R$ 100,00",
		"Carried from the bill before  R$ 40,00\n",
		"Total  R$ 115,00\n",
	}
	position := 0
	for _, want := range order {
		index := strings.Index(bare[position:], want)
		if index < 0 {
			t.Fatalf("plain details lack %q after position %d, or it is out of order:\n%s", want, position, bare)
		}
		position += index + len(want)
	}

	empty := sampleDetails()
	empty.Transactions, empty.Pending, empty.Bill.Carried, empty.Bill.Spent = nil, nil, 0, 0
	empty.Card.DeletedAt.Valid = true
	bare = CardDetailsView(empty, true)
	for _, want := range []string{"Status     archived\n", "No transactions in this bill.\n"} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain details lack %q:\n%s", want, bare)
		}
	}
	for _, unwanted := range []string{"is late", "Carried", "Total"} {
		if strings.Contains(bare, unwanted) {
			t.Errorf("plain details show %q:\n%s", unwanted, bare)
		}
	}
}

func TestCardDetailsStyled(t *testing.T) {
	details := sampleDetails()
	closed := sampleBills()[1]
	closed.Period, closed.ClosesOn, closed.DueOn = "2026-12", "2026-11-10", "2026-11-20"
	details.Pending = append(details.Pending, closed)
	styled := CardDetailsView(details, false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") {
		t.Fatalf("styled details have no color or border:\n%s", styled)
	}
	text := plain(styled)
	order := []string{
		"VISA", "Visa Gold", "#1", "main card", "AVAILABLE", "R$ 3.750,00", "BRL", "█", "░", "R$ 1.250,00 used", "of R$ 5.000,00",
		"Closes day 25", "Due day 5", "Created",
		"Bill 11/2026 is late", "R$ 850,00", "due 05/11/2026",
		"Bill 12/2026 is closed", "due 20/11/2026",
		"OPEN BILL 12/2026", "closes 25/11/2026", "due 05/12/2026",
		"20/11", "▲", "Refund", "R$ 25,00",
		"17/11", "▼", "Market", "#market", "-R$ 100,00",
		"Carried from the bill before", "R$ 40,00",
		"Total", "R$ 115,00",
	}
	position := 0
	for _, want := range order {
		index := strings.Index(text[position:], want)
		if index < 0 {
			t.Fatalf("styled details lack %q after %q, or it is out of order:\n%s", want, order[0], text)
		}
		position += index + len(want)
	}
	// The late warning is red, the arrows have the fixed colors.
	for _, want := range []string{"38;2;239;68;68", "38;2;34;197;94"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled details lack the color %s", want)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if lipglossWidth(line) > 80 {
			t.Fatalf("line is too wide: %q", line)
		}
	}

	details.Card.Used, details.Card.DeletedAt.Valid = 600000, true
	over := plain(CardDetailsView(details, false))
	if !strings.Contains(over, "ARCHIVED") || !strings.Contains(over, "-R$ 1.000,00") {
		t.Fatalf("card over the limit:\n%s", over)
	}
}

func TestTransactionCardOfACard(t *testing.T) {
	purchase := samplePurchases()[1]
	bare := TransactionCard(purchase, true)
	for _, want := range []string{"Card      VISA  Visa Gold\n", "Bill      12/2026\n"} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain card lacks %q:\n%s", want, bare)
		}
	}
	if strings.Contains(bare, "Account ") {
		t.Errorf("plain card of a purchase shows an account:\n%s", bare)
	}
	styled := plain(TransactionCard(purchase, false))
	for _, want := range []string{"Card", "VISA", "Bill", "12/2026"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled card lacks %q:\n%s", want, styled)
		}
	}

	visa := sampleCards()[0]
	bill := sampleBills()[2]
	shown := account.Account{Name: visa.Name, Code: visa.Code, Color: visa.Color, Currency: visa.Currency}
	payment := transaction.View{
		Transaction: transaction.Transaction{ID: 7, Title: "Payment VISA 10/2026", Amount: -6000, Type: transaction.Payment, Date: "2026-10-03", BillID: 1},
		Account:     sampleAccounts()[0], Category: sampleCategories()[2], Other: &shown, Card: &visa, Bill: &bill,
	}
	bare = TransactionCard(payment, true)
	for _, want := range []string{"Type      payment\n", "Account   INTER  Banco Inter\n", "Payment   bill 10/2026 of VISA  Visa Gold\n"} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain payment lacks %q:\n%s", want, bare)
		}
	}
	if strings.Contains(bare, "Transfer  ") || strings.Contains(bare, "transaction 0") {
		t.Errorf("plain payment looks like a transfer:\n%s", bare)
	}
	styled = plain(TransactionCard(payment, false))
	for _, want := range []string{"PAYMENT", "To", "VISA", "Bill", "10/2026"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled payment lacks %q:\n%s", want, styled)
		}
	}
	if strings.Contains(styled, "#0") {
		t.Errorf("styled payment shows a pair id:\n%s", styled)
	}
}

func TestTransactionFormWithCards(t *testing.T) {
	options := txOptions()
	options.Cards = sampleCards()
	options.Preset = []TxField{TxTitle}
	options.Initial.Title = "Market"
	var model tea.Model = NewTransactionForm(options)
	model = step(t, model, TxValue, keyEnter)
	model = step(t, model, TxInstallments, append(text("100"), keyEnter)...)
	model = step(t, model, TxAccount, keyEnter)
	// The cards come after the accounts: 3 accounts, then VISA and AMEX.
	model = send(t, model, keyRight, keyRight, keyRight)
	f := txForm(t, model)
	if f.Value(TxAccount) != "VISA" {
		t.Fatalf("account = %q, want VISA", f.Value(TxAccount))
	}
	view := plain(f.View().Content)
	for _, want := range []string{"VISA", "Visa Gold · BRL · credit card", "4/5"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	draft := f.Draft()
	if draft.CardID != 1 || draft.AccountID != 0 || draft.Value != 10000 || draft.Type != transaction.Expense {
		t.Fatalf("draft = %+v", draft)
	}

	// A transfer cannot use a card: the form goes back to an account,
	// and no card is in the two account lists.
	model = send(t, model, keyShiftTab, keyShiftTab, keyShiftTab)
	model = step(t, model, TxType)
	model = send(t, model, keyRight, keyRight)
	f = txForm(t, model)
	if f.Value(TxType) != transaction.Transfer || f.Value(TxAccount) != "INTER" {
		t.Fatalf("type %q, account %q", f.Value(TxType), f.Value(TxAccount))
	}
	view = plain(f.View().Content)
	if !strings.Contains(view, "1/3") || !strings.Contains(view, "1/2") || strings.Contains(view, "credit card") {
		t.Fatalf("transfer form shows a card:\n%s", view)
	}
	if draft := f.Draft(); draft.CardID != 0 || draft.AccountID != 1 || draft.ToAccountID != 2 {
		t.Fatalf("transfer draft = %+v", draft)
	}

	// Edit of a card transaction starts on its card.
	options = txOptions()
	options.Cards = sampleCards()
	options.Edit = true
	options.Initial = transaction.Draft{Title: "Market", Type: transaction.Expense, CardID: 2, CategoryID: 2, Date: "2026-11-17"}
	options.Value = "10.00"
	f = NewTransactionForm(options)
	if f.Value(TxAccount) != "AMEX" {
		t.Fatalf("edit form account = %q, want AMEX", f.Value(TxAccount))
	}
	if draft := f.Draft(); draft.CardID != 2 || draft.AccountID != 0 || draft.Value != 1000 {
		t.Fatalf("edit draft = %+v", draft)
	}

	// Only cards, no account: the form works without the transfer type.
	options = txOptions()
	options.Accounts, options.Cards = nil, sampleCards()
	f = NewTransactionForm(options)
	if f.Value(TxAccount) != "VISA" || strings.Contains(plain(f.View().Content), "transfer") {
		t.Fatalf("form with only cards: account %q\n%s", f.Value(TxAccount), plain(f.View().Content))
	}
}

func TestCardFormCreateFlow(t *testing.T) {
	var model tea.Model = NewCardForm(CardFormOptions{Taken: free})
	view := plain(form(t, model).View().Content)
	for _, want := range []string{"New credit card", "Name", "Code", "Description", "Currency", "Limit", "Closing day", "Due day", "Color"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Opening balance") {
		t.Errorf("card form shows the opening balance:\n%s", view)
	}
	model = send(t, model, text("Visa Gold")...)
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
		{append(text("5000"), keyTab), FieldClosing, ""},
		{append(text("40"), keyTab), FieldClosing, "Closing day must be from 1 to 31"},
		{[]tea.Msg{keyBack, keyBack, tea.KeyPressMsg{Code: '2', Text: "2"}, tea.KeyPressMsg{Code: '5', Text: "5"}, keyTab}, FieldDue, ""},
		{append(text("x"), keyTab), FieldDue, "Due day must be from 1 to 31"},
		{[]tea.Msg{keyBack, tea.KeyPressMsg{Code: '5', Text: "5"}, keyTab}, FieldOwner, ""},
		{[]tea.Msg{keyTab}, FieldColor, ""},
	}
	for index, step := range steps {
		model = send(t, model, step.keys...)
		if f := form(t, model); f.Focus() != step.focus || f.Error() != step.err {
			t.Fatalf("step %d: focus = %v, error %q; want %v, %q", index, f.Focus(), f.Error(), step.focus, step.err)
		}
	}
	model = send(t, model, keyEnter)
	f := form(t, model)
	if !f.Submitted() {
		t.Fatalf("form not submitted: error %q", f.Error())
	}
	got := f.Card()
	if got.Name != "Visa Gold" || got.Code != "VGOLD" || got.Currency != "USD" || got.Limit != 500000 || got.ClosingDay != 25 || got.DueDay != 5 || got.Color == "" {
		t.Fatalf("card = %+v", got)
	}
}

func TestCardFormEditMode(t *testing.T) {
	visa := sampleCards()[0]
	f := NewCardForm(CardFormOptions{Title: "Edit VISA", Initial: visa, Limit: "5000,00", Edit: true, Taken: free})
	view := plain(f.View().Content)
	if strings.Contains(view, "Currency") || !strings.Contains(view, "5000,00") || !strings.Contains(view, "Preview") ||
		!strings.Contains(view, "R$ 5.000,00 · closes 25 · due 5") {
		t.Fatalf("edit view:\n%s", view)
	}
	if f.Value(FieldClosing) != "25" || f.Value(FieldDue) != "5" || f.Value(FieldColor) != "blue" {
		t.Fatalf("values: closing %q, due %q, color %q", f.Value(FieldClosing), f.Value(FieldDue), f.Value(FieldColor))
	}
	var model tea.Model = f
	model = send(t, model, keySave)
	got := form(t, model).Card()
	if got.ID != 1 || got.Currency != "BRL" || got.Limit != 500000 || got.Used != visa.Used {
		t.Fatalf("card = %+v", got)
	}
}
