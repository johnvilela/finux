package main

import (
	"strings"
	"testing"
)

// The tests use the database of newCardsHome: the account NU in BRL with
// R$ 5.000,00 and the credit card VISA. The date of today is 17/11/2026,
// a Tuesday.

func TestBudgetsLifecycle(t *testing.T) {
	h := newCardsHome(t)
	if got := h.ok("bg"); got != "No budgets yet. Run 'finux bg add'.\n" {
		t.Fatalf("empty list = %q", got)
	}
	if got := h.ok("budgets", "add", "--title", "Food", "--limit", "800", "--currency", "brl", "--categories", "food, fun", "--every", "month", "--code", "food", "--description", "Market"); got != "✓ Budget FOOD created\n" {
		t.Fatalf("add output = %q", got)
	}
	details := h.ok("bg", "FOOD")
	h.has(details, "FOOD  Food", "Market", "Currency    BRL", "Limit       R$ 800,00", "Period      every month", "Start       01/11/2026",
		"Rollover    no", "Categories  FOOD, FUN", "Status      ok", "Current period: 01/11/2026 to 30/11/2026",
		"Available   R$ 800,00", "Spent       R$ 0,00 (0%)", "Remaining   R$ 800,00", "No transactions count in this period.")
	if got := h.ok("budget", "show", "food"); got != details {
		t.Fatalf("show = %q, want the details", got)
	}

	// The start date is the first day of the periods. Without it, a week
	// starts on Monday and any other period starts today.
	h.ok("bg", "a", "--title", "Pay cycle", "--limit", "500", "--currency", "BRL", "--start", "05/11/2026", "--every", "month", "--rollover", "--code", "CYCLE")
	h.has(h.ok("bg", "CYCLE"), "Start       05/11/2026", "Rollover    yes", "Categories  all", "Current period: 05/11/2026 to 04/12/2026", "Carried     R$ 0,00")
	h.ok("bg", "a", "--title", "Week", "--limit", "100", "--currency", "BRL", "--every", "weekly", "--code", "WEEK")
	h.has(h.ok("bg", "WEEK"), "Period      every week", "Current period: 16/11/2026 to 22/11/2026")
	h.ok("bg", "a", "--title", "Sprint", "--limit", "100", "--currency", "BRL", "--every", "2 weeks", "--until", "31/12/2026", "--code", "SPRIN")
	h.has(h.ok("bg", "SPRIN"), "Period      every 2 weeks", "Start       17/11/2026", "End         31/12/2026", "Current period: 17/11/2026 to 30/11/2026")
	// An end date without a period is a one-time budget.
	h.ok("bg", "a", "--title", "Trip to Japan", "--limit", "3000", "--currency", "USD", "--start", "01/12/2026", "--until", "31/12/2026")
	h.has(h.ok("bg", "TTJAP"), "Period      one time", "Limit       $3,000.00", "Status      upcoming", "The budget starts on 01/12/2026.")

	h.fail(2, "--title, --limit and --currency are necessary, with --every or --until", "bg", "add", "--title", "X")
	h.fail(2, "--title, --limit and --currency are necessary, with --every or --until", "bg", "add", "--title", "X", "--limit", "1", "--currency", "BRL")
	h.fail(2, "give a period or an end date", "bg", "add", "--title", "X", "--limit", "1", "--currency", "BRL", "--every", "none")
	h.fail(2, "limit must be more than zero", "bg", "add", "--title", "X", "--limit", "0", "--currency", "BRL", "--every", "month")
	h.fail(2, "unknown currency", "bg", "add", "--title", "X", "--limit", "1", "--currency", "XYZ", "--every", "month")
	h.fail(2, `invalid period "fortnight"`, "bg", "add", "--title", "X", "--limit", "1", "--currency", "BRL", "--every", "fortnight")
	h.fail(2, "invalid date", "bg", "add", "--title", "X", "--limit", "1", "--currency", "BRL", "--every", "month", "--start", "soon")
	h.fail(2, "end date is before the start date", "bg", "add", "--title", "X", "--limit", "1", "--currency", "BRL", "--start", "10/11/2026", "--until", "09/11/2026")
	h.fail(2, "category not found: NOPE", "bg", "add", "--title", "X", "--limit", "1", "--currency", "BRL", "--every", "month", "--categories", "FOOD,NOPE")
	h.fail(1, "already in use", "bg", "add", "--title", "X", "--limit", "1", "--currency", "BRL", "--every", "month", "--code", "FOOD")

	list := h.ok("bg", "list")
	h.has(list, "CODE", "TITLE", "PERIOD", "UNTIL", "STATUS", "SPENT", "AVAILABLE", "FOOD", "Food", "every month", "30/11/2026", "ok", "R$ 0,00", "0%", "R$ 800,00", "upcoming", "5 budgets")
	h.fail(1, "budget not found: NOPE", "bg", "show", "NOPE")
	h.fail(2, "a budget code or id is necessary", "bg", "show")

	if got := h.ok("bg", "e", "FOOD", "--title", "Meals", "--code", "MEALS", "--limit", "900", "--categories", "all", "--every", "2 weeks", "--start", "02/11/2026", "--until", "31/12/2026", "--rollover", "--description", ""); got != "✓ Budget MEALS updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	details = h.ok("bg", "MEALS")
	h.has(details, "MEALS  Meals", "Limit       R$ 900,00", "Period      every 2 weeks", "Start       02/11/2026", "End         31/12/2026",
		"Rollover    yes", "Categories  all", "Current period: 16/11/2026 to 29/11/2026")
	if strings.Contains(details, "Market") {
		t.Fatalf("edit kept the description:\n%s", details)
	}
	h.ok("bg", "e", "MEALS", "--rollover=false", "--until", "none", "--categories", "FOOD")
	details = h.ok("bg", "MEALS")
	h.has(details, "Rollover    no", "Categories  FOOD")
	if strings.Contains(details, "End ") {
		t.Fatalf("--until none kept the end date:\n%s", details)
	}
	h.fail(2, "give --title", "bg", "edit", "MEALS")
	h.fail(2, "limit must be more than zero", "bg", "edit", "MEALS", "--limit", "-5")
	h.fail(2, "give a period or an end date", "bg", "edit", "MEALS", "--every", "none")
	h.fail(2, "end date is before the start date", "bg", "edit", "MEALS", "--until", "01/11/2026")
	h.ok("bg", "e", "MEALS", "--every", "none", "--until", "31/12/2026")
	h.has(h.ok("bg", "MEALS"), "Period      one time", "Current period: 02/11/2026 to 31/12/2026")

	if got := h.ok("bg", "d", "MEALS", "--yes"); got != "✓ Budget MEALS archived\n" {
		t.Fatalf("archive output = %q", got)
	}
	if list := h.ok("bg"); strings.Contains(list, "MEALS") {
		t.Fatalf("list shows an archived budget:\n%s", list)
	}
	h.has(h.ok("bg", "l", "--all"), "Meals (archived)")
	h.has(h.ok("bg", "show", "MEALS"), "archived")
	h.fail(1, "budget MEALS is already archived; use --hard", "bg", "d", "MEALS", "--yes")
	h.fail(1, "it is archived; restore it first", "bg", "e", "MEALS", "--title", "X")
	h.fail(2, "--yes is necessary", "bg", "d", "MEALS")
	if got := h.ok("bg", "r", "MEALS"); got != "✓ Budget MEALS restored\n" {
		t.Fatalf("restore output = %q", got)
	}
	h.fail(1, "budget MEALS is not archived", "bg", "r", "MEALS")
	if got := h.ok("bg", "delete", "MEALS", "--hard", "--yes"); got != "✓ Budget MEALS deleted permanently\n" {
		t.Fatalf("delete output = %q", got)
	}
	h.fail(1, "budget not found: MEALS", "bg", "show", "MEALS")
	if got := h.ok("bg", "--help"); !strings.Contains(got, "finux budgets") || !strings.Contains(got, "--rollover") {
		t.Fatalf("help = %q", got)
	}
	if got := h.ok(); !strings.Contains(got, "  budgets, bg       Manage budgets\n") {
		t.Fatalf("welcome lacks the budgets command:\n%s", got)
	}
}

