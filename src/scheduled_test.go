package main

import (
	"strings"
	"testing"
)

// The tests use the database of newCardsHome: the account NU in BRL with
// R$ 5.000,00 and the credit card VISA (closing day 25, due day 5). The
// date of today is 17/11/2026.

func TestScheduledLifecycle(t *testing.T) {
	h := newCardsHome(t)
	if got := h.ok("sc"); got != "No scheduled transactions yet. Run 'finux sc add'.\n" {
		t.Fatalf("empty list = %q", got)
	}
	if got := h.ok("scheduled", "add", "--title", "Rent", "--value", "1500", "--account", "nu", "--category", "home", "--every", "month", "--date", "05/11/2026", "--code", "rent", "--tags", "fixed", "--description", "Apartment"); got != "✓ Schedule RENT created\n" {
		t.Fatalf("add output = %q", got)
	}
	// One time, on a credit card, with a code from the title.
	if got := h.ok("sc", "a", "--title", "Dentist", "--value", "300", "--account", "VISA", "--category", "HEALT", "--date", "10/12/2026"); got != "✓ Schedule DENTI created\n" {
		t.Fatalf("add output = %q", got)
	}
	h.ok("sc", "a", "--title", "Salary", "--value", "4000", "--account", "NU", "--category", "SAL", "--every", "month", "--date", "30/11/2026", "--times", "3", "--code", "SAL")

	list := h.ok("sc")
	h.has(list, "CODE   TITLE    REPEATS      NEXT        STATUS    ACCOUNT         VALUE\n",
		"DENTI  Dentist  one time     10/12/2026  upcoming  VISA       -R$ 300,00\n",
		"RENT   Rent     every month  05/11/2026  due       NU       -R$ 1.500,00\n",
		"SAL    Salary   every month  30/11/2026  upcoming  NU        R$ 4.000,00\n", "3 schedules\n")
	if got := h.ok("schedule", "list"); got != list {
		t.Fatalf("schedule list = %q", got)
	}
	details := h.ok("sc", "RENT")
	h.has(details, "RENT  Rent\nApartment\n", "Type        expense\n", "Value       R$ 1.500,00\n", "Account     NU  Nubank\n", "Category    HOME  Housing\n",
		"Tags        #fixed\n", "Repeats     every month\n", "Confirm     manual\n", "Status      due\n", "Due         05/11/2026\n", "Next        05/12/2026\n",
		"No transactions yet.\n")
	if got := h.ok("sc", "show", "rent"); got != details {
		t.Fatalf("show = %q", got)
	}
	h.has(h.ok("sc", "DENTI"), "Card        VISA  Visa Gold\n", "Repeats     one time\n", "Status      upcoming\n", "Next        10/12/2026\n")
	h.has(h.ok("sc", "SAL"), "Type        income\n", "Times left  3\n")
	// A schedule is not a transaction: the balance does not change.
	h.has(h.ok("ac", "NU"), "R$ 5.000,00")
	if got := h.ok("tx"); !strings.HasPrefix(got, "No transactions yet.") {
		t.Fatalf("tx = %q", got)
	}

	// Pay creates the transaction with the date and the value of the occurrence.
	if got := h.ok("sc", "pay", "RENT"); got != "✓ Schedule RENT paid: transaction 1 created (05/11/2026, -R$ 1.500,00)\nNext: 05/12/2026\n" {
		t.Fatalf("pay output = %q", got)
	}
	h.has(h.ok("tx", "1"), "Rent\nApartment\n", "Value     -R$ 1.500,00\n", "Account   NU  Nubank\n", "Category  HOME  Housing\n", "Date      05/11/2026\n", "Tags      #fixed\n", "Schedule  RENT  Rent\n")
	h.has(h.ok("ac", "NU"), "R$ 3.500,00")
	h.has(h.ok("sc", "RENT"), "Status      upcoming\n", "Next        05/12/2026\n", "1   05/11/2026  Rent")
	// An occurrence can be paid before its date, with another value, date and account.
	h.ok("ac", "a", "--name", "Wallet", "--currency", "BRL", "--code", "CASH", "--balance", "2000")
	if got := h.ok("sc", "p", "RENT", "--value", "1550", "--date", "16/11/2026", "--account", "CASH"); got != "✓ Schedule RENT paid: transaction 2 created (16/11/2026, -R$ 1.550,00)\nNext: 05/01/2027\n" {
		t.Fatalf("early pay output = %q", got)
	}
	h.has(h.ok("ac", "CASH"), "R$ 450,00")
	if got := h.ok("sc", "confirm", "SAL"); got != "✓ Schedule SAL received: transaction 3 created (17/11/2026, R$ 4.000,00)\nNext: 30/12/2026\n" {
		t.Fatalf("income output = %q", got)
	}
	if got := h.ok("sc", "skip", "SAL"); got != "✓ Schedule SAL skipped: 30/12/2026\nNext: 30/01/2027\n" {
		t.Fatalf("skip output = %q", got)
	}
	h.has(h.ok("sc", "SAL"), "Times left  1\n")
	if got := h.ok("sc", "pay", "DENTI", "--date", "10/12/2026"); got != "✓ Schedule DENTI paid: transaction 4 created (10/12/2026, -R$ 300,00)\nNo occurrence is left.\n" {
		t.Fatalf("one-time output = %q", got)
	}
	h.has(h.ok("tx", "4"), "Card      VISA  Visa Gold\n", "Bill      01/2027\n")
	h.fail(1, "schedule DENTI has no occurrence left", "sc", "pay", "DENTI")
	h.fail(1, "schedule DENTI has no occurrence left", "sc", "skip", "DENTI")
	// The list leaves out a schedule that is done.
	if list = h.ok("sc"); strings.Contains(list, "DENTI") || !strings.Contains(list, "2 schedules\n") {
		t.Fatalf("list with a done schedule:\n%s", list)
	}
	h.has(h.ok("sc", "--all"), "DENTI  Dentist  one time     -           done ", "3 schedules\n")

	h.has(h.ok("sc", "--help"), "Usage: finux scheduled", "pay", "skip", "--every <period>", "--times <number>", "--auto")
	h.fail(1, "schedule not found: NONE", "sc", "NONE")
	h.fail(2, "a schedule code or id is necessary", "sc", "pay")
}

