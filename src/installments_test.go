package main

import (
	"strings"
	"testing"
)

// The tests use the database of newCardsHome: the account NU in BRL with
// R$ 5.000,00 and the credit card VISA (closing day 25, due day 5). The
// date of today is 17/11/2026.

func TestInstallmentsAdd(t *testing.T) {
	h := newCardsHome(t)
	if got := h.ok("tx", "a", "--title", "Notebook", "--value", "1200", "--installments", "10", "--account", "VISA", "--category", "SHOP"); got != "✓ Transactions 1 to 10 created: 10 installments of R$ 120,00\n" {
		t.Fatalf("add output = %q", got)
	}
	h.has(h.ok("tx", "3"), "Notebook\n", "Value     -R$ 120,00\n", "Date      17/01/2027\n", "Part      3/10\n", "Bill      02/2027\n")
	h.has(h.ok("tx", "--account", "VISA"), "10  17/08/2027  Notebook 10/10  SHOP", "1   17/11/2026  Notebook 1/10   SHOP", "10 transactions\n")
	// All the parts use the limit of the card from the first day.
	h.has(h.ok("cc", "VISA"), "R$ 1.200,00")

	// The cents that the division leaves go to the first part. On an
	// account a part changes the balance on its date.
	if got := h.ok("tx", "a", "--title", "Sofa", "--value", "100", "--installments", "3", "--account", "NU", "--category", "SHOP"); got != "✓ Transactions 11 to 13 created: 3 installments, the first of R$ 33,34 and the others of R$ 33,33\n" {
		t.Fatalf("add output = %q", got)
	}
	h.has(h.ok("ac", "NU"), "R$ 4.966,66")
	h.ok("tx", "a", "--title", "Rent", "--value", "10", "--account", "NU", "--category", "HOME", "--date", "18/11/2026")
	h.has(h.ok("ac", "NU"), "R$ 4.966,66")

	// A purchase in progress: the value of one part and the first part to create.
	if got := h.ok("tx", "a", "--title", "Phone", "--installment-value", "50", "--installments", "6", "--from", "4", "--account", "VISA", "--category", "SHOP"); got != "✓ Transactions 15 to 17 created: installments 4 to 6 of 6, R$ 50,00 each\n" {
		t.Fatalf("add output = %q", got)
	}
	h.has(h.ok("tx", "15"), "Part      4/6\n", "Date      17/11/2026\n", "Bill      12/2026\n", "Value     -R$ 50,00\n")
	if got := h.ok("tx", "a", "--title", "Bonus", "--installment-value", "10", "--installments", "2", "--account", "NU", "--category", "SAL"); got != "✓ Transactions 18 to 19 created: 2 installments of R$ 10,00\n" {
		t.Fatalf("income output = %q", got)
	}

	h.ok("ac", "a", "--name", "Wallet", "--currency", "BRL", "--code", "CASH")
	base := []string{"tx", "a", "--title", "X", "--account", "NU", "--category", "SHOP"}
	for want, args := range map[string][]string{
		"add: give --value or --installment-value, not the two":             {"--value", "10", "--installment-value", "5", "--installments", "2"},
		"add: --installment-value is only for installments":                 {"--installment-value", "5"},
		"add: --from is only for installments: give --installments":         {"--value", "10", "--from", "2"},
		"add: --from must be a number from 1 to 6":                          {"--value", "10", "--installments", "6", "--from", "7"},
		"add: --installments must be a number from 2 to 120":                {"--value", "10", "--installments", "1"},
		"add: --installments must be a number from 2 to 120\n":              {"--value", "10", "--installments", "121"},
		"add: a transfer cannot have installments":                          {"--value", "10", "--installments", "2", "--to", "CASH"},
		"value is too small for 3 installments":                             {"--value", "0,02", "--installments", "3"},
		"add: invalid value \"x\" for flag -installments":                   {"--value", "10", "--installments", "x"},
		"--title, --value, --account and --category are necessary when inp": {"--installments", "3"},
	} {
		status := 2
		if strings.HasPrefix(want, "value is") {
			status = 1
		}
		h.fail(status, strings.TrimSpace(want), append(append([]string{}, base...), args...)...)
	}
	h.has(h.ok("tx", "--help"), "--installments <number>", "--installment-value <amount>", "--from <number>", "--all", "--next")
}

func TestInstallmentsBudgetWarning(t *testing.T) {
	h := newCardsHome(t)
	h.ok("bg", "add", "--title", "Health", "--limit", "100", "--currency", "BRL", "--categories", "HEALT", "--every", "month", "--code", "HLT")
	// Only the first part gives a warning.
	want := "✓ Transactions 1 to 3 created: 3 installments of R$ 100,00\n! Budget HLT: 100% spent, R$ 0,00 left until 30/11/2026\n"
	if got := h.ok("tx", "a", "--title", "Dentist", "--value", "300", "--installments", "3", "--account", "NU", "--category", "HEALT"); got != want {
		t.Fatalf("add output = %q", got)
	}
}