func TestBudgetsCountTransactions(t *testing.T) {
	h := newCardsHome(t)
	h.ok("ac", "a", "--name", "Inter", "--currency", "BRL", "--code", "INTER")
	h.ok("ac", "a", "--name", "Wise", "--currency", "USD", "--code", "WISE", "--balance", "1000")
	h.ok("bg", "a", "--title", "Food", "--limit", "800", "--currency", "BRL", "--categories", "FOOD", "--start", "01/09/2026", "--every", "month", "--code", "FOOD")
	h.ok("bg", "a", "--title", "Other", "--limit", "100", "--currency", "BRL", "--categories", "OTHER", "--every", "month", "--code", "MISC")

	// October: over the limit. September: nothing.
	h.ok("tx", "a", "--title", "Party", "--value", "900", "--account", "NU", "--category", "FOOD", "--date", "10/10/2026")
	// November: an expense of an account and a purchase of a card count.
	h.ok("tx", "a", "--title", "Market", "--value", "400", "--account", "NU", "--category", "FOOD", "--date", "03/11/2026")
	h.purchase("Dinner", "150", "2026-11-10")
	// These do not count: another category, another currency, a transfer.
	h.ok("tx", "a", "--title", "Cinema", "--value", "70", "--account", "NU", "--category", "FUN")
	h.ok("tx", "a", "--title", "Lunch abroad", "--value", "60", "--account", "WISE", "--category", "FOOD")
	h.ok("tx", "a", "--title", "To Inter", "--value", "300", "--account", "NU", "--to", "INTER")

	details := h.ok("bg", "FOOD")
	h.has(details, "Status      ok", "Current period: 01/11/2026 to 30/11/2026", "Spent       R$ 550,00 (68%)", "Remaining   R$ 250,00",
		"Market", "R$ 400,00", "Dinner", "R$ 150,00", "VISA",
		"History", "01/10/2026 to 31/10/2026", "over", "R$ 900,00", "112%", "01/09/2026 to 30/09/2026", "R$ 0,00")
	for _, other := range []string{"Cinema", "Lunch abroad", "To Inter", "Party"} {
		if strings.Contains(details, other) {
			t.Fatalf("details show %q:\n%s", other, details)
		}
	}
	h.has(h.ok("bg"), "FOOD", "R$ 550,00", "68%")

	// A past period, by any date in it.
	past := h.ok("bg", "show", "FOOD", "--period", "15/10/2026")
	h.has(past, "Status      over", "Period: 01/10/2026 to 31/10/2026", "Spent       R$ 900,00 (112%)", "Over        R$ 100,00", "Party", "01/09/2026 to 30/09/2026")
	if strings.Contains(past, "Market") || strings.Contains(past, "Current period") {
		t.Fatalf("past period:\n%s", past)
	}
	h.fail(1, "budget FOOD has no period on 31/08/2026", "bg", "show", "FOOD", "--period", "31/08/2026")
	h.fail(2, "invalid date", "bg", "show", "FOOD", "--period", "soon")
	if short := h.ok("bg", "show", "FOOD", "--history", "1"); !strings.Contains(short, "01/10/2026 to 31/10/2026") || strings.Contains(short, "01/09/2026 to 30/09/2026") {
		t.Fatalf("--history 1:\n%s", short)
	}
	if none := h.ok("bg", "show", "FOOD", "--history", "0"); strings.Contains(none, "History") {
		t.Fatalf("--history 0:\n%s", none)
	}

	// Rollover: October spent R$ 100,00 too much; September left R$ 800,00.
	h.ok("bg", "e", "FOOD", "--rollover")
	h.has(h.ok("bg", "FOOD"), "Carried     R$ 700,00", "Available   R$ 1.500,00", "Spent       R$ 550,00 (36%)")
	h.ok("bg", "e", "FOOD", "--rollover=false")

	// An income of a category of the budget gives money back.
	h.ok("tx", "a", "--title", "Gift bought", "--value", "90", "--account", "NU", "--category", "OTHER", "--type", "expense")
	h.ok("tx", "a", "--title", "Gift returned", "--value", "40", "--account", "NU", "--category", "OTHER", "--type", "income")
	h.has(h.ok("bg", "MISC"), "Spent       R$ 50,00 (50%)", "Gift bought", "R$ 90,00", "Gift returned", "-R$ 40,00")
}

