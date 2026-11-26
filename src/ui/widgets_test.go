package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"finux/src/account"
	"finux/src/category"
	"finux/src/transaction"
)

func selector(t *testing.T, model tea.Model) Selector {
	t.Helper()
	s, ok := model.(Selector)
	if !ok {
		t.Fatalf("model is %T, want Selector", model)
	}
	return s
}

func TestSelectorPicksAccount(t *testing.T) {
	accounts := sampleAccounts()
	var model tea.Model = NewSelector("Accounts", "account", "accounts", AccountEntries(accounts))
	view := plain(selector(t, model).View().Content)
	for _, want := range []string{"Accounts", "Search", "INTER", "Banco Inter", "main account", "R$ 1.234,56", "NUBAN", "-R$ 10,05", "WISE", "$500.00", "3 accounts", "enter", "esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if _, ok := selector(t, model).Selected(); ok {
		t.Fatal("selected before enter")
	}
	model = send(t, model, keyDown, keyEnter)
	if got, ok := selector(t, model).Selected(); !ok || got != 1 {
		t.Fatalf("selected = %+v, %v; want NUBAN", got, ok)
	}
}

func TestSelectorCancel(t *testing.T) {
	for _, key := range []tea.Msg{keyEscape, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}} {
		model := send(t, NewSelector("Accounts", "account", "accounts", AccountEntries(sampleAccounts())), key)
		if _, ok := selector(t, model).Selected(); ok || !selector(t, model).Cancelled() {
			t.Fatalf("key %v did not cancel", key)
		}
	}
}

func TestSelectorSearch(t *testing.T) {
	var model tea.Model = NewSelector("Accounts", "account", "accounts", AccountEntries(sampleAccounts()))
	model = send(t, model, text("WIS")...)
	view := plain(selector(t, model).View().Content)
	if selector(t, model).Query() != "WIS" || !strings.Contains(view, "Wise") || strings.Contains(view, "Nubank") || !strings.Contains(view, "1 of 3 accounts") {
		t.Fatalf("search by code:\n%s", view)
	}
	model = send(t, model, keyEnter)
	if got, ok := selector(t, model).Selected(); !ok || got != 2 {
		t.Fatalf("selected = %+v, %v; want WISE", got, ok)
	}

	model = send(t, NewSelector("Accounts", "account", "accounts", AccountEntries(sampleAccounts())), text("co in")...)
	if view := plain(selector(t, model).View().Content); !strings.Contains(view, "Banco Inter") || strings.Contains(view, "Wise") {
		t.Fatalf("search by name, lowercase:\n%s", view)
	}

	model = send(t, NewSelector("Accounts", "account", "accounts", AccountEntries(sampleAccounts())), text("qjk?/")...)
	s := selector(t, model)
	if s.Cancelled() || s.Query() != "qjk?/" || !strings.Contains(plain(s.View().Content), "No accounts match") {
		t.Fatalf("letters must go to the search: query %q, cancelled %v", s.Query(), s.Cancelled())
	}
	model = send(t, model, keyEnter)
	if _, ok := selector(t, model).Selected(); ok {
		t.Fatal("enter selected an account when no account matches")
	}
	model = send(t, model, keyEscape)
	s = selector(t, model)
	if s.Cancelled() || s.Query() != "" || !strings.Contains(plain(s.View().Content), "Nubank") {
		t.Fatalf("first escape must clear the search: query %q, cancelled %v", s.Query(), s.Cancelled())
	}
	if model = send(t, model, keyEscape); !selector(t, model).Cancelled() {
		t.Fatal("second escape did not cancel")
	}
}

