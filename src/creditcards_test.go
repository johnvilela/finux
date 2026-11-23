package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// cardsHome is a database with the account NU and the credit card VISA
// (closing day 25, due day 5). The date of today is 17/11/2026.
type cardsHome struct {
	t    *testing.T
	home string
}

func newCardsHome(t *testing.T) cardsHome {
	t.Helper()
	h := cardsHome{t: t, home: filepath.Join(t.TempDir(), "data")}
	h.ok("init")
	h.ok("ac", "a", "--name", "Nubank", "--currency", "BRL", "--code", "NU", "--balance", "5000")
	if got := h.ok("cc", "add", "--name", "Visa Gold", "--currency", "brl", "--limit", "5000", "--closing", "25", "--due", "5", "--code", "visa"); got != "✓ Credit card VISA created\n" {
		t.Fatalf("add output = %q", got)
	}
	return h
}

func (h cardsHome) run(args ...string) (stdout, stderr string, status int) {
	h.t.Helper()
	return finuxEnv(h.t, []string{"FINUX_DB=" + filepath.Join(h.home, "finux.db"), "HOME=" + h.home, "FINUX_TODAY=2026-11-17"}, args...)
}

func (h cardsHome) ok(args ...string) string {
	h.t.Helper()
	stdout, stderr, status := h.run(args...)
	if status != 0 || stderr != "" {
		h.t.Fatalf("finux %v: status %d, stderr %q, stdout %q", args, status, stderr, stdout)
	}
	return stdout
}

func (h cardsHome) fail(wantStatus int, wantStderr string, args ...string) {
	h.t.Helper()
	stdout, stderr, status := h.run(args...)
	if status != wantStatus || stdout != "" || !strings.Contains(stderr, wantStderr) || !strings.HasPrefix(stderr, "finux: ") {
		h.t.Fatalf("finux %v: status %d, stdout %q, stderr %q; want status %d and stderr with %q", args, status, stdout, stderr, wantStatus, wantStderr)
	}
}

func (h cardsHome) has(output string, wants ...string) {
	h.t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			h.t.Fatalf("output lacks %q:\n%s", want, output)
		}
	}
}

func (h cardsHome) purchase(title, value, date string) {
	h.t.Helper()
	h.ok("tx", "a", "--title", title, "--value", value, "--account", "VISA", "--category", "FOOD", "--date", date)
}