func TestScheduledAddChecks(t *testing.T) {
	h := newCardsHome(t)
	h.ok("ac", "a", "--name", "Wise", "--currency", "USD", "--code", "WISE")
	h.ok("gl", "add", "--title", "Trip", "--type", "save", "--target", "1000", "--currency", "USD", "--code", "TRIP")
	base := []string{"sc", "add", "--title", "Rent", "--value", "10", "--account", "NU", "--category", "HOME"}
	h.fail(2, "add: --title, --value and --category are necessary", "sc", "add", "--title", "Rent")
	h.fail(2, "add: --type is necessary: the category OTHER is for all types", "sc", "add", "--title", "Rent", "--value", "10", "--account", "NU", "--category", "OTHER")
	for want, extra := range map[string][]string{
		"invalid period":                     {"--every", "fortnight"},
		"invalid date":                       {"--date", "31/02/2026"},
		"--times must be a number from 1 to": {"--every", "month", "--times", "0"},
		"only for a schedule that repeats":   {"--times", "3"},
		"only for a schedule that repeats\n": {"--until", "31/12/2027"},
		"end date is before":                 {"--every", "month", "--date", "05/12/2026", "--until", "01/12/2026"},
		"unknown type \"transfer\"":          {"--type", "transfer"},
		"category HOME is for expense":       {"--type", "income"},
		"goal TRIP is in USD, not in BRL":    {"--goal", "TRIP"},
		"goal not found: NOPE":               {"--goal", "NOPE"},
		"account not found: NONE":            {"--account", "NONE"},
		"category not found: NONE":           {"--category", "NONE"},
		"the value must be more than zero":   {"--value", "0"},
	} {
		stdout, stderr, status := h.run(append(append([]string{}, base...), extra...)...)
		if status == 0 || stdout != "" || !strings.Contains(stderr, strings.TrimSpace(want)) {
			t.Errorf("%v: status %d, stdout %q, stderr %q; want %q", extra, status, stdout, stderr, want)
		}
	}
	h.ok(append(append([]string{}, base...), "--code", "RENT")...)
	h.fail(1, "schedule code is already in use: RENT", append(append([]string{}, base...), "--code", "RENT")...)
	// The date of a new schedule is today when no date is given.
	h.has(h.ok("sc", "RENT"), "Status      due\n", "Due         17/11/2026\n")
	h.ok("sc", "a", "--title", "Fund", "--value", "50", "--account", "WISE", "--category", "OTHER", "--type", "expense", "--goal", "TRIP", "--every", "2 weeks")
	h.has(h.ok("sc", "FUND"), "Goal        TRIP  Trip\n", "Repeats     every 2 weeks\n", "Value       $50.00\n")
}

