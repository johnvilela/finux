package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"finux/src/account"
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
	var model tea.Model = NewSelector("Accounts", accounts)
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
	if got, ok := selector(t, model).Selected(); !ok || got.Code != "NUBAN" {
		t.Fatalf("selected = %+v, %v; want NUBAN", got, ok)
	}
}

func TestSelectorCancel(t *testing.T) {
	for _, key := range []tea.Msg{keyEscape, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}} {
		model := send(t, NewSelector("Accounts", sampleAccounts()), key)
		if _, ok := selector(t, model).Selected(); ok || !selector(t, model).Cancelled() {
			t.Fatalf("key %v did not cancel", key)
		}
	}
}

func TestSelectorSearch(t *testing.T) {
	var model tea.Model = NewSelector("Accounts", sampleAccounts())
	model = send(t, model, text("WIS")...)
	view := plain(selector(t, model).View().Content)
	if selector(t, model).Query() != "WIS" || !strings.Contains(view, "Wise") || strings.Contains(view, "Nubank") || !strings.Contains(view, "1 of 3 accounts") {
		t.Fatalf("search by code:\n%s", view)
	}
	model = send(t, model, keyEnter)
	if got, ok := selector(t, model).Selected(); !ok || got.Code != "WISE" {
		t.Fatalf("selected = %+v, %v; want WISE", got, ok)
	}

	model = send(t, NewSelector("Accounts", sampleAccounts()), text("co in")...)
	if view := plain(selector(t, model).View().Content); !strings.Contains(view, "Banco Inter") || strings.Contains(view, "Wise") {
		t.Fatalf("search by name, lowercase:\n%s", view)
	}

	model = send(t, NewSelector("Accounts", sampleAccounts()), text("qjk?/")...)
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
	var model tea.Model = NewSelector("Accounts", accounts)
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
	if got, ok := selector(t, model).Selected(); !ok || got.Code != "HH" {
		t.Fatalf("search for an account off the first page = %+v, %v", got, ok)
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