func TestCreditCardsLifecycle(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	succeed(t, home, "init")
	const empty = "No credit cards yet. Run 'finux cc add'.\n"
	for _, args := range [][]string{{"cc"}, {"credit-card"}, {"cc", "list"}, {"credit-card", "l"}} {
		if got := succeed(t, home, args...); got != empty {
			t.Fatalf("finux %v = %q", args, got)
		}
	}
	if got := succeed(t, home, "cc", "--help"); !strings.Contains(got, "Manage credit cards") || !strings.Contains(got, "finux credit-card [command]") {
		t.Fatalf("help = %q", got)
	}
	if got := succeed(t, home); !strings.Contains(got, "  credit-card, cc   Manage credit cards\n") {
		t.Fatalf("welcome lacks the credit card module:\n%s", got)
	}

	h := newCardsHome(t)
	h.fail(2, "add: --name, --currency, --limit, --closing and --due are necessary when input is not a terminal", "cc", "add", "--name", "X")
	h.fail(2, "add: closing day must be from 1 to 31", "cc", "add", "--name", "X", "--currency", "BRL", "--limit", "1", "--closing", "40", "--due", "5")
	h.fail(2, "add: invalid amount", "cc", "add", "--name", "X", "--currency", "BRL", "--limit", "abc", "--closing", "4", "--due", "5")
	h.fail(1, "code is already in use: NU", "cc", "add", "--name", "X", "--code", "NU", "--currency", "BRL", "--limit", "1", "--closing", "4", "--due", "5")
	h.fail(1, "account code is already in use: VISA", "ac", "add", "--name", "X", "--code", "VISA", "--currency", "BRL")
	// The code comes from the name when it is not given.
	if got := h.ok("cc", "a", "--name", "Amex", "--currency", "USD", "--limit", "2000", "--closing", "3", "--due", "10"); got != "✓ Credit card AMEX created\n" {
		t.Fatalf("add output = %q", got)
	}

	list := h.ok("cc")
	h.has(list, "CODE", "NAME", "CLOSES", "DUE", "LIMIT", "USED", "AVAILABLE", "VISA", "Visa Gold", "R$ 5.000,00", "AMEX", "$2,000.00", "2 credit cards\n")
	for _, args := range [][]string{{"cc", "VISA"}, {"cc", "visa"}, {"cc", "show", "VISA"}, {"credit-card", "1"}} {
		h.has(h.ok(args...), "VISA  Visa Gold\n", "Limit      R$ 5.000,00\n", "Used       R$ 0,00\n", "Closing    day 25\n", "Due        day 5\n",
			"Open bill 12/2026: closes 25/11/2026, due 05/12/2026\n", "No transactions in this bill.\n")
	}
	h.fail(1, "credit card not found: NOPE", "cc", "NOPE")
	h.fail(2, "show: a credit card code or id is necessary when input is not a terminal", "cc", "show")

	if got := h.ok("cc", "e", "VISA", "--limit", "8000", "--due", "7", "--name", "Visa Platinum"); got != "✓ Credit card VISA updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	h.has(h.ok("cc", "VISA"), "VISA  Visa Platinum\n", "Limit      R$ 8.000,00\n", "Due        day 7\n", "due 07/12/2026")
	h.fail(2, "edit: due day must be from 1 to 31", "cc", "e", "VISA", "--due", "0")
	h.fail(2, "edit: give --name", "cc", "e", "VISA")

	h.fail(2, "delete: --yes is necessary", "cc", "d", "VISA")
	if got := h.ok("cc", "d", "VISA", "--yes"); got != "✓ Credit card VISA archived\n" {
		t.Fatalf("archive output = %q", got)
	}
	if got := h.ok("cc"); strings.Contains(got, "VISA") {
		t.Fatalf("archived card in the list:\n%s", got)
	}
	h.has(h.ok("cc", "l", "--all"), "Visa Platinum (archived)")
	h.fail(1, "account not found: VISA", "tx", "a", "--title", "X", "--value", "1", "--account", "VISA", "--category", "FOOD")
	if got := h.ok("cc", "r", "VISA"); got != "✓ Credit card VISA restored\n" {
		t.Fatalf("restore output = %q", got)
	}
	if got := h.ok("cc", "d", "AMEX", "--hard", "--yes"); got != "✓ Credit card AMEX deleted permanently\n" {
		t.Fatalf("delete output = %q", got)
	}
	h.fail(1, "credit card not found: AMEX", "cc", "AMEX")
}

