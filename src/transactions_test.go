package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ledger makes a database with three accounts: INTER and NU in BRL and
// WISE in USD.
func ledger(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "data")
	succeed(t, home, "init")
	succeed(t, home, "ac", "add", "--name", "Banco Inter", "--code", "INTER", "--currency", "BRL", "--balance", "1000")
	succeed(t, home, "ac", "add", "--name", "Nubank", "--code", "NU", "--currency", "BRL")
	succeed(t, home, "ac", "add", "--name", "Wise", "--code", "WISE", "--currency", "USD", "--balance", "50")
	return home
}

func balance(t *testing.T, home, code string) string {
	t.Helper()
	for _, line := range strings.Split(succeed(t, home, "ac", "show", code), "\n") {
		if strings.HasPrefix(line, "Balance") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Balance"))
		}
	}
	t.Fatalf("no balance for %s", code)
	return ""
}

func TestTransactionsLifecycle(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 1, "Run 'finux init'", "tx")
	home = ledger(t)
	const empty = "No transactions yet. Run 'finux tx add'.\n"
	if got := succeed(t, home, "tx"); got != empty {
		t.Fatalf("empty list = %q", got)
	}

	// The category gives the type: red is an expense, green an income.
	got := succeed(t, home, "transactions", "add", "--title", "Weekly market", "--value", "50,25", "--account", "inter", "--category", "food",
		"--date", "17/11/2026", "--tags", "Market #weekly", "--description", "fruit")
	if got != "✓ Transaction 1 created\n" {
		t.Fatalf("add output = %q", got)
	}
	if got := succeed(t, home, "tx", "a", "--title", "Salary", "--value", "3000", "--account", "INTER", "--category", "SAL", "--date", "2026/11/05"); got != "✓ Transaction 2 created\n" {
		t.Fatalf("add income output = %q", got)
	}
	// A new transaction has the date of today.
	succeed(t, home, "tx", "add", "--title", "Found money", "--value", "10", "--account", "NU", "--category", "OTHER", "--type", "income")
	today := time.Now().Format("02/01/2006")
	if card := succeed(t, home, "tx", "3"); !strings.Contains(card, "Date      "+today+"\n") || !strings.Contains(card, "Type      income\n") {
		t.Fatalf("card of a transaction without date:\n%s", card)
	}
	succeed(t, home, "tx", "e", "3", "--date", "01/10/2026")

	const table = "" +
		"ID  DATE        TITLE          CATEGORY  ACCOUNT        VALUE\n" +
		"1   17/11/2026  Weekly market  FOOD      INTER      -R$ 50,25\n" +
		"2   05/11/2026  Salary         SAL       INTER    R$ 3.000,00\n" +
		"3   01/10/2026  Found money    OTHER     NU          R$ 10,00\n" +
		"3 transactions\n"
	for _, args := range [][]string{{"tx"}, {"tx", "list"}, {"transactions", "l"}, {"transactions"}} {
		if got := succeed(t, home, args...); got != table {
			t.Fatalf("finux %v =\n%s\nwant\n%s", args, got, table)
		}
	}
	if got := balance(t, home, "INTER"); got != "R$ 3.949,75" {
		t.Fatalf("balance of INTER = %q", got)
	}
	if got := succeed(t, home, "ac"); !strings.Contains(got, "R$ 3.949,75") {
		t.Fatalf("list of accounts does not show the new balance:\n%s", got)
	}

	for _, args := range [][]string{{"tx", "1"}, {"transactions", "show", "1"}} {
		card := succeed(t, home, args...)
		for _, want := range []string{"Weekly market\n", "fruit\n", "ID        1\n", "Type      expense\n", "Value     -R$ 50,25\n", "Account   INTER  Banco Inter\n", "Category  FOOD  Food\n", "Date      17/11/2026\n", "Tags      #market #weekly\n", "Created   ", "Updated   "} {
			if !strings.Contains(card, want) {
				t.Fatalf("finux %v lacks %q:\n%s", args, want, card)
			}
		}
	}

	if got := succeed(t, home, "tx", "edit", "1", "--title", "Market", "--value", "60", "--tags", "", "--description", "", "--category", "OTHER", "--account", "NU"); got != "✓ Transaction 1 updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	card := succeed(t, home, "tx", "1")
	if !strings.Contains(card, "Market\n") || strings.Contains(card, "fruit") || strings.Contains(card, "Tags") || !strings.Contains(card, "Value     -R$ 60,00\n") ||
		!strings.Contains(card, "Account   NU  Nubank\n") || !strings.Contains(card, "Category  OTHER  Other\n") || !strings.Contains(card, "Date      17/11/2026\n") {
		t.Fatalf("after edit:\n%s", card)
	}
	// An expense becomes an income; its category must accept the new type.
	fail(t, home, 1, "FOOD", "tx", "e", "1", "--type", "income", "--category", "FOOD")
	succeed(t, home, "tx", "e", "1", "--type", "income")
	if got := balance(t, home, "NU"); got != "R$ 70,00" {
		t.Fatalf("balance of NU = %q", got)
	}
	// A new account with another currency needs a new value.
	fail(t, home, 2, "--value", "tx", "e", "1", "--account", "WISE")
	succeed(t, home, "tx", "e", "1", "--account", "WISE", "--value", "12.5")
	if got := balance(t, home, "WISE"); got != "$62.50" {
		t.Fatalf("balance of WISE = %q", got)
	}

	if got := succeed(t, home, "tx", "d", "1", "--yes"); got != "✓ Transaction 1 deleted\n" {
		t.Fatalf("delete output = %q", got)
	}
	fail(t, home, 1, "transaction not found: 1", "tx", "1")
	fail(t, home, 1, "transaction not found: 1", "tx", "delete", "1", "--yes")
	fail(t, home, 1, "transaction not found: abc", "tx", "abc")
	if got := balance(t, home, "WISE"); got != "$50.00" {
		t.Fatalf("balance of WISE after delete = %q", got)
	}
}