func TestSelectorShowsFiveAccounts(t *testing.T) {
	var accounts []account.Account
	for _, code := range []string{"AA", "BB", "CC", "DD", "EE", "FF", "GG", "HH"} {
		accounts = append(accounts, account.Account{Name: "Bank " + code, Code: code, Color: "blue", Currency: "USD"})
	}
	var model tea.Model = NewSelector("Accounts", "account", "accounts", AccountEntries(accounts))
	view := plain(selector(t, model).View().Content)
	for _, code := range []string{"AA", "BB", "CC", "DD", "EE"} {
		if !strings.Contains(view, "Bank "+code) {
			t.Fatalf("first page lacks %s:\n%s", code, view)
		}
	}
	if strings.Contains(view, "Bank FF") || !strings.Contains(view, "8 accounts") {
		t.Fatalf("first page shows more than 5 accounts or no count:\n%s", view)
	}
	paged := send(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if view := plain(selector(t, paged).View().Content); !strings.Contains(view, "Bank FF") || strings.Contains(view, "Bank AA") {
		t.Fatalf("second page:\n%s", view)
	}
	model = send(t, model, text("hh")...)
	model = send(t, model, keyEnter)
	if got, ok := selector(t, model).Selected(); !ok || got != 7 {
		t.Fatalf("search for an account off the first page = %+v, %v", got, ok)
	}
}

func TestSelectorUsesTheTerminal(t *testing.T) {
	var accounts []account.Account
	for i := range 12 {
		code := string(rune('A'+i)) + "Z"
		accounts = append(accounts, account.Account{Name: "Bank " + code, Code: code, Color: "blue", Currency: "USD"})
	}
	var model tea.Model = NewSelector("Accounts", "account", "accounts", AccountEntries(accounts))
	// A tall terminal shows all the records and no keys for pages.
	tall := plain(selector(t, send(t, model, tea.WindowSizeMsg{Width: 100, Height: 60})).View().Content)
	if got := strings.Count(tall, "Bank "); got != 12 || strings.Contains(tall, "pgup") {
		t.Fatalf("tall terminal shows %d records:\n%s", got, tall)
	}
	// The value of a record ends at the right edge.
	widest := 0
	for _, line := range strings.Split(tall, "\n") {
		widest = max(widest, lipglossWidth(strings.TrimRight(line, " ")))
	}
	if widest != 98 {
		t.Fatalf("wide terminal: the widest line has %d columns, want 98:\n%s", widest, tall)
	}
	// The records fill the height and the screen stays in the terminal.
	for _, size := range []int{10, 13, 22, 30} {
		view := plain(selector(t, send(t, model, tea.WindowSizeMsg{Width: 100, Height: size})).View().Content)
		want := (size - 7) / 3
		if got := strings.Count(view, "Bank "); got != want || strings.Count(view, "\n") >= size || !strings.Contains(view, "pgup") {
			t.Fatalf("height %d: %d records, want %d, in %d lines:\n%s", size, got, want, strings.Count(view, "\n"), view)
		}
	}
}

func TestSelectorOfCategories(t *testing.T) {
	categories := []category.Category{
		{ID: 1, Name: "Food", Code: "FOOD", Color: "red", Description: "Groceries"},
		{ID: 2, Name: "Salary", Code: "SAL", Color: "green"},
	}
	var model tea.Model = NewSelector("Categories", "category", "categories", CategoryEntries(categories))
	view := plain(selector(t, model).View().Content)
	for _, want := range []string{"Categories", "FOOD", "Food", "Groceries", "expense", "SAL", "Salary", "income", "2 categories"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	model = send(t, model, text("zz")...)
	if view := plain(selector(t, model).View().Content); !strings.Contains(view, "No categories match the search") {
		t.Fatalf("empty search:\n%s", view)
	}
	model = send(t, NewSelector("Categories", "category", "categories", CategoryEntries(categories[:1])), keyEnter)
	if got, ok := selector(t, model).Selected(); !ok || got != 0 {
		t.Fatalf("selected = %v, %v; want 0", got, ok)
	}
	if view := plain(NewSelector("Categories", "category", "categories", CategoryEntries(categories[:1])).View().Content); !strings.Contains(view, "1 category") {
		t.Fatalf("singular count:\n%s", view)
	}
}

func confirm(t *testing.T, model tea.Model) Confirm {
	t.Helper()
	c, ok := model.(Confirm)
	if !ok {
		t.Fatalf("model is %T, want Confirm", model)
	}
	return c
}

func TestConfirm(t *testing.T) {
	for name, tc := range map[string]struct {
		keys []tea.Msg
		want bool
	}{
		"enter keeps cancel":    {[]tea.Msg{keyEnter}, false},
		"right then enter":      {[]tea.Msg{keyRight, keyEnter}, true},
		"tab then enter":        {[]tea.Msg{keyTab, keyEnter}, true},
		"right left enter":      {[]tea.Msg{keyRight, keyLeft, keyEnter}, false},
		"y confirms":            {text("y"), true},
		"n cancels":             {text("n"), false},
		"escape cancels":        {[]tea.Msg{keyRight, keyEscape}, false},
		"no key is not a yes":   {nil, false},
		"up and down are inert": {[]tea.Msg{keyUp, keyDown, keyEnter}, false},
	} {
		var model tea.Model = NewConfirm("Archive account INTER?", "Archive", false)
		if view := plain(confirm(t, model).View().Content); !strings.Contains(view, "Archive account INTER?") || !strings.Contains(view, "Cancel") {
			t.Fatalf("view = %q", view)
		}
		model = send(t, model, tc.keys...)
		if got := confirm(t, model).Confirmed(); got != tc.want {
			t.Errorf("%s: confirmed = %v, want %v", name, got, tc.want)
		}
	}
}

func TestAccountTable(t *testing.T) {
	accounts := sampleAccounts()
	accounts[1].DeletedAt.Valid = true
	styled := AccountTable(accounts, false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") {
		t.Fatalf("styled table has no color or border:\n%s", styled)
	}
	for _, table := range []string{plain(styled), AccountTable(accounts, true)} {
		for _, want := range []string{"CODE", "NAME", "CURRENCY", "BALANCE", "INTER", "Banco Inter", "R$ 1.234,56", "Nubank (archived)", "-R$ 10,05", "$500.00", "3 accounts"} {
			if !strings.Contains(table, want) {
				t.Errorf("table lacks %q:\n%s", want, table)
			}
		}
	}
	bare := AccountTable(accounts, true)
	if !strings.HasPrefix(bare, "CODE ") || strings.Contains(bare, "ID") {
		t.Fatalf("table shows the id column:\n%s", bare)
	}
	if strings.ContainsAny(bare, "╭│─\x1b") {
		t.Fatalf("plain table has borders or colors:\n%s", bare)
	}
	if got := AccountTable(accounts[:1], true); !strings.Contains(got, "1 account\n") {
		t.Fatalf("singular count missing:\n%s", got)
	}
	if got := AccountTable(nil, true); got != "No accounts yet. Run 'finux ac add'.\n" {
		t.Fatalf("empty table = %q", got)
	}
}

func TestAccountCardPlain(t *testing.T) {
	account := sampleAccounts()[0]
	bare := AccountCard(account, true)
	if strings.ContainsAny(bare, "╭│─\x1b") {
		t.Fatalf("plain card has borders or colors:\n%s", bare)
	}
	for _, want := range []string{"INTER  Banco Inter\n", "main account\n", "ID        1\n", "Currency  BRL\n", "Balance   R$ 1.234,56\n", "Color     orange\n", "Created   ", "Updated   "} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain card lacks %q:\n%s", want, bare)
		}
	}
	if strings.Contains(bare, "archived") {
		t.Fatal("active account shown as archived")
	}
	account.DeletedAt.Valid = true
	if !strings.Contains(AccountCard(account, true), "Status    archived\n") {
		t.Fatal("archived account not marked")
	}
}