func TestScheduledEditDeleteRestore(t *testing.T) {
	h := newCardsHome(t)
	h.ok("sc", "a", "--title", "Rent", "--value", "1500", "--account", "NU", "--category", "HOME", "--every", "month", "--date", "05/10/2026", "--times", "12", "--code", "RENT")
	h.ok("sc", "pay", "RENT")
	h.ok("sc", "skip", "RENT")
	if got := h.ok("sc", "edit", "RENT", "--title", "Home rent", "--value", "1600", "--category", "BILLS", "--tags", "home", "--code", "HOME", "--description", "New"); got != "✓ Schedule HOME updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	h.has(h.ok("sc", "HOME"), "HOME  Home rent\nNew\n", "Value       R$ 1.600,00\n", "Category    BILLS  Bills\n", "Tags        #home\n", "Times left  10\n", "Next        05/12/2026\n")
	// A new date is the date of the next occurrence; the others follow it.
	h.ok("sc", "e", "HOME", "--date", "10/12/2026")
	h.has(h.ok("sc", "HOME"), "Next        10/12/2026\n", "Times left  10\n")
	h.ok("sc", "pay", "HOME", "--date", "10/12/2026")
	h.has(h.ok("sc", "HOME"), "Next        10/01/2027\n", "Times left  9\n")
	// A new period counts from the next occurrence.
	h.ok("sc", "e", "HOME", "--every", "2 weeks", "--until", "31/01/2027", "--times", "none")
	details := h.ok("sc", "HOME")
	h.has(details, "Repeats     every 2 weeks\n", "Until       31/01/2027\n", "Next        10/01/2027\n", "Times left  2\n")
	h.ok("sc", "e", "HOME", "--every", "none", "--until", "none")
	h.has(h.ok("sc", "HOME"), "Repeats     one time\n", "Next        10/01/2027\n")
	h.ok("sc", "e", "HOME", "--account", "VISA", "--auto")
	h.has(h.ok("sc", "HOME"), "Card        VISA  Visa Gold\n", "Confirm     automatic (off in 'finux config')\n")
	h.ok("sc", "e", "HOME", "--auto=false")
	h.has(h.ok("sc", "HOME"), "Confirm     manual\n")
	h.fail(2, "edit: give --title", "sc", "edit", "HOME")
	h.fail(2, "category SAL is for income", "sc", "edit", "HOME", "--category", "SAL")
	h.ok("ac", "a", "--name", "Wise", "--currency", "USD", "--code", "WISE")
	h.fail(2, "edit: give --value: the account WISE is in USD, not in BRL", "sc", "edit", "HOME", "--account", "WISE")

	h.fail(2, "delete: --yes is necessary", "sc", "delete", "HOME")
	if got := h.ok("sc", "d", "HOME", "--yes"); got != "✓ Schedule HOME archived\n" {
		t.Fatalf("archive output = %q", got)
	}
	if got := h.ok("sc"); got != "No scheduled transactions yet. Run 'finux sc add'.\n" {
		t.Fatalf("list after archive = %q", got)
	}
	h.has(h.ok("sc", "--all"), "Home rent (archived)")
	h.fail(1, "schedule not found: HOME (it is archived; restore it first)", "sc", "pay", "HOME")
	h.fail(1, "schedule not found: HOME (it is archived; restore it first)", "sc", "edit", "HOME", "--title", "X")
	if got := h.ok("sc", "restore", "HOME"); got != "✓ Schedule HOME restored\n" {
		t.Fatalf("restore output = %q", got)
	}
	h.fail(1, "schedule HOME is not archived", "sc", "r", "HOME")
	if got := h.ok("sc", "delete", "HOME", "--hard", "--yes"); got != "✓ Schedule HOME deleted permanently\n" {
		t.Fatalf("delete output = %q", got)
	}
	// The transactions of a deleted schedule stay, without the link.
	if got := h.ok("tx", "1"); strings.Contains(got, "Schedule") || !strings.Contains(got, "Rent\n") {
		t.Fatalf("transaction after the delete:\n%s", got)
	}
}