func TestCreditCardBills(t *testing.T) {
	h := newCardsHome(t)
	h.purchase("Market", "100", "17/11/2026")
	h.purchase("Old shoes", "850", "10/10/2026")
	h.fail(1, "a credit card cannot have a transfer", "tx", "a", "--title", "X", "--value", "1", "--account", "VISA", "--to", "NU")
	h.fail(1, "account not found: VISA", "tx", "a", "--title", "X", "--value", "1", "--account", "NU", "--to", "VISA")
	// Without --date the transaction has the date of today.
	h.ok("tx", "a", "--title", "Coffee", "--value", "8", "--account", "VISA", "--category", "FOOD")

	show := h.ok("cc", "VISA")
	h.has(show, "Used       R$ 958,00\n", "Available  R$ 4.042,00\n",
		"Bill 11/2026 is late: R$ 850,00, due 05/11/2026\n",
		"Open bill 12/2026: closes 25/11/2026, due 05/12/2026\n", "Coffee", "Market", "-R$ 100,00", "Total  R$ 108,00\n")
	if strings.Contains(show, "Old shoes") {
		t.Fatalf("details show a transaction of another bill:\n%s", show)
	}
	bills := strings.Split(h.ok("cc", "bills", "VISA"), "\n")
	h.has(bills[0], "BILL", "CLOSES", "DUE", "STATUS", "PAID", "TOTAL")
	h.has(bills[1], "12/2026", "25/11/2026", "05/12/2026", "open", "R$ 108,00")
	h.has(bills[2], "11/2026", "25/10/2026", "05/11/2026", "late", "R$ 850,00")
	h.has(h.ok("tx", "--account", "VISA"), "Market", "Old shoes", "Coffee", "3 transactions\n")
	h.has(h.ok("tx", "1"), "Card      VISA  Visa Gold\n", "Bill      12/2026\n")

	h.fail(2, "pay: --from is necessary", "cc", "pay", "VISA")
	h.fail(1, "account not found: NOPE", "cc", "pay", "VISA", "--from", "NOPE")
	h.fail(1, "bill 12/2026 of VISA is still open", "cc", "pay", "VISA", "--from", "NU", "--bill", "12/2026")
	h.fail(1, "bill 01/2020 of VISA not found", "cc", "pay", "VISA", "--from", "NU", "--bill", "01/2020")
	h.fail(2, "pay: invalid bill", "cc", "pay", "VISA", "--from", "NU", "--bill", "x")
	h.fail(1, "value is more than the total of the bill: R$ 850,00", "cc", "pay", "VISA", "--from", "NU", "--value", "900")

	if got := h.ok("cc", "pay", "VISA", "--from", "NU", "--value", "600"); got != "✓ Bill 11/2026 of VISA paid: transaction 4; R$ 250,00 carried to bill 12/2026\n" {
		t.Fatalf("pay output = %q", got)
	}
	h.has(h.ok("ac", "NU"), "Balance   R$ 4.400,00\n")
	h.has(h.ok("cc", "VISA"), "Used       R$ 358,00\n", "Carried from the bill before  R$ 250,00\n", "Total  R$ 358,00\n")
	if strings.Contains(h.ok("cc", "VISA"), "is late") {
		t.Fatal("a paid bill is shown as late")
	}
	h.has(h.ok("tx", "4"), "Payment VISA 11/2026\n", "Type      payment\n", "Value     -R$ 600,00\n", "Account   NU  Nubank\n", "Payment   bill 11/2026 of VISA  Visa Gold\n")
	h.has(h.ok("cc", "b", "VISA"), "paid", "R$ 600,00")

	// A paid bill is locked.
	h.fail(1, "bill 11/2026 of VISA is paid: delete its payment first", "tx", "e", "2", "--title", "X")
	h.fail(1, "bill 11/2026 of VISA is paid", "tx", "d", "2", "--yes")
	h.fail(1, "bill 11/2026 of VISA is paid", "tx", "a", "--title", "X", "--value", "1", "--account", "VISA", "--category", "FOOD", "--date", "11/10/2026")
	h.fail(1, "a payment cannot change", "tx", "e", "4", "--title", "X")
	h.fail(1, "no bill of VISA waits for a payment", "cc", "pay", "VISA", "--from", "NU")

	h.fail(2, "unpay: --yes is necessary", "cc", "unpay", "VISA")
	if got := h.ok("cc", "unpay", "VISA", "--yes"); got != "✓ Payment of bill 11/2026 of VISA deleted\n" {
		t.Fatalf("unpay output = %q", got)
	}
	h.fail(1, "VISA has no paid bill", "cc", "unpay", "VISA", "--yes")
	h.has(h.ok("ac", "NU"), "Balance   R$ 5.000,00\n")
	if got := h.ok("cc", "p", "VISA", "--from", "NU", "--date", "20/11/2026"); got != "✓ Bill 11/2026 of VISA paid: transaction 5\n" {
		t.Fatalf("full pay output = %q", got)
	}
	// Delete of the payment transaction also opens the bill.
	if got := h.ok("tx", "d", "5", "--yes"); got != "✓ Transaction 5 deleted\n" {
		t.Fatalf("delete of the payment = %q", got)
	}
	h.has(h.ok("cc", "VISA"), "Bill 11/2026 is late")
	h.ok("cc", "pay", "VISA", "--from", "NU", "--bill", "11/2026")

	// A permanent delete keeps the payment as an expense of the account.
	if got := h.ok("cc", "d", "VISA", "--hard", "--yes"); got != "✓ Credit card VISA deleted permanently, with 3 transactions and 1 payment\n" {
		t.Fatalf("hard delete output = %q", got)
	}
	h.has(h.ok("tx"), "Payment VISA 11/2026", "1 transaction\n")
	h.has(h.ok("tx", "6"), "Type      expense\n", "Payment of VISA (deleted card)\n", "Category  OTHER")
	h.has(h.ok("ac", "NU"), "Balance   R$ 4.150,00\n")
	// The code is free again.
	h.ok("ac", "a", "--name", "Visa account", "--code", "VISA", "--currency", "BRL")
}