func TestTransactionsTransfer(t *testing.T) {
	home := ledger(t)
	// --to makes a transfer; its category is XFER when none is given.
	if got := succeed(t, home, "tx", "add", "--title", "Card payment", "--value", "200", "--account", "INTER", "--to", "NU", "--date", "10/11/2026"); got != "✓ Transfer created: transactions 1 and 2\n" {
		t.Fatalf("transfer output = %q", got)
	}
	out, in := succeed(t, home, "tx", "1"), succeed(t, home, "tx", "2")
	if !strings.Contains(out, "Type      transfer\n") || !strings.Contains(out, "Value     -R$ 200,00\n") || !strings.Contains(out, "Category  XFER  Transfer\n") ||
		!strings.Contains(out, "Transfer  to NU  Nubank (transaction 2)\n") {
		t.Fatalf("outgoing side:\n%s", out)
	}
	if !strings.Contains(in, "Value     R$ 200,00\n") || !strings.Contains(in, "Account   NU  Nubank\n") || !strings.Contains(in, "Transfer  from INTER  Banco Inter (transaction 1)\n") {
		t.Fatalf("incoming side:\n%s", in)
	}
	if balance(t, home, "INTER") != "R$ 800,00" || balance(t, home, "NU") != "R$ 200,00" {
		t.Fatalf("balances = %s, %s", balance(t, home, "INTER"), balance(t, home, "NU"))
	}

	// Between currencies the transfer needs the received value.
	fail(t, home, 1, "received value is required", "tx", "add", "--title", "Travel", "--value", "550", "--account", "INTER", "--to", "WISE", "--type", "transfer")
	succeed(t, home, "tx", "add", "--title", "Travel", "--value", "550", "--account", "INTER", "--to", "WISE", "--received", "100", "--category", "OTHER")
	if balance(t, home, "INTER") != "R$ 250,00" || balance(t, home, "WISE") != "$150.00" {
		t.Fatalf("balances = %s, %s", balance(t, home, "INTER"), balance(t, home, "WISE"))
	}

	// A change of one side goes to the other side.
	if got := succeed(t, home, "tx", "e", "2", "--title", "Card", "--value", "300", "--tags", "bill"); got != "✓ Transfer updated: transactions 2 and 1\n" {
		t.Fatalf("edit output = %q", got)
	}
	if out = succeed(t, home, "tx", "1"); !strings.Contains(out, "Card\n") || !strings.Contains(out, "Value     -R$ 300,00\n") || !strings.Contains(out, "Tags      #bill\n") {
		t.Fatalf("other side after edit:\n%s", out)
	}
	for name, args := range map[string][]string{
		"type":     {"tx", "e", "1", "--type", "expense", "--category", "FOOD"},
		"account":  {"tx", "e", "1", "--account", "WISE", "--value", "1"},
		"category": {"tx", "e", "1", "--category", "FOOD"},
	} {
		if _, stderr, status := finux(t, home, args...); status != 1 || !strings.HasPrefix(stderr, "finux: ") {
			t.Errorf("transfer accepted a change of %s: status %d, stderr %q", name, status, stderr)
		}
	}
	fail(t, home, 2, "-to", "tx", "e", "1", "--to", "WISE")
	fail(t, home, 2, "--to", "tx", "add", "--title", "X", "--value", "1", "--account", "INTER", "--type", "transfer")
	fail(t, home, 2, "only a transfer", "tx", "add", "--title", "X", "--value", "1", "--account", "INTER", "--category", "FOOD", "--type", "expense", "--to", "NU")
	fail(t, home, 1, "two different accounts", "tx", "add", "--title", "X", "--value", "1", "--account", "INTER", "--to", "INTER")

	if got := succeed(t, home, "tx", "d", "2", "--yes"); got != "✓ Transfer deleted: transactions 2 and 1\n" {
		t.Fatalf("delete output = %q", got)
	}
	if got := succeed(t, home, "tx"); !strings.Contains(got, "\n2 transactions\n") || strings.Contains(got, "Card") {
		t.Fatalf("list after delete:\n%s", got)
	}
	if balance(t, home, "INTER") != "R$ 450,00" || balance(t, home, "NU") != "R$ 0,00" {
		t.Fatalf("balances after delete = %s, %s", balance(t, home, "INTER"), balance(t, home, "NU"))
	}
}