func TestInstallmentsEditAndDelete(t *testing.T) {
	h := newCardsHome(t)
	h.ok("tx", "a", "--title", "Notebook", "--value", "1200", "--installments", "10", "--account", "VISA", "--category", "SHOP")
	h.ok("tx", "a", "--title", "Market", "--value", "30", "--account", "NU", "--category", "FOOD")

	// Without a scope, a change is for one part.
	if got := h.ok("tx", "e", "3", "--value", "150"); got != "✓ Transaction 3 updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	h.has(h.ok("tx", "3"), "Value     -R$ 150,00\n", "Bill      02/2027\n")
	h.has(h.ok("tx", "4"), "Value     -R$ 120,00\n")

	if got := h.ok("tx", "e", "3", "--next", "--category", "EDU"); got != "✓ Installments 3 to 10 of Notebook updated (8 transactions)\n" {
		t.Fatalf("edit --next output = %q", got)
	}
	h.has(h.ok("tx", "2"), "Category  SHOP")
	h.has(h.ok("tx", "10"), "Category  EDU", "Bill      09/2027\n")
	if got := h.ok("tx", "edit", "5", "--all", "--title", "Laptop", "--tags", "work"); got != "✓ Installments 1 to 10 of Laptop updated (10 transactions)\n" {
		t.Fatalf("edit --all output = %q", got)
	}
	h.has(h.ok("tx", "1"), "Laptop\n", "Tags      #work\n", "Value     -R$ 120,00\n")
	// With a scope, a value is the total of its parts.
	h.ok("tx", "e", "9", "--next", "--value", "301")
	h.has(h.ok("tx", "9"), "Value     -R$ 150,50\n")
	h.has(h.ok("tx", "10"), "Value     -R$ 150,50\n")
	h.ok("tx", "e", "9", "--next", "--installment-value", "100")
	h.has(h.ok("tx", "10"), "Value     -R$ 100,00\n")
	// A date is for the first part of the scope; the others follow.
	h.ok("tx", "e", "9", "--next", "--date", "26/08/2027")
	h.has(h.ok("tx", "9"), "Date      26/08/2027\n", "Bill      10/2027\n")
	h.has(h.ok("tx", "10"), "Date      26/09/2027\n", "Bill      11/2027\n")

	h.fail(2, "edit: give --all or --next, not the two", "tx", "e", "3", "--all", "--next", "--title", "X")
	h.fail(2, "edit: transaction 11 is not an installment: --all and --next do not apply", "tx", "e", "11", "--all", "--title", "X")
	h.fail(2, "edit: --installment-value needs --all or --next", "tx", "e", "3", "--installment-value", "10")
	h.fail(2, "edit: give the options to change with --all or --next", "tx", "e", "3", "--all")
	h.fail(2, "edit: give --value or --installment-value, not the two", "tx", "e", "3", "--all", "--value", "10", "--installment-value", "1")

	h.fail(2, "delete: --yes is necessary", "tx", "d", "8", "--next")
	h.fail(2, "delete: give --all or --next, not the two", "tx", "d", "8", "--next", "--all", "--yes")
	h.fail(2, "delete: transaction 11 is not an installment", "tx", "d", "11", "--all", "--yes")
	if got := h.ok("tx", "d", "8", "--next", "--yes"); got != "✓ Installments 8 to 10 of Laptop deleted (3 transactions)\n" {
		t.Fatalf("delete --next output = %q", got)
	}
	if got := h.ok("tx", "delete", "2", "--yes"); got != "✓ Transaction 2 deleted\n" {
		t.Fatalf("delete output = %q", got)
	}
	// The parts that stay keep their numbers.
	h.has(h.ok("tx", "7"), "Part      7/10\n")
	if got := h.ok("tx", "d", "4", "--all", "--yes"); got != "✓ Installments 1 to 7 of Laptop deleted (6 transactions)\n" {
		t.Fatalf("delete --all output = %q", got)
	}
	h.has(h.ok("tx"), "1 transaction\n")

	// A part in a paid bill stops a change of all the parts.
	h.ok("tx", "a", "--title", "Chair", "--value", "300", "--installments", "3", "--account", "VISA", "--category", "SHOP", "--date", "20/10/2026")
	h.ok("cc", "pay", "VISA", "--from", "NU", "--bill", "11/2026", "--date", "17/11/2026")
	h.fail(1, "installment 1/3 (transaction 12): bill 11/2026 of VISA is paid", "tx", "e", "13", "--all", "--title", "Stool")
	h.fail(1, "installment 1/3 (transaction 12): bill 11/2026 of VISA is paid", "tx", "d", "13", "--all", "--yes")
	if got := h.ok("tx", "d", "13", "--next", "--yes"); got != "✓ Installments 2 to 3 of Chair deleted (2 transactions)\n" {
		t.Fatalf("delete --next output = %q", got)
	}
}