func TestScheduledAutomatic(t *testing.T) {
	h := newCardsHome(t)
	// The setting is off: a schedule with --auto waits as the others, and
	// Finux tells how to turn it on.
	want := "✓ Schedule NET created\n! Automatic schedules are off, so NET waits for your confirmation. Run 'finux config set automatic-schedules on' to turn them on.\n"
	if got := h.ok("sc", "a", "--title", "Internet", "--value", "99,90", "--account", "NU", "--category", "BILLS", "--every", "month", "--date", "10/10/2026", "--auto", "--code", "NET"); got != want {
		t.Fatalf("add --auto with the setting off = %q", got)
	}
	if got := h.ok("tx"); !strings.HasPrefix(got, "No transactions yet.") || !strings.Contains(got, "! 2 scheduled transactions are due.") {
		t.Fatalf("an automatic schedule ran with the setting off:\n%s", got)
	}
	h.has(h.ok("sc", "NET"), "Confirm     automatic (off in 'finux config')\n", "Status      2 due\n")
	h.ok("config", "set", "automatic-schedules", "on")
	// The next command creates the transactions of the automatic schedule
	// that are due, with their dates, and tells them.
	want = "✓ Schedule NET paid: transaction 1 created (10/10/2026, -R$ 99,90)\n✓ Schedule NET paid: transaction 2 created (10/11/2026, -R$ 99,90)\n✓ Schedule RENT created\n"
	if got := h.ok("sc", "a", "--title", "Rent", "--value", "1500", "--account", "NU", "--category", "HOME", "--every", "month", "--date", "05/11/2026", "--code", "RENT"); got != want {
		t.Fatalf("output of the next command = %q", got)
	}
	// The manual schedule waits.
	out := h.ok("tx")
	h.has(out, "2   10/11/2026  Internet", "2 transactions\n", "! 1 scheduled transaction is due. Run 'finux sc'.\n")
	if strings.Contains(out, "Rent") || strings.Contains(out, "paid") {
		t.Fatalf("a manual schedule became a transaction:\n%s", out)
	}
	// It does it one time only.
	if again := h.ok("tx"); strings.Contains(again, "paid") || !strings.Contains(again, "2 transactions\n") {
		t.Fatalf("second run:\n%s", again)
	}
	h.has(h.ok("sc", "NET"), "Confirm     automatic\n", "Status      upcoming\n", "Next        10/12/2026\n")
	h.has(h.ok("ac", "NU"), "R$ 4.800,20")

	// An automatic occurrence that fails stays due, and the command still works.
	h.ok("sc", "a", "--title", "Gym", "--value", "80", "--account", "VISA", "--category", "HEALT", "--every", "month", "--date", "20/10/2026", "--code", "GYM")
	h.ok("tx", "a", "--title", "Chair", "--value", "100", "--account", "VISA", "--category", "SHOP", "--date", "20/10/2026")
	h.ok("cc", "pay", "VISA", "--from", "NU", "--bill", "11/2026", "--date", "17/11/2026")
	h.ok("sc", "e", "GYM", "--auto")
	out = h.ok("ac")
	h.has(out, "! Schedule GYM is automatic, but its occurrence of 20/10/2026 failed: bill 11/2026 of VISA is paid: delete its payment first\n", "NU")
	h.has(h.ok("sc", "--all"), "GYM", "due")
	h.ok("sc", "skip", "GYM")
	h.has(h.ok("tx"), "! 1 scheduled transaction is due. Run 'finux sc'.\n")
}

func TestScheduledFollowsDeletes(t *testing.T) {
	h := newCardsHome(t)
	h.ok("ac", "a", "--name", "Wallet", "--currency", "BRL", "--code", "CASH")
	h.ok("gl", "add", "--title", "Trip", "--type", "save", "--target", "1000", "--currency", "BRL", "--code", "TRIP")
	h.ok("ct", "add", "--name", "Pets", "--color", "red", "--code", "PETS")
	h.ok("sc", "a", "--title", "Vet", "--value", "100", "--account", "CASH", "--category", "PETS", "--goal", "TRIP", "--date", "01/12/2026", "--code", "VET")
	h.ok("sc", "a", "--title", "Food", "--value", "50", "--account", "VISA", "--category", "PETS", "--date", "01/12/2026", "--code", "FOOD")

	// A category in a schedule cannot go.
	h.fail(1, "category PETS is in the schedules FOOD, VET: change their category or delete them first", "ct", "delete", "PETS", "--yes")
	h.ok("gl", "delete", "TRIP", "--hard", "--yes")
	if got := h.ok("sc", "VET"); strings.Contains(got, "Goal") {
		t.Fatalf("schedule keeps a deleted goal:\n%s", got)
	}
	// A schedule goes with its account or its card.
	h.ok("ac", "delete", "CASH", "--hard", "--yes")
	h.ok("cc", "delete", "VISA", "--hard", "--yes")
	if got := h.ok("sc", "--all"); got != "No scheduled transactions yet. Run 'finux sc add'.\n" {
		t.Fatalf("schedules after the deletes:\n%s", got)
	}
	h.ok("ct", "delete", "PETS", "--yes")
}

