package main

import (
	"strings"
	"testing"
)

// The tests use the database of newCardsHome: the account NU in BRL with
// R$ 5.000,00 and the credit card VISA. The date of today is 17/11/2026.

func TestGoalsLifecycle(t *testing.T) {
	h := newCardsHome(t)
	if got := h.ok("gl"); got != "No goals yet. Run 'finux gl add'.\n" {
		t.Fatalf("empty list = %q", got)
	}
	if got := h.ok("goals", "add", "--title", "Trip to Japan", "--type", "save", "--target", "10000", "--currency", "brl", "--due", "30/06/2027", "--code", "trip", "--description", "In 2027"); got != "✓ Goal TRIP created\n" {
		t.Fatalf("add output = %q", got)
	}
	// Without a code, the title gives it. "pay off" has three spellings.
	for _, kind := range []string{"payoff", "pay-off", "Pay Off"} {
		h.ok("gl", "a", "--title", "Debt "+kind, "--type", kind, "--target", "800", "--currency", "BRL")
	}
	h.has(h.ok("gl", "show", "DPAYO"), "Type       payoff")

	h.fail(2, "--title, --type, --target and --currency are necessary", "gl", "add", "--title", "X")
	h.fail(2, `unknown type "budget": use one of save, payoff, purchase`, "gl", "add", "--title", "X", "--type", "budget", "--target", "1", "--currency", "BRL")
	h.fail(2, "target must be more than zero", "gl", "add", "--title", "X", "--type", "save", "--target", "0", "--currency", "BRL")
	h.fail(2, "invalid date", "gl", "add", "--title", "X", "--type", "save", "--target", "1", "--currency", "BRL", "--due", "soon")
	h.fail(2, "unknown currency", "gl", "add", "--title", "X", "--type", "save", "--target", "1", "--currency", "XYZ")
	h.fail(1, "already in use", "gl", "add", "--title", "X", "--type", "save", "--target", "1", "--currency", "BRL", "--code", "TRIP")
	h.fail(2, "the code NONE is reserved", "gl", "add", "--title", "X", "--type", "save", "--target", "1", "--currency", "BRL", "--code", "none")

	list := h.ok("gl", "list")
	h.has(list, "CODE", "TITLE", "TYPE", "DUE", "STATUS", "PROGRESS", "TARGET", "TRIP", "Trip to Japan", "save", "30/06/2027", "active", "R$ 0,00", "0%", "R$ 10.000,00", "4 goals")
	details := h.ok("gl", "TRIP")
	h.has(details, "TRIP  Trip to Japan", "In 2027", "Target     R$ 10.000,00", "Progress   R$ 0,00 (0%)", "Due        30/06/2027", "Status     active", "No transactions linked to this goal.")
	if got := h.ok("gl", "show", "trip"); got != details {
		t.Fatalf("show = %q, want the details", got)
	}
	h.fail(1, "goal not found: NOPE", "gl", "show", "NOPE")
	h.fail(2, "a goal code or id is necessary", "gl", "show")

	if got := h.ok("gl", "e", "TRIP", "--title", "Japan", "--code", "JAPAN", "--type", "purchase", "--target", "8000", "--due", "none", "--description", ""); got != "✓ Goal JAPAN updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	details = h.ok("gl", "JAPAN")
	h.has(details, "JAPAN  Japan", "Type       purchase", "Target     R$ 8.000,00")
	if strings.Contains(details, "Due ") || strings.Contains(details, "In 2027") {
		t.Fatalf("edit kept the date or the description:\n%s", details)
	}
	h.fail(2, "give --title", "gl", "edit", "JAPAN")
	h.fail(2, "target must be more than zero", "gl", "edit", "JAPAN", "--target", "-5")

	if got := h.ok("gl", "d", "JAPAN", "--yes"); got != "✓ Goal JAPAN archived\n" {
		t.Fatalf("archive output = %q", got)
	}
	if list := h.ok("gl"); strings.Contains(list, "JAPAN") {
		t.Fatalf("list shows an archived goal:\n%s", list)
	}
	h.has(h.ok("gl", "l", "--all"), "Japan (archived)")
	h.fail(1, "goal JAPAN is already archived; use --hard", "gl", "d", "JAPAN", "--yes")
	h.fail(1, "it is archived; restore it first", "gl", "e", "JAPAN", "--title", "X")
	h.fail(2, "--yes is necessary", "gl", "d", "JAPAN")
	if got := h.ok("gl", "r", "JAPAN"); got != "✓ Goal JAPAN restored\n" {
		t.Fatalf("restore output = %q", got)
	}
	h.fail(1, "goal JAPAN is not archived", "gl", "r", "JAPAN")
	if got := h.ok("gl", "delete", "JAPAN", "--hard", "--yes"); got != "✓ Goal JAPAN deleted permanently\n" {
		t.Fatalf("delete output = %q", got)
	}
	h.fail(1, "goal not found: JAPAN", "gl", "show", "JAPAN")
	if got := h.ok("gl", "--help"); !strings.Contains(got, "finux goals") || !strings.Contains(got, "--target") {
		t.Fatalf("help = %q", got)
	}
}