func TestBudgetWarnings(t *testing.T) {
	h := newCardsHome(t)
	h.ok("bg", "a", "--title", "Food", "--limit", "800", "--currency", "BRL", "--categories", "FOOD", "--every", "month", "--code", "FOOD")
	h.ok("bg", "a", "--title", "All", "--limit", "5000", "--currency", "BRL", "--every", "month", "--code", "ALL")
	// Below 80% there is no warning.
	if got := h.ok("tx", "a", "--title", "Market", "--value", "600", "--account", "NU", "--category", "FOOD"); got != "✓ Transaction 1 created\n" {
		t.Fatalf("add below the warning = %q", got)
	}
	if got := h.ok("tx", "a", "--title", "Bakery", "--value", "80", "--account", "VISA", "--category", "FOOD"); got != "✓ Transaction 2 created\n! Budget FOOD: 85% spent, R$ 120,00 left until 30/11/2026\n" {
		t.Fatalf("add near the limit = %q", got)
	}
	if got := h.ok("tx", "a", "--title", "Dinner", "--value", "200", "--account", "NU", "--category", "FOOD"); got != "✓ Transaction 3 created\n! Budget FOOD: over the limit by R$ 80,00 until 30/11/2026\n" {
		t.Fatalf("add over the limit = %q", got)
	}
	// No warning: another period, another category, an income category, a
	// transfer.
	for _, args := range [][]string{
		{"tx", "a", "--title", "Old", "--value", "10", "--account", "NU", "--category", "FOOD", "--date", "10/10/2026"},
		{"tx", "a", "--title", "Cinema", "--value", "10", "--account", "NU", "--category", "FUN"},
		{"tx", "a", "--title", "Salary", "--value", "9000", "--account", "NU", "--category", "SAL"},
	} {
		if got := h.ok(args...); strings.Contains(got, "Budget") {
			t.Fatalf("finux %v warns: %q", args, got)
		}
	}
	// An edit warns too, for the budgets of the new values.
	if got := h.ok("tx", "e", "5", "--category", "FOOD"); got != "✓ Transaction 5 updated\n! Budget FOOD: over the limit by R$ 90,00 until 30/11/2026\n" {
		t.Fatalf("edit over the limit = %q", got)
	}
	if got := h.ok("tx", "e", "5", "--title", "Movies", "--category", "FUN"); got != "✓ Transaction 5 updated\n" {
		t.Fatalf("edit out of the budget = %q", got)
	}
	// An archived budget does not warn.
	h.ok("bg", "d", "FOOD", "--yes")
	if got := h.ok("tx", "a", "--title", "Snack", "--value", "5", "--account", "NU", "--category", "FOOD"); got != "✓ Transaction 7 created\n" {
		t.Fatalf("add with an archived budget = %q", got)
	}
}

func TestBudgetsAndDeletedCategories(t *testing.T) {
	h := newCardsHome(t)
	h.ok("bg", "a", "--title", "Both", "--limit", "800", "--currency", "BRL", "--categories", "FOOD,FUN", "--every", "month", "--code", "BOTH")
	h.ok("bg", "a", "--title", "Fun", "--limit", "100", "--currency", "BRL", "--categories", "FUN", "--every", "month", "--code", "FUN")
	h.fail(1, "category FUN is the only category of the budget FUN: change or delete the budget first", "ct", "delete", "FUN", "--yes")
	h.has(h.ok("bg", "BOTH"), "Categories  FOOD, FUN")
	// An archived budget still holds the category.
	h.ok("bg", "d", "FUN", "--yes")
	h.fail(1, "category FUN is the only category of the budget FUN", "ct", "delete", "FUN", "--yes")
	h.ok("bg", "d", "FUN", "--hard", "--yes")
	h.ok("ct", "delete", "FUN", "--yes")
	h.has(h.ok("bg", "BOTH"), "Categories  FOOD\n")
}