func TestScheduledInResumeAndBudgets(t *testing.T) {
	h := newCardsHome(t)
	h.ok("ac", "a", "--name", "Wise", "--currency", "USD", "--code", "WISE", "--balance", "100")
	h.ok("sc", "a", "--title", "Rent", "--value", "1500", "--account", "NU", "--category", "HOME", "--every", "month", "--date", "05/10/2026", "--code", "RENT")
	h.ok("sc", "a", "--title", "Internet", "--value", "100", "--account", "NU", "--category", "BILLS", "--every", "month", "--date", "17/11/2026", "--code", "NET")
	h.ok("sc", "a", "--title", "Salary", "--value", "4000", "--account", "NU", "--category", "SAL", "--every", "month", "--date", "30/11/2026", "--code", "SAL")
	h.ok("sc", "a", "--title", "Gym", "--value", "80", "--account", "VISA", "--category", "HEALT", "--every", "week", "--date", "20/11/2026", "--code", "GYM")
	h.ok("sc", "a", "--title", "Dentist", "--value", "300", "--account", "NU", "--category", "HEALT", "--date", "10/12/2026", "--code", "DENT")
	h.ok("sc", "a", "--title", "Hosting", "--value", "20", "--account", "WISE", "--category", "BILLS", "--date", "25/11/2026", "--code", "HOST")
	h.ok("sc", "a", "--title", "Old", "--value", "999", "--account", "NU", "--category", "HOME", "--date", "20/11/2026", "--code", "OLD")
	h.ok("sc", "d", "OLD", "--yes")

	// The resume lists what is due and what comes until the end of the
	// month of today, and the balances after them. A credit card is not
	// in a balance.
	out := h.ok("resume")
	h.has(out, "Scheduled until 30/11/2026\n",
		"05/10/2026  RENT  Rent      overdue   NU       -R$ 1.500,00\n",
		"05/11/2026  RENT  Rent      overdue   NU       -R$ 1.500,00\n",
		"17/11/2026  NET   Internet  today     NU         -R$ 100,00\n",
		"20/11/2026  GYM   Gym       upcoming  VISA        -R$ 80,00\n",
		"25/11/2026  HOST  Hosting   upcoming  WISE          -$20.00\n",
		"27/11/2026  GYM   Gym       upcoming  VISA        -R$ 80,00\n",
		"30/11/2026  SAL   Salary    upcoming  NU        R$ 4.000,00\n",
		"After BRL  R$ 5.900,00\n", "After USD  $80.00\n")
	for _, absent := range []string{"DENT", "OLD"} {
		if strings.Contains(out, absent) {
			t.Errorf("resume shows %s:\n%s", absent, out)
		}
	}
	if order := strings.Index(out, "05/10/2026  RENT") < strings.Index(out, "17/11/2026  NET") && strings.Index(out, "Scheduled until") > strings.Index(out, "Accounts\n"); !order {
		t.Errorf("order of the resume:\n%s", out)
	}
	h.ok("sc", "pay", "RENT")
	h.ok("sc", "pay", "RENT")
	h.has(h.ok("resume"), "After BRL  R$ 5.900,00\n", "NU    Nubank  R$ 2.000,00\n")

	// A budget shows what the schedules of its categories still take from
	// its current period. An income gives back.
	h.ok("bg", "add", "--title", "Home", "--limit", "2000", "--currency", "BRL", "--categories", "HOME,BILLS,HEALT", "--every", "month", "--start", "01/10/2026", "--code", "HOME")
	h.has(h.ok("bg", "HOME"), "Spent       R$ 1.500,00 (75%)\n", "Scheduled   R$ 260,00\n")
	h.ok("bg", "add", "--title", "All", "--limit", "9000", "--currency", "BRL", "--every", "month", "--code", "ALL")
	h.has(h.ok("bg", "ALL"), "Scheduled   R$ 260,00\n")
	h.ok("bg", "add", "--title", "Food", "--limit", "500", "--currency", "BRL", "--categories", "FOOD", "--every", "month", "--code", "FOOD")
	if got := h.ok("bg", "FOOD"); strings.Contains(got, "Scheduled") {
		t.Fatalf("budget without schedules:\n%s", got)
	}
	// A period that is not the current one has no scheduled amount.
	if got := h.ok("bg", "show", "HOME", "--period", "15/10/2026"); strings.Contains(got, "Scheduled") {
		t.Fatalf("past period:\n%s", got)
	}
}