func TestTransactionsFilters(t *testing.T) {
	home := ledger(t)
	add := func(title, value, accountCode, categoryCode, date, tags string) {
		t.Helper()
		succeed(t, home, "tx", "add", "--title", title, "--value", value, "--account", accountCode, "--category", categoryCode, "--date", date, "--tags", tags)
	}
	add("Weekly market", "50", "INTER", "FOOD", "17/11/2026", "#market #weekly")
	add("Salary", "3000", "INTER", "SAL", "05/11/2026", "")
	add("Rent", "900", "NU", "HOME", "01/10/2026", "#fixed")
	add("Night market", "20", "NU", "FOOD", "30/12/2025", "#market")
	succeed(t, home, "tx", "add", "--title", "Card payment", "--value", "200", "--account", "INTER", "--to", "NU", "--date", "10/11/2026")

	for name, tc := range map[string]struct {
		args []string
		want string // ids in order
	}{
		"category":           {[]string{"--category", "food"}, "1 4"},
		"tag":                {[]string{"--tag", "market"}, "1 4"},
		"tag with sign":      {[]string{"--tag", "#Weekly"}, "1"},
		"two tags":           {[]string{"--tag", "market weekly"}, "1"},
		"text":               {[]string{"--search", "MARKET"}, "1 4"},
		"year":               {[]string{"--date", "2025"}, "4"},
		"month":              {[]string{"--date", "11/2026"}, "1 6 5 2"},
		"month, other order": {[]string{"--date", "2026/11"}, "1 6 5 2"},
		"day":                {[]string{"--date", "01/10/2026"}, "3"},
		"day, other order":   {[]string{"--date", "2026/10/01"}, "3"},
		"account":            {[]string{"--account", "nu"}, "6 3 4"},
		"type":               {[]string{"--type", "transfer"}, "6 5"},
		"type income":        {[]string{"l", "--type", "income"}, "2"},
		"all":                {[]string{"list", "--category", "FOOD", "--tag", "market", "--search", "night", "--date", "2025", "--account", "NU", "--type", "expense"}, "4"},
	} {
		var ids []string
		for _, line := range strings.Split(succeed(t, home, append([]string{"tx"}, tc.args...)...), "\n")[1:] {
			if fields := strings.Fields(line); len(fields) > 2 {
				ids = append(ids, fields[0])
			}
		}
		if got := strings.Join(ids, " "); got != tc.want {
			t.Errorf("%s: ids %q, want %q", name, got, tc.want)
		}
	}
	if got := succeed(t, home, "tx", "--category", "TAX"); got != "No transactions match the filter.\n" {
		t.Fatalf("no match = %q", got)
	}
	fail(t, home, 1, "category not found: NOPE", "tx", "--category", "NOPE")
	fail(t, home, 1, "account not found: NOPE", "tx", "--account", "NOPE")
	fail(t, home, 2, "unknown type", "tx", "--type", "loan")
	fail(t, home, 2, "invalid date", "tx", "--date", "13/2026")
	fail(t, home, 2, "invalid tag", "tx", "--tag", "a-b")
}