func TestAccountCardStyled(t *testing.T) {
	account := sampleAccounts()[0]
	styled := AccountCard(account, false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") {
		t.Fatalf("styled card has no color or border:\n%s", styled)
	}
	card := plain(styled)
	// The order is the hierarchy: who, how much, then the small details.
	order := []string{"INTER", "Banco Inter", "#1", "main account", "BALANCE", "R$ 1.234,56", "BRL", "────", "Created", "Updated"}
	position := 0
	for _, want := range order {
		index := strings.Index(card[position:], want)
		if index < 0 {
			t.Fatalf("styled card lacks %q after %q, or it is out of order:\n%s", want, card[:position], card)
		}
		position += index + len(want)
	}
	lines := strings.Split(strings.TrimRight(card, "\n"), "\n")
	for _, line := range lines {
		if lipglossWidth(line) != lipglossWidth(lines[0]) {
			t.Fatalf("card lines have different widths:\n%s", card)
		}
	}
	for _, unwanted := range []string{"ID", "Color", "orange", "ARCHIVED"} {
		if strings.Contains(card, unwanted) {
			t.Fatalf("styled card shows %q:\n%s", unwanted, card)
		}
	}
	account.DeletedAt.Valid = true
	account.Description = ""
	account.OpeningBalance = -5
	if archived := plain(AccountCard(account, false)); !strings.Contains(archived, "ARCHIVED") || !strings.Contains(archived, "-R$ 0,05") {
		t.Fatalf("archived card:\n%s", archived)
	}
	account.Description = strings.Repeat("long description ", 12)
	for _, line := range strings.Split(plain(AccountCard(account, false)), "\n") {
		if lipglossWidth(line) > 70 {
			t.Fatalf("long description makes the card too wide: %q", line)
		}
	}
}