// The user does not always know which account will pay a bill. A
// schedule can wait without an account; the payment gives it.
func TestScheduledWithoutAccount(t *testing.T) {
	h := newCardsHome(t)
	if got := h.ok("sc", "add", "--title", "Energy", "--value", "187", "--category", "BILLS", "--every", "month", "--date", "06/10/2026", "--code", "ENERG"); got != "✓ Schedule ENERG created\n" {
		t.Fatalf("add output = %q", got)
	}
	h.has(h.ok("sc"), "ENERG  Energy  every month  06/10/2026  2 due   -        -R$ 187,00\n")
	h.has(h.ok("sc", "ENERG"), "Value       R$ 187,00\n", "Account     not decided\n")
	// It still counts in the list of what is due.
	h.has(h.ok("resume"), "Energy")

	h.fail(2, "pay: give --account: the schedule ENERG has no account", "sc", "pay", "ENERG")
	h.fail(2, "an automatic schedule needs an account", "sc", "edit", "ENERG", "--auto")
	h.fail(2, "an automatic schedule needs an account", "sc", "add", "--title", "Water", "--value", "50", "--category", "BILLS", "--auto")

	// The payment tells the account, or the credit card.
	if got := h.ok("sc", "pay", "ENERG", "--account", "NU"); got != "✓ Schedule ENERG paid: transaction 1 created (06/10/2026, -R$ 187,00)\nNext: 06/11/2026\n" {
		t.Fatalf("pay output = %q", got)
	}
	h.has(h.ok("tx", "1"), "Account   NU  Nubank\n", "Schedule  ENERG  Energy\n")
	if got := h.ok("sc", "pay", "ENERG", "--account", "VISA", "--value", "190,50"); got != "✓ Schedule ENERG paid: transaction 2 created (06/11/2026, -R$ 190,50)\nNext: 06/12/2026\n" {
		t.Fatalf("pay with a card output = %q", got)
	}
	h.has(h.ok("tx", "2"), "Card      VISA  Visa Gold\n")
	// The schedule itself stays without an account.
	h.has(h.ok("sc", "ENERG"), "Account     not decided\n")

	// An edit gives the account and takes it away again.
	h.ok("sc", "edit", "ENERG", "--account", "NU")
	h.has(h.ok("sc", "ENERG"), "Account     NU  Nubank\n")
	h.ok("sc", "edit", "ENERG", "--account", "none")
	h.has(h.ok("sc", "ENERG"), "Account     not decided\n", "Value       R$ 187,00\n")

	// With accounts in two currencies the schedule needs its currency.
	h.ok("ac", "a", "--name", "Wise", "--currency", "USD", "--code", "WISE", "--balance", "100")
	h.fail(2, "add: give --account or --currency: your accounts have more than one currency", "sc", "add", "--title", "Hosting", "--value", "12", "--category", "BILLS")
	h.fail(2, "unknown currency", "sc", "add", "--title", "Hosting", "--value", "12", "--category", "BILLS", "--currency", "XXX")
	h.fail(2, "add: give --account or --currency, not the two", "sc", "add", "--title", "Hosting", "--value", "12", "--category", "BILLS", "--currency", "USD", "--account", "WISE")
	h.ok("sc", "add", "--title", "Hosting", "--value", "12", "--category", "BILLS", "--currency", "usd", "--code", "HOST")
	h.has(h.ok("sc", "HOST"), "Value       $12.00\n", "Account     not decided\n")
	// An account in another currency needs the real value.
	h.fail(2, "pay: give --value: the account NU is in BRL, not in USD", "sc", "pay", "HOST", "--account", "NU")
	h.ok("sc", "pay", "HOST", "--account", "WISE")
	h.has(h.ok("ac", "WISE"), "$88.00")
}