func TestTransactionsUsageErrors(t *testing.T) {
	home := ledger(t)
	succeed(t, home, "tx", "add", "--title", "Market", "--value", "50", "--account", "INTER", "--category", "FOOD")
	base := []string{"tx", "add", "--title", "X", "--value", "1", "--account", "INTER", "--category", "FOOD"}
	with := func(extra ...string) []string { return append(append([]string{}, base...), extra...) }
	for name, tc := range map[string]struct {
		want string
		args []string
	}{
		"unknown flag":             {"-currency", with("--currency", "BRL")},
		"unknown flag alone":       {"-nope", []string{"tx", "--nope"}},
		"add with nothing":         {"--title", []string{"tx", "a"}},
		"add without title":        {"--title", []string{"tx", "add", "--value", "1", "--account", "INTER", "--category", "FOOD"}},
		"add without value":        {"--value", []string{"tx", "add", "--title", "X", "--account", "INTER", "--category", "FOOD"}},
		"add without account":      {"--account", []string{"tx", "add", "--title", "X", "--value", "1", "--category", "FOOD"}},
		"add without category":     {"--category", []string{"tx", "add", "--title", "X", "--value", "1", "--account", "INTER"}},
		"add empty title":          {"title", with("--title", " ")},
		"add long title":           {"too long", with("--title", strings.Repeat("x", 61))},
		"add long description":     {"too long", with("--description", strings.Repeat("x", 201))},
		"add bad value":            {"abc", with("--value", "abc")},
		"add zero value":           {"more than zero", with("--value", "0")},
		"add negative value":       {"more than zero", with("--value", "-5")},
		"add bad date":             {"invalid date", with("--date", "31/02/2026")},
		"add bad tag":              {"invalid tag", with("--tags", "#a-b")},
		"add unknown type":         {"unknown type", with("--type", "loan")},
		"add grey without type":    {"--type", []string{"tx", "add", "--title", "X", "--value", "1", "--account", "INTER", "--category", "OTHER"}},
		"add received, no to":      {"--received", with("--received", "5")},
		"add with argument":        {"unexpected argument", with("extra")},
		"reference with argument":  {"unexpected argument", []string{"tx", "1", "extra"}},
		"show without reference":   {"id", []string{"tx", "show"}},
		"edit without reference":   {"id", []string{"tx", "edit", "--title", "X"}},
		"edit without options":     {"--title", []string{"tx", "edit", "1"}},
		"edit empty title":         {"title", []string{"tx", "edit", "1", "--title", ""}},
		"edit bad value":           {"abc", []string{"tx", "e", "1", "--value", "abc"}},
		"edit bad date":            {"invalid date", []string{"tx", "e", "1", "--date", "2026"}},
		"edit received":            {"-received", []string{"tx", "e", "1", "--received", "5"}},
		"delete without yes":       {"--yes", []string{"tx", "delete", "1"}},
		"delete without reference": {"id", []string{"tx", "d", "--yes"}},
		"list with argument":       {"unexpected argument", []string{"tx", "list", "extra"}},
	} {
		t.Run(name, func(t *testing.T) {
			fail(t, home, 2, tc.want, tc.args...)
			_, stderr, _ := finux(t, home, tc.args...)
			if !strings.HasPrefix(stderr, "finux: transactions: ") || !strings.HasSuffix(stderr, "\nRun 'finux transactions --help' for usage.\n") {
				t.Fatalf("stderr has no module name or usage hint: %q", stderr)
			}
		})
	}
	fail(t, home, 1, "account not found: NOPE", "tx", "add", "--title", "X", "--value", "1", "--account", "NOPE", "--category", "FOOD")
	fail(t, home, 1, "category not found: NOPE", "tx", "add", "--title", "X", "--value", "1", "--account", "INTER", "--category", "NOPE")
	fail(t, home, 1, "SAL", "tx", "add", "--title", "X", "--value", "1", "--account", "INTER", "--category", "SAL", "--type", "expense")
	// An archived account takes no new transactions.
	succeed(t, home, "ac", "d", "NU", "--yes")
	fail(t, home, 1, "account not found: NU", "tx", "add", "--title", "X", "--value", "1", "--account", "NU", "--category", "FOOD")
	if got := succeed(t, home, "tx"); !strings.Contains(got, "\n1 transaction\n") {
		t.Fatalf("failed commands changed data:\n%s", got)
	}
}