func TestAccountDeleteKeepsBillsPaid(t *testing.T) {
	h := newCardsHome(t)
	h.purchase("Old shoes", "850", "10/10/2026")
	h.ok("cc", "pay", "VISA", "--from", "NU")
	if got := h.ok("ac", "d", "NU", "--hard", "--yes"); got != "✓ Account NU deleted permanently, with 1 transaction and 0 transfers\n" {
		t.Fatalf("delete output = %q", got)
	}
	h.has(h.ok("cc", "b", "VISA"), "11/2026", "paid")
}

func TestVirtualCardTransactions(t *testing.T) {
	h := newCardsHome(t)
	h.purchase("Market", "100", "17/11/2026")
	if got := h.ok("tx", "a", "--title", "Streaming", "--value", "40", "--account", "VISA", "--category", "FOOD", "--date", "16/11/2026", "--virtual"); got != "✓ Transaction 2 created\n" {
		t.Fatalf("add output = %q", got)
	}
	// Only a credit card has virtual cards.
	h.fail(1, "only a credit card transaction can be made with a virtual card",
		"tx", "a", "--title", "Bread", "--value", "5", "--account", "NU", "--category", "FOOD", "--virtual")

	h.has(h.ok("tx", "2"), "Card      VISA  Visa Gold\n", "Virtual   yes\n")
	if got := h.ok("tx", "1"); strings.Contains(got, "Virtual") {
		t.Fatalf("card of a transaction with no mark:\n%s", got)
	}
	const marked = "2   16/11/2026  Streaming [virtual]  FOOD      VISA      -R$ 40,00\n"
	h.has(h.ok("tx"), marked, "Market     ")
	if got := h.ok("tx", "l", "--virtual"); !strings.Contains(got, "Streaming [virtual]") || strings.Contains(got, "Market") || !strings.Contains(got, "1 transaction\n") {
		t.Fatalf("list --virtual =\n%s", got)
	}
	if got := h.ok("tx", "l", "--virtual=false"); strings.Contains(got, "Streaming") || !strings.Contains(got, "Market") {
		t.Fatalf("list --virtual=false =\n%s", got)
	}

	// The open bill marks the transaction and gives the total of the virtual card.
	h.has(h.ok("cc", "VISA"), "Streaming [virtual]", "Virtual card  R$ 40,00 (1 transaction)\n", "Total  R$ 140,00\n")

	// An edit takes the mark away, and gives it.
	h.ok("tx", "e", "2", "--virtual=false")
	if got := h.ok("cc", "VISA"); strings.Contains(got, "irtual") {
		t.Fatalf("card with no virtual transaction:\n%s", got)
	}
	h.ok("tx", "e", "1", "--virtual")
	h.has(h.ok("tx", "1"), "Virtual   yes\n")
	// A transaction that goes to an account loses the mark.
	h.ok("tx", "e", "1", "--account", "NU")
	if got := h.ok("tx", "1"); strings.Contains(got, "Virtual") {
		t.Fatalf("card of a transaction that went to an account:\n%s", got)
	}
	h.fail(1, "only a credit card transaction", "tx", "e", "1", "--virtual")

	// All the parts of a purchase have the mark; --all changes it in each one.
	h.ok("tx", "a", "--title", "Phone", "--value", "300", "--account", "VISA", "--category", "FOOD", "--date", "17/11/2026", "--installments", "3", "--virtual")
	if got := h.ok("tx", "l", "--virtual"); !strings.Contains(got, "3 transactions\n") || !strings.Contains(got, "Phone 2/3 [virtual]") {
		t.Fatalf("parts with the mark =\n%s", got)
	}
	h.ok("tx", "e", "4", "--virtual=false", "--next")
	if got := h.ok("tx", "l", "--virtual"); !strings.Contains(got, "1 transaction\n") || !strings.Contains(got, "Phone 1/3 [virtual]") {
		t.Fatalf("parts after --next =\n%s", got)
	}
}
