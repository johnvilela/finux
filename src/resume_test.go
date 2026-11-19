package main

import (
	"strings"
	"testing"
)

// The tests use the database of newCardsHome: the account NU in BRL with
// R$ 5.000,00 and the credit card VISA (closing day 25, due day 5). The
// date of today is 17/11/2026.

func TestResumeOfAnEmptyMonth(t *testing.T) {
	h := newCardsHome(t)
	out := h.ok("resume")
	h.has(out, "Resume of November 2026\n", "Today       17/11/2026 (day 17 of 30)\n",
		"No incomes or expenses in this month.\n", "Accounts\n", "NU    Nubank  R$ 5.000,00\n", "Total BRL  R$ 5.000,00\n")
	for _, absent := range []string{"Bills", "Budgets", "Goals", "Month in"} {
		if strings.Contains(out, absent) {
			t.Errorf("empty resume has %q:\n%s", absent, out)
		}
	}
	if got := h.ok("rs"); got != out {
		t.Fatalf("rs = %q, want the same as resume", got)
	}
}

func TestResume(t *testing.T) {
	h := newCardsHome(t)
	h.ok("ac", "a", "--name", "Wise", "--currency", "USD", "--code", "WISE", "--balance", "50")
	add := func(title, value, account, category, date string) {
		t.Helper()
		h.ok("tx", "a", "--title", title, "--value", value, "--account", account, "--category", category, "--date", date)
	}
	add("Salary", "3000", "NU", "SAL", "05/11/2026")
	add("Market", "300", "NU", "FOOD", "10/11/2026")
	add("Cinema", "150", "NU", "FUN", "12/11/2026")
	add("Dinner", "100", "VISA", "FOOD", "15/11/2026")
	add("Rent", "1000", "NU", "HOME", "10/10/2026")
	add("Book", "20", "WISE", "EDU", "02/11/2026")
	// A transfer is not an income and not an expense.
	h.ok("ac", "a", "--name", "Wallet", "--currency", "BRL", "--code", "CASH")
	h.ok("tx", "a", "--title", "Cash", "--value", "200", "--type", "transfer", "--account", "NU", "--to", "CASH", "--date", "03/11/2026")
	h.ok("bg", "add", "--title", "Food", "--limit", "800", "--currency", "BRL", "--categories", "FOOD", "--every", "month", "--code", "FOOD")
	h.ok("bg", "add", "--title", "Trip", "--limit", "900", "--currency", "BRL", "--start", "01/12/2026", "--until", "31/12/2026", "--code", "LATER")
	h.ok("gl", "add", "--title", "Trip to Japan", "--type", "save", "--target", "10000", "--currency", "BRL", "--due", "30/06/2027", "--code", "TRIP")

	out := h.ok("resume")
	h.has(out, "Resume of November 2026\n", "Today       17/11/2026 (day 17 of 30)\n",
		"Month in BRL\n", "Income      R$ 3.000,00\n", "Expenses    R$ 550,00\n", "Net         R$ 2.450,00\n", "Per day     R$ 32,35\n",
		"Top expenses in BRL\n", "FOOD  Food      72%  R$ 400,00\n", "FUN   Leisure   27%  R$ 150,00\n",
		"Month in USD\n", "Income      $0.00\n", "Expenses    $20.00\n", "Net         -$20.00\n",
		"Accounts\n", "NU    Nubank  R$ 6.350,00\n", "CASH  Wallet    R$ 200,00\n", "Total BRL  R$ 6.550,00\n", "Total USD  $30.00\n",
		"Bills to pay\n", "VISA  12/2026  open    05/12/2026  R$ 100,00\n",
		"Budgets\n", "FOOD  Food   ok      50%  R$ 400,00  R$ 800,00\n",
		"Goals\n", "TRIP  Trip to Japan  active  30/06/2027  0%  R$ 0,00  R$ 10.000,00\n")
	for _, absent := range []string{"Rent", "HOME", "LATER", "XFER"} {
		if strings.Contains(out, absent) {
			t.Errorf("resume of November has %q:\n%s", absent, out)
		}
	}

	// Another month changes the incomes and expenses only.
	october := h.ok("resume", "--month", "10/2026")
	h.has(october, "Resume of October 2026\n", "Today       17/11/2026\n", "Month in BRL\n", "Income      R$ 0,00\n", "Expenses    R$ 1.000,00\n",
		"Net         -R$ 1.000,00\n", "Per day     R$ 32,25\n", "HOME  Housing   100%  R$ 1.000,00\n", "Total BRL  R$ 6.550,00\n", "Bills to pay\n")
	if strings.Contains(october, "day 17 of") || strings.Contains(october, "Month in USD") {
		t.Errorf("resume of October:\n%s", october)
	}
	if got := h.ok("resume", "--month", "2026/10"); got != october {
		t.Fatalf("--month 2026/10 = %q, want the same as 10/2026", got)
	}
	// A month that did not start has no average per day.
	if december := h.ok("resume", "--month", "12/2026"); !strings.Contains(december, "Resume of December 2026\n") || strings.Contains(december, "Per day") {
		t.Errorf("resume of December:\n%s", december)
	}

	h.fail(2, "invalid month", "resume", "--month", "2026")
	h.fail(2, "invalid month", "resume", "--month", "17/11/2026")
	h.fail(2, "unexpected argument", "resume", "extra")
	h.has(h.ok("resume", "--help"), "Usage: finux resume", "--month")
}