func TestGoalLinksFromTransactions(t *testing.T) {
	h := newCardsHome(t)
	h.ok("ac", "a", "--name", "Inter", "--currency", "BRL", "--code", "INTER")
	h.ok("ac", "a", "--name", "Wise", "--currency", "USD", "--code", "WISE")
	h.ok("gl", "a", "--title", "Trip", "--type", "save", "--target", "1000", "--currency", "BRL", "--code", "TRIP", "--due", "10/11/2026")
	h.ok("gl", "a", "--title", "Loan", "--type", "payoff", "--target", "500", "--currency", "BRL", "--code", "LOAN")

	h.ok("tx", "a", "--title", "Bonus", "--value", "400", "--account", "NU", "--category", "SAL", "--goal", "trip")
	h.ok("tx", "a", "--title", "Withdrawal", "--value", "100", "--account", "NU", "--category", "OTHER", "--type", "expense", "--goal", "TRIP")
	h.ok("tx", "a", "--title", "To savings", "--value", "200", "--account", "NU", "--to", "INTER", "--goal", "TRIP")
	h.ok("tx", "a", "--title", "Loan part", "--value", "150", "--account", "VISA", "--category", "OTHER", "--type", "expense", "--goal", "LOAN")
	h.ok("tx", "a", "--title", "Market", "--value", "50", "--account", "NU", "--category", "FOOD")

	details := h.ok("gl", "TRIP")
	h.has(details, "Progress   R$ 500,00 (50%)", "Remaining  R$ 500,00", "Status     overdue",
		"Bonus", "+R$ 400,00", "Withdrawal", "-R$ 100,00", "To savings", "+R$ 200,00")
	if strings.Count(details, "To savings") != 1 || strings.Contains(details, "Market") {
		t.Fatalf("linked transactions:\n%s", details)
	}
	h.has(h.ok("gl", "LOAN"), "Progress   R$ 150,00 (30%)", "Loan part", "+R$ 150,00", "VISA")
	h.has(h.ok("gl"), "overdue", "50%", "30%")

	h.has(h.ok("tx", "show", "1"), "Goal      TRIP  Trip")
	if strings.Contains(h.ok("tx", "show", "6"), "Goal") {
		t.Fatal("a transaction without a goal shows a goal")
	}
	list := h.ok("tx", "list", "--goal", "TRIP")
	h.has(list, "Bonus", "Withdrawal", "To savings", "4 transactions")
	if strings.Contains(list, "Market") || strings.Contains(list, "Loan part") {
		t.Fatalf("list --goal TRIP:\n%s", list)
	}

	h.fail(1, "goal not found: NOPE", "tx", "a", "--title", "A", "--value", "1", "--account", "NU", "--category", "FOOD", "--goal", "NOPE")
	h.fail(1, "goal TRIP is in BRL, not in USD", "tx", "a", "--title", "A", "--value", "1", "--account", "WISE", "--category", "FOOD", "--goal", "TRIP")
	h.fail(1, "goal not found: NOPE", "tx", "list", "--goal", "NOPE")

	// Edit: move to a different goal, then take the goal away.
	if got := h.ok("tx", "e", "2", "--goal", "LOAN"); got != "✓ Transaction 2 updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	h.has(h.ok("gl", "TRIP"), "Progress   R$ 600,00 (60%)")
	h.has(h.ok("gl", "LOAN"), "Progress   R$ 250,00 (50%)")
	h.ok("tx", "e", "2", "--goal", "none")
	h.has(h.ok("gl", "LOAN"), "Progress   R$ 150,00 (30%)")
	if strings.Contains(h.ok("tx", "show", "2"), "Goal") {
		t.Fatal("--goal none kept the goal")
	}
	// An edit of a different field keeps the goal.
	h.ok("tx", "e", "1", "--title", "Big bonus")
	h.has(h.ok("tx", "show", "1"), "Big bonus", "Goal      TRIP  Trip")

	// A payment of a bill never has a goal.
	h.purchase("Fridge", "300", "2026-10-20")
	h.ok("cc", "pay", "VISA", "--from", "NU")
	h.fail(1, "a payment cannot change", "tx", "e", "8", "--goal", "LOAN")

	// An archived goal keeps its links and takes no new one.
	h.ok("gl", "d", "TRIP", "--yes")
	h.fail(1, "goal TRIP is archived: restore it first", "tx", "a", "--title", "A", "--value", "1", "--account", "NU", "--category", "SAL", "--goal", "TRIP")
	h.has(h.ok("gl", "show", "TRIP"), "Progress   R$ 600,00 (60%)", "archived")
	h.ok("tx", "e", "1", "--title", "Bonus")
	h.has(h.ok("tx", "show", "1"), "Goal      TRIP  Trip")

	// A permanent delete keeps the transactions, without the goal.
	if got := h.ok("gl", "d", "TRIP", "--hard", "--yes"); got != "✓ Goal TRIP deleted permanently; 2 transactions lost the link\n" {
		t.Fatalf("delete output = %q", got)
	}
	shown := h.ok("tx", "show", "1")
	h.has(shown, "Bonus", "R$ 400,00")
	if strings.Contains(shown, "Goal") {
		t.Fatalf("the transaction kept the deleted goal:\n%s", shown)
	}
	h.has(h.ok("tx", "list"), "8 transactions")
	h.has(h.ok("ac", "NU"), "R$ 4.750,00")
}