func lipglossWidth(text string) int { return lipgloss.Width(text) }

func sampleCategories() []category.Category {
	return []category.Category{
		{ID: 1, Name: "Food", Code: "FOOD", Color: "red", Description: "Groceries and restaurants"},
		{ID: 2, Name: "Salary", Code: "SAL", Color: "green"},
		{ID: 3, Name: "Transfer", Code: "XFER", Color: "grey"},
	}
}

func TestCategoryTable(t *testing.T) {
	categories := sampleCategories()
	styled := CategoryTable(categories, false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") {
		t.Fatalf("styled table has no color or border:\n%s", styled)
	}
	for _, want := range []string{"CODE", "NAME", "TYPE", "FOOD", "Food", "expense", "SAL", "income", "XFER", "neutral", "3 categories"} {
		if !strings.Contains(plain(styled), want) {
			t.Errorf("styled table lacks %q:\n%s", want, plain(styled))
		}
	}
	const want = "" +
		"CODE  NAME      TYPE\n" +
		"FOOD  Food      expense\n" +
		"SAL   Salary    income\n" +
		"XFER  Transfer  neutral\n" +
		"3 categories\n"
	if got := CategoryTable(categories, true); got != want {
		t.Fatalf("plain table =\n%s\nwant\n%s", got, want)
	}
	if got := CategoryTable(categories[:1], true); !strings.HasSuffix(got, "\n1 category\n") {
		t.Fatalf("singular count missing:\n%s", got)
	}
	if got := CategoryTable(nil, true); got != "No categories yet. Run 'finux ct add'.\n" {
		t.Fatalf("empty table = %q", got)
	}
}

func TestCategoryCard(t *testing.T) {
	food := sampleCategories()[0]
	bare := CategoryCard(food, true)
	if strings.ContainsAny(bare, "╭│─\x1b") {
		t.Fatalf("plain card has borders or colors:\n%s", bare)
	}
	for _, want := range []string{"FOOD  Food\n", "Groceries and restaurants\n", "ID        1\n", "Type      expense\n", "Color     red\n", "Created   ", "Updated   "} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain card lacks %q:\n%s", want, bare)
		}
	}
	styled := CategoryCard(food, false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") {
		t.Fatalf("styled card has no color or border:\n%s", styled)
	}
	card := plain(styled)
	position := 0
	for _, want := range []string{"FOOD", "Food", "#1", "Groceries and restaurants", "expense", "Created", "Updated"} {
		index := strings.Index(card[position:], want)
		if index < 0 {
			t.Fatalf("styled card lacks %q or it is out of order:\n%s", want, card)
		}
		position += index + len(want)
	}
	lines := strings.Split(strings.TrimRight(card, "\n"), "\n")
	for _, line := range lines {
		if lipglossWidth(line) != lipglossWidth(lines[0]) {
			t.Fatalf("card lines have different widths:\n%s", card)
		}
	}
}