func TestTransactionsHelp(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	want := succeed(t, home, "transactions", "help")
	for _, text := range []string{"Usage: finux transactions [command]", "finux tx [command]", "<id>", "list, l", "add, a", "edit, e", "delete, d", "show",
		"--title", "--description", "--value", "--type", "--category", "--tags", "--account", "--date", "--to", "--received", "--yes",
		"--tag", "--search", "expense, income, transfer", "DD/MM/YYYY", "YYYY/MM/DD", "11/2026", "#tag", "@ACCOUNT", "cat:CODE"} {
		if !strings.Contains(want, text) {
			t.Errorf("help lacks %q:\n%s", text, want)
		}
	}
	for _, args := range [][]string{{"tx", "--help"}, {"tx", "-h"}, {"tx", "help"}, {"transactions", "-h"}, {"tx", "add", "--help"}, {"tx", "a", "help"}, {"tx", "l", "help"}} {
		if got := succeed(t, home, args...); got != want {
			t.Errorf("finux %v differs from 'finux transactions help'", args)
		}
	}
}

func TestDeleteAccountWithTransactions(t *testing.T) {
	home := ledger(t)
	succeed(t, home, "tx", "add", "--title", "Market", "--value", "50", "--account", "INTER", "--category", "FOOD")
	succeed(t, home, "tx", "add", "--title", "Salary", "--value", "3000", "--account", "INTER", "--category", "SAL")
	succeed(t, home, "tx", "add", "--title", "Coffee", "--value", "5", "--account", "NU", "--category", "FOOD")
	succeed(t, home, "tx", "add", "--title", "Card payment", "--value", "200", "--account", "INTER", "--to", "NU", "--description", "monthly")
	succeed(t, home, "tx", "add", "--title", "Back home", "--value", "10", "--account", "WISE", "--to", "INTER", "--received", "55")

	// Archive keeps the transactions.
	succeed(t, home, "ac", "d", "INTER", "--yes")
	if got := succeed(t, home, "tx"); !strings.Contains(got, "\n7 transactions\n") {
		t.Fatalf("archive changed the transactions:\n%s", got)
	}
	if got := succeed(t, home, "ac", "d", "INTER", "--hard", "--yes"); got != "✓ Account INTER deleted permanently, with 2 transactions and 2 transfers\n" {
		t.Fatalf("hard delete output = %q", got)
	}
	list := succeed(t, home, "tx")
	if !strings.Contains(list, "\n3 transactions\n") || strings.Contains(list, "Market") || strings.Contains(list, "INTER") {
		t.Fatalf("transactions after the delete:\n%s", list)
	}
	// The other side of each transfer stays, as an income or an expense.
	income := succeed(t, home, "tx", "5")
	for _, want := range []string{"Card payment\n", "monthly. Transfer from INTER (deleted account)\n", "Type      income\n", "Value     R$ 200,00\n", "Category  OTHER  Other\n", "Account   NU  Nubank\n"} {
		if !strings.Contains(income, want) {
			t.Errorf("incoming side lacks %q:\n%s", want, income)
		}
	}
	expense := succeed(t, home, "tx", "6")
	for _, want := range []string{"Transfer to INTER (deleted account)\n", "Type      expense\n", "Value     -$10.00\n", "Category  OTHER  Other\n"} {
		if !strings.Contains(expense, want) {
			t.Errorf("outgoing side lacks %q:\n%s", want, expense)
		}
	}
	if strings.Contains(income+expense, "Transfer  ") {
		t.Fatalf("a kept side still points to a transfer:\n%s\n%s", income, expense)
	}
	if balance(t, home, "NU") != "R$ 195,00" || balance(t, home, "WISE") != "$40.00" {
		t.Fatalf("balances changed: %s, %s", balance(t, home, "NU"), balance(t, home, "WISE"))
	}
	// An account without transactions has the short message.
	succeed(t, home, "ac", "add", "--name", "Empty", "--currency", "EUR")
	if got := succeed(t, home, "ac", "d", "EMPTY", "--hard", "--yes"); got != "✓ Account EMPTY deleted permanently\n" {
		t.Fatalf("delete of empty account = %q", got)
	}
}

