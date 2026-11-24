package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/transaction"
)

func ledger(t *testing.T, model tea.Model) Ledger {
	t.Helper()
	l, ok := model.(Ledger)
	if !ok {
		t.Fatalf("model is %T, want Ledger", model)
	}
	return l
}

func ledgerView(t *testing.T, model tea.Model) string {
	t.Helper()
	return plain(ledger(t, model).View().Content)
}

func TestLedgerView(t *testing.T) {
	var model tea.Model = NewLedger("Transactions", sampleTransactions())
	view := ledgerView(t, model)
	for _, want := range []string{
		"Transactions", "title, #tag, @ACCOUNT, cat:CODE, 2026-11",
		// One heading for each day, then its transactions in one line each.
		"17/11/2026  Tuesday", "05/11/2026  Thursday", "30/10/2026  Friday",
		"Weekly market", "#market #weekly", "FOOD", "INTER", "-R$ 50,25",
		"Salary", "SAL", "R$ 3.000,00",
		"Travel money", "XFER", "INTER → WISE", "-R$ 550,00", "WISE ← INTER", "$100.00",
		// Totals of the transactions in the list; transfers are not in or out.
		"BRL", "in R$ 3.000,00", "out R$ 50,25", "net R$ 2.949,75",
		"4 transactions", "enter", "esc",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	// A transaction is further in than the heading of its day, and an
	// empty line separates the days.
	for _, want := range []string{"\n  17/11/2026", "\n▌   ▼ Weekly market", "\n\n  05/11/2026", "\n    ▲ Salary", "\n\n  30/10/2026", "\n    ↔ Travel money"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "\n\n\n  17/11/2026") {
		t.Errorf("empty line before the first day:\n%s", view)
	}
	if strings.Count(view, "30/10/2026") != 1 {
		t.Errorf("two transactions of one day need one heading:\n%s", view)
	}
	// WISE has only a side of a transfer: no money came in or went out.
	if strings.Contains(view, "USD") {
		t.Errorf("totals show a currency without incomes or expenses:\n%s", view)
	}
	// The order is the hierarchy: search, totals, days, then the help.
	position := 0
	for _, want := range []string{"Search", "net", "17/11/2026", "Weekly market", "05/11/2026", "Salary", "30/10/2026", "4 transactions"} {
		index := strings.Index(view[position:], want)
		if index < 0 {
			t.Fatalf("view lacks %q or it is out of order:\n%s", want, view)
		}
		position += index + len(want)
	}
	// The signs have fixed colors, the same as the category dots: the
	// colors of a terminal theme can make red and green look the same.
	raw := ledger(t, model).View().Content
	for _, want := range []string{paint("#22C55E", "▲"), paint("#EF4444", "▼")} {
		if !strings.Contains(raw, want) {
			t.Errorf("view lacks the colored sign %q", want)
		}
	}
}

func TestLedgerSearch(t *testing.T) {
	var model tea.Model = NewLedger("Transactions", sampleTransactions())
	for query, want := range map[string]string{
		"sal":           "1 of 4 transactions",
		"#week":         "1 of 4 transactions",
		"@wise":         "1 of 4 transactions",
		"cat:xf":        "2 of 4 transactions",
		"type:income":   "1 of 4 transactions",
		"2026-11":       "2 of 4 transactions",
		"2026-10 @int":  "1 of 4 transactions",
		"travel 2026":   "2 of 4 transactions",
		"market #rent":  "No transactions match the search",
		"@":             "4 transactions",
		"30/10/2026 tr": "2 of 4 transactions",
	} {
		searched := send(t, model, text(query)...)
		if view := ledgerView(t, searched); !strings.Contains(view, want) {
			t.Errorf("search %q lacks %q:\n%s", query, want, view)
		}
	}
	// The totals are for the transactions the search selects.
	searched := send(t, model, text("sal")...)
	view := ledgerView(t, searched)
	if ledger(t, searched).Query() != "sal" || !strings.Contains(view, "net R$ 3.000,00") || strings.Contains(view, "Weekly market") || strings.Contains(view, "17/11/2026") {
		t.Fatalf("search for sal:\n%s", view)
	}
	// The first escape clears the search, the second leaves.
	searched = send(t, searched, keyEscape)
	if l := ledger(t, searched); l.Cancelled() || l.Query() != "" || !strings.Contains(ledgerView(t, searched), "Weekly market") {
		t.Fatal("first escape must clear the search")
	}
	if searched = send(t, searched, keyEscape); !ledger(t, searched).Cancelled() {
		t.Fatal("second escape did not cancel")
	}
	if cancelled := send(t, model, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); !ledger(t, cancelled).Cancelled() {
		t.Fatal("ctrl+c did not cancel")
	}
	if ledger(t, searched).View().Content != "" {
		t.Fatal("closed ledger still has a view")
	}
}

func TestLedgerSelect(t *testing.T) {
	var model tea.Model = NewLedger("Transactions", sampleTransactions())
	if _, ok := ledger(t, model).Selected(); ok {
		t.Fatal("selected before enter")
	}
	if got, ok := ledger(t, send(t, model, keyEnter)).Selected(); !ok || got != 0 {
		t.Fatalf("selected = %d, %v; want 0", got, ok)
	}
	// Up at the top and down at the bottom stay in the list.
	if got, _ := ledger(t, send(t, model, keyUp, keyDown, keyDown, keyEnter)).Selected(); got != 2 {
		t.Fatalf("selected after two downs = %d, want 2", got)
	}
	if got, _ := ledger(t, send(t, model, keyDown, keyDown, keyDown, keyDown, keyDown, keyEnter)).Selected(); got != 3 {
		t.Fatalf("selected after many downs = %d, want 3", got)
	}
	// The index is of the list the caller gave, not of the search result.
	if got, ok := ledger(t, send(t, model, append(text("@wise"), keyEnter)...)).Selected(); !ok || got != 3 {
		t.Fatalf("selected after search = %d, %v; want 3", got, ok)
	}
	none := send(t, model, append(text("zzz"), keyEnter)...)
	if _, ok := ledger(t, none).Selected(); ok {
		t.Fatal("enter selected a transaction when none matches")
	}
}

func TestLedgerScrollsAndFits(t *testing.T) {
	var views []transaction.View
	base := sampleTransactions()[0]
	for i := range 40 {
		view := base
		view.ID, view.Title, view.Date = uint(i+1), fmt.Sprintf("Purchase %02d", i+1), fmt.Sprintf("2026-10-%02d", 28-i/2)
		views = append(views, view)
	}
	views[3].Title, views[3].Tags = strings.Repeat("long title ", 6), "#one #two #three #four #five #six #seven"
	var model tea.Model = NewLedger("Transactions", views)
	first := ledgerView(t, model)
	if !strings.Contains(first, "Purchase 01") || strings.Contains(first, "Purchase 40") {
		t.Fatalf("first screen:\n%s", first)
	}
	height := strings.Count(first, "\n")
	// A day heading at the bottom, without its transactions, is not shown.
	// At all heights and positions the list starts with a day heading
	// and ends with a transaction, and the screen keeps its height.
	for _, size := range []int{19, 20, 21, 22, 23} {
		sized := send(t, model, tea.WindowSizeMsg{Width: 120, Height: size})
		lines := strings.Count(ledgerView(t, sized), "\n")
		for range 12 {
			view := ledgerView(t, sized)
			rows := strings.Split(view, "\n")
			list := rows[5 : len(rows)-4]
			for len(list) > 0 && strings.TrimSpace(list[len(list)-1]) == "" {
				list = list[:len(list)-1]
			}
			if !strings.Contains(list[0], "──") || !strings.Contains(list[len(list)-1], "Purchase") || strings.Count(view, "\n") != lines {
				t.Fatalf("height %d: list must start with a day and end with a transaction:\n%s", size, view)
			}
			sized = send(t, sized, keyDown, keyDown, keyDown)
		}
	}
	for _, line := range strings.Split(first, "\n") {
		if lipglossWidth(line) > ledgerWidth {
			t.Fatalf("line is wider than the ledger: %q", line)
		}
	}
	paged := send(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if view := ledgerView(t, paged); strings.Contains(view, "Purchase 01") || strings.Count(view, "\n") != height {
		t.Fatalf("after page down (height %d, want %d):\n%s", strings.Count(view, "\n"), height, view)
	}
	last := send(t, model, tea.KeyPressMsg{Code: tea.KeyEnd})
	if view := ledgerView(t, last); !strings.Contains(view, "Purchase 40") || strings.Count(view, "\n") != height {
		t.Fatalf("after end:\n%s", view)
	}
	if got, _ := ledger(t, send(t, last, keyEnter)).Selected(); got != 39 {
		t.Fatalf("selected at the end = %d", got)
	}
	if got, _ := ledger(t, send(t, last, tea.KeyPressMsg{Code: tea.KeyHome}, keyEnter)).Selected(); got != 0 {
		t.Fatalf("selected after home = %d", got)
	}
	// The screen keeps its height when the search selects few transactions.
	if view := ledgerView(t, send(t, model, text("Purchase 07")...)); strings.Count(view, "\n") != height {
		t.Fatalf("height changed with the search:\n%s", view)
	}
	// A tall and wide terminal is all in use: the screen has one line
	// less than the terminal, for the cursor after the last line, and the
	// amounts end 2 columns before the right edge, as the left margin.
	tall := ledgerView(t, send(t, model, tea.WindowSizeMsg{Width: 120, Height: 50}))
	if got := strings.Count(tall, "\n"); got != 49 {
		t.Fatalf("tall terminal: %d lines, want 49:\n%s", got, tall)
	}
	if got := strings.Count(tall, "Purchase"); got < 19 {
		t.Fatalf("tall terminal shows %d transactions:\n%s", got, tall)
	}
	widest := 0
	for _, line := range strings.Split(tall, "\n") {
		widest = max(widest, lipglossWidth(strings.TrimRight(line, " ")))
	}
	if widest != 118 {
		t.Fatalf("wide terminal: the widest line has %d columns, want 118:\n%s", widest, tall)
	}
	// A small terminal shows fewer rows.
	small := send(t, model, tea.WindowSizeMsg{Width: 70, Height: 18})
	view := ledgerView(t, small)
	if got := strings.Count(view, "\n"); got > 18 || got >= height {
		t.Fatalf("small terminal: %d lines:\n%s", got, view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipglossWidth(line) > 70 {
			t.Fatalf("line is wider than the terminal: %q", line)
		}
	}
}

func TestLedgerMarksAVirtualCard(t *testing.T) {
	views := sampleTransactions()
	views[0].Virtual = true
	var model tea.Model = NewLedger("Transactions", views)
	if view := ledgerView(t, model); !strings.Contains(view, "▼ Weekly market virtual") || strings.Contains(view, "Salary virtual") {
		t.Fatalf("view does not mark the virtual card:\n%s", view)
	}
	// The search reads the mark.
	model = send(t, model, text("virtual:yes")...)
	if view := ledgerView(t, model); !strings.Contains(view, "Weekly market") || strings.Contains(view, "Salary") || !strings.Contains(view, "virtual:yes") {
		t.Fatalf("search for the virtual card:\n%s", view)
	}
}