func sampleTransactions() []transaction.View {
	accounts := sampleAccounts()
	categories := sampleCategories()
	return []transaction.View{
		{
			Transaction: transaction.Transaction{ID: 12, Title: "Weekly market", Description: "fruit and vegetables", Amount: -5025, Type: transaction.Expense, Date: "2026-11-17", Tags: "#market #weekly"},
			Account:     accounts[0], Category: categories[0],
		},
		{
			Transaction: transaction.Transaction{ID: 11, Title: "Salary", Amount: 300000, Type: transaction.Income, Date: "2026-11-05"},
			Account:     accounts[0], Category: categories[1],
		},
		{
			Transaction: transaction.Transaction{ID: 9, Title: "Travel money", Amount: -55000, Type: transaction.Transfer, Date: "2026-10-30", PairID: 10},
			Account:     accounts[0], Category: categories[2], Other: &accounts[2],
		},
		{
			Transaction: transaction.Transaction{ID: 10, Title: "Travel money", Amount: 10000, Type: transaction.Transfer, Date: "2026-10-30", PairID: 9},
			Account:     accounts[2], Category: categories[2], Other: &accounts[0],
		},
	}
}

func TestTransactionTable(t *testing.T) {
	views := sampleTransactions()
	styled := TransactionTable(views, false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") {
		t.Fatalf("styled table has no color or border:\n%s", styled)
	}
	const want = "" +
		"ID  DATE        TITLE          CATEGORY  ACCOUNT        VALUE\n" +
		"12  17/11/2026  Weekly market  FOOD      INTER      -R$ 50,25\n" +
		"11  05/11/2026  Salary         SAL       INTER    R$ 3.000,00\n" +
		"9   30/10/2026  Travel money   XFER      INTER     -R$ 550,00\n" +
		"10  30/10/2026  Travel money   XFER      WISE         $100.00\n" +
		"4 transactions\n"
	if got := TransactionTable(views, true); got != want {
		t.Fatalf("plain table =\n%s\nwant\n%s", got, want)
	}
	for _, text := range []string{"Weekly market", "FOOD", "INTER", "-R$ 50,25", "17/11/2026", "4 transactions"} {
		if !strings.Contains(plain(styled), text) {
			t.Errorf("styled table lacks %q:\n%s", text, plain(styled))
		}
	}
	if got := TransactionTable(views[:1], true); !strings.HasSuffix(got, "\n1 transaction\n") {
		t.Fatalf("singular count missing:\n%s", got)
	}
	if got := TransactionTable(nil, true); got != "No transactions yet. Run 'finux tx add'.\n" {
		t.Fatalf("empty table = %q", got)
	}
}