func TestDeleteCategoryWithTransactions(t *testing.T) {
	home := ledger(t)
	succeed(t, home, "tx", "add", "--title", "Market", "--value", "50", "--account", "INTER", "--category", "FOOD")
	succeed(t, home, "tx", "add", "--title", "Lunch", "--value", "20", "--account", "INTER", "--category", "FOOD")
	succeed(t, home, "tx", "add", "--title", "Salary", "--value", "3000", "--account", "INTER", "--category", "SAL")

	// A color that the transactions do not accept is refused.
	fail(t, home, 1, "2 transactions", "ct", "e", "FOOD", "--color", "green")
	succeed(t, home, "ct", "e", "FOOD", "--color", "grey")
	succeed(t, home, "ct", "e", "FOOD", "--color", "red", "--name", "Groceries")

	if got := succeed(t, home, "ct", "d", "FOOD", "--yes"); got != "✓ Category FOOD deleted; 2 transactions moved to OTHER\n" {
		t.Fatalf("delete output = %q", got)
	}
	if got := succeed(t, home, "tx", "--category", "OTHER"); !strings.Contains(got, "Market") || !strings.Contains(got, "Lunch") || !strings.Contains(got, "\n2 transactions\n") {
		t.Fatalf("OTHER after the delete:\n%s", got)
	}
	if got := succeed(t, home, "ct", "d", "SAL", "--yes"); got != "✓ Category SAL deleted; 1 transaction moved to OTHER\n" {
		t.Fatalf("delete output = %q", got)
	}
	// OTHER receives transactions, so it stays while it has some.
	fail(t, home, 1, "OTHER", "ct", "d", "OTHER", "--yes")
	fail(t, home, 1, "1 transaction", "ct", "e", "OTHER", "--color", "red")
	if got := succeed(t, home, "ct", "d", "TAX", "--yes"); got != "✓ Category TAX deleted\n" {
		t.Fatalf("delete of unused category = %q", got)
	}
	// Without OTHER a category with transactions cannot go.
	succeed(t, home, "tx", "add", "--title", "Bus", "--value", "5", "--account", "INTER", "--category", "TRANS")
	for _, id := range []string{"1", "2", "3"} {
		succeed(t, home, "tx", "d", id, "--yes")
	}
	succeed(t, home, "ct", "d", "OTHER", "--yes")
	fail(t, home, 1, "Run 'finux ct add --name Other --code OTHER --color grey' to add it", "ct", "d", "TRANS", "--yes")
	succeed(t, home, "ct", "add", "--name", "Other", "--code", "OTHER", "--color", "grey")
	succeed(t, home, "ct", "d", "TRANS", "--yes")
}