func TestTransactionCard(t *testing.T) {
	views := sampleTransactions()
	bare := TransactionCard(views[0], true)
	if strings.ContainsAny(bare, "╭│─\x1b") {
		t.Fatalf("plain card has borders or colors:\n%s", bare)
	}
	for _, want := range []string{"Weekly market\n", "fruit and vegetables\n", "ID        12\n", "Type      expense\n", "Value     -R$ 50,25\n", "Account   INTER  Banco Inter\n", "Category  FOOD  Food\n", "Date      17/11/2026\n", "Tags      #market #weekly\n", "Created   ", "Updated   "} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain card lacks %q:\n%s", want, bare)
		}
	}
	if strings.Contains(bare, "Transfer") {
		t.Fatalf("expense card has a transfer line:\n%s", bare)
	}
	if got := TransactionCard(views[1], true); strings.Contains(got, "Tags") || !strings.Contains(got, "Value     R$ 3.000,00\n") {
		t.Fatalf("income without tags:\n%s", got)
	}
	if got := TransactionCard(views[2], true); !strings.Contains(got, "Transfer  to WISE  Wise (transaction 10)\n") {
		t.Fatalf("outgoing side:\n%s", got)
	}
	if got := TransactionCard(views[3], true); !strings.Contains(got, "Transfer  from INTER  Banco Inter (transaction 9)\n") || !strings.Contains(got, "Value     $100.00\n") {
		t.Fatalf("incoming side:\n%s", got)
	}

	if strings.Contains(bare, "Due ") || strings.Contains(bare, "Timing") {
		t.Fatalf("card without a due date has one:\n%s", bare)
	}
	planned := views[0]
	planned.Date, planned.DueDate, planned.Timing = "2026-11-25", "2026-10-06", transaction.TimingLate
	for _, card := range []string{TransactionCard(planned, true), plain(TransactionCard(planned, false))} {
		if !strings.Contains(card, "06/10/2026") || !strings.Contains(card, "50 days late") {
			t.Fatalf("card of a late transaction:\n%s", card)
		}
	}

	styled := TransactionCard(views[0], false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") {
		t.Fatalf("styled card has no color or border:\n%s", styled)
	}
	card := plain(styled)
	// The order is the hierarchy: what, how much, where, then the small details.
	position := 0
	for _, want := range []string{"EXPENSE", "Weekly market", "#12", "fruit and vegetables", "-R$ 50,25", "BRL", "Account", "INTER", "Category", "FOOD", "Date", "17/11/2026", "Tags", "#market #weekly", "────", "Created", "Updated"} {
		index := strings.Index(card[position:], want)
		if index < 0 {
			t.Fatalf("styled card lacks %q or it is out of order:\n%s", want, card)
		}
		position += index + len(want)
	}
	lines := strings.Split(strings.TrimRight(card, "\n"), "\n")
	for _, line := range lines {
		if lipglossWidth(line) != lipglossWidth(lines[0]) {
			t.Fatalf("card lines have different widths:\n%s", card)
		}
	}
	// An income card is green and an expense card is red: the border,
	// the badge, the amount and the tags. The colors are fixed, because
	// the colors of a terminal theme can make red and green look the same.
	const green, red = "34;197;94", "239;68;68"
	income := TransactionCard(views[1], false)
	for _, want := range []string{"\x1b[38;2;" + green + "m╭", "48;2;" + green + "m", "38;2;" + green + "mR$ 3.000,00"} {
		if !strings.Contains(income, want) {
			t.Errorf("income card lacks %q:\n%q", want, income)
		}
	}
	for _, want := range []string{"\x1b[38;2;" + red + "m╭", "48;2;" + red + "m", "38;2;" + red + "m-R$ 50,25", "\x1b[38;2;" + red + "m#market #weekly"} {
		if !strings.Contains(styled, want) {
			t.Errorf("expense card lacks %q:\n%q", want, styled)
		}
	}
	if strings.Contains(income, red) || strings.Contains(styled, green) {
		t.Fatalf("a card has the color of the other type:\n%q\n%q", income, styled)
	}
	if neutral := TransactionCard(views[2], false); strings.Contains(neutral, red+"m╭") || strings.Contains(neutral, green+"m╭") {
		t.Fatalf("transfer card has the border of an income or an expense:\n%q", neutral)
	}
	if transfer := plain(TransactionCard(views[2], false)); !strings.Contains(transfer, "TRANSFER") || !strings.Contains(transfer, "To") || !strings.Contains(transfer, "WISE") {
		t.Fatalf("styled transfer card:\n%s", transfer)
	}
	long := views[0]
	long.Title, long.Description = strings.Repeat("t", 60), strings.Repeat("long description ", 12)
	for _, line := range strings.Split(plain(TransactionCard(long, false)), "\n") {
		if lipglossWidth(line) > 74 {
			t.Fatalf("long text makes the card too wide: %q", line)
		}
	}
}
