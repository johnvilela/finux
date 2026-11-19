package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// devBinary builds the CLI in development mode: its default database is
// database and it has the seed command.
func devBinary(t *testing.T, database string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "finux-dev")
	output, err := exec.Command("go", "build", "-ldflags", "-X 'main.devDatabase="+database+"'", "-o", binary, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build dev finux: %v\n%s", err, output)
	}
	return binary
}

func runBinary(t *testing.T, binary string, env []string, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Env = append([]string{"TERM=xterm-256color", "HOME=" + t.TempDir()}, env...)
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut
	if err := command.Run(); err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		status = exit.ExitCode()
	}
	return out.String(), errOut.String(), status
}

func TestSeedOnlyInDevBuild(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	fail(t, home, 2, "unsupported arguments: seed", "seed")
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("production build created a database for seed: %v", err)
	}
}

func TestDevBuildSeedsItsOwnDatabase(t *testing.T) {
	database := filepath.Join(t.TempDir(), "project root", "finux-dev.db")
	binary := devBinary(t, database)
	must := func(env []string, args ...string) string {
		t.Helper()
		stdout, stderr, status := runBinary(t, binary, env, args...)
		if status != 0 || stderr != "" {
			t.Fatalf("finux-dev %v: status %d, stderr %q, stdout %q", args, status, stderr, stdout)
		}
		return stdout
	}

	if got := must(nil); !strings.Contains(got, "\n  seed              Add sample data to the development database\n") {
		t.Fatalf("dev help lacks the seed command:\n%s", got)
	}
	created := "✓ Development database created: " + database + " (8 accounts, 2 credit cards, 3 goals, 3 budgets, 4 schedules, 30 transactions)\n"
	if got := must(nil, "seed"); got != created {
		t.Fatalf("first seed = %q, want %q", got, created)
	}
	if got := must(nil, "config", "doctor"); got != "✓ Database found: "+database+"\n✓ Migrations up to date\n" {
		t.Fatalf("doctor in dev build = %q", got)
	}
	first := must(nil, "ac", "--all")
	for _, want := range []string{"INTER", "NUBAN", "WISE", "BRL", "USD", "EUR", "BTC", "(archived)", "8 accounts\n", "-R$ "} {
		if !strings.Contains(first, want) {
			t.Errorf("seeded list lacks %q:\n%s", want, first)
		}
	}
	if categories := must(nil, "ct"); !strings.Contains(categories, "\n14 categories\n") {
		t.Fatalf("a new development database lacks the base categories:\n%s", categories)
	}
	if active := must(nil, "ac"); !strings.Contains(active, "7 accounts\n") {
		t.Fatalf("active seeded accounts:\n%s", active)
	}
	// The sample transactions have all types, tags and a transfer between
	// two currencies.
	transactions := must(nil, "tx")
	for _, want := range []string{"Salary", "Weekly market", "Card payment", "Travel money", "FOOD", "XFER", "-R$ 550,00", "$100.00", "Payment VISA", "Laptop 1/6", "Laptop 6/6", "\n30 transactions\n"} {
		if !strings.Contains(transactions, want) {
			t.Errorf("seeded transactions lack %q:\n%s", want, transactions)
		}
	}
	// The sample schedules: one that is due, two that come and one for one time.
	schedules := must(nil, "sc")
	for _, want := range []string{"RENT", "every month", "due", "NET", "upcoming", "DENT", "one time", "VISA", "SAL", "R$ 2.400,00", "4 schedules\n"} {
		if !strings.Contains(schedules, want) {
			t.Errorf("schedules lack %q:\n%s", want, schedules)
		}
	}
	// The sample goals have one of each type and of each status.
	goals := must(nil, "gl")
	for _, want := range []string{"TRIP", "save", "active", "24%", "CAR", "payoff", "overdue", "50%", "BIKE", "purchase", "reached", "100%", "3 goals\n"} {
		if !strings.Contains(goals, want) {
			t.Errorf("seeded goals lack %q:\n%s", want, goals)
		}
	}
	// The sample budgets are near the limit, over it after a rollover,
	// and not started.
	budgets := must(nil, "bg")
	for _, want := range []string{"FOOD", "every month", "near", "R$ 839,25", "83%", "FUN", "every 2 weeks", "over", "R$ 55,90", "155%", "R$ 36,00", "VACA", "one time", "upcoming", "3 budgets\n"} {
		if !strings.Contains(budgets, want) {
			t.Errorf("seeded budgets lack %q:\n%s", want, budgets)
		}
	}
	// The sample credit cards have an open bill, a late bill with the
	// rest of a partial payment, and a paid bill.
	cards := must(nil, "cc")
	for _, want := range []string{"VISA", "Visa Gold", "AMEX", "2 credit cards\n"} {
		if !strings.Contains(cards, want) {
			t.Errorf("seeded credit cards lack %q:\n%s", want, cards)
		}
	}
	visa := must(nil, "cc", "VISA")
	for _, want := range []string{"is late", "Open bill", "Streaming", "Total  R$ 277,80\n"} {
		if !strings.Contains(visa, want) {
			t.Errorf("seeded card details lack %q:\n%s", want, visa)
		}
	}
	bills := must(nil, "cc", "bills", "VISA")
	for _, want := range []string{"open", "late", "paid", "R$ 300,00", "3 bills\n"} {
		if !strings.Contains(bills, want) {
			t.Errorf("seeded bills lack %q:\n%s", want, bills)
		}
	}
	if tagged := must(nil, "tx", "--tag", "market"); !strings.Contains(tagged, "\n3 transactions\n") {
		t.Fatalf("seeded transactions with the tag #market:\n%s", tagged)
	}

	// A seed of a database that exists keeps its data and adds accounts
	// and transactions. CASH has one transaction, which goes with it.
	must(nil, "ac", "add", "--name", "Temporary", "--currency", "USD", "--code", "TEMP")
	must(nil, "ac", "d", "CASH", "--hard", "--yes")
	total := 8
	for _, want := range []string{
		"3 accounts and 5 transactions added; 11 accounts and 34 transactions in total",
		"3 accounts and 5 transactions added; 14 accounts and 39 transactions in total",
	} {
		total += 3
		if got := must(nil, "seed"); got != "✓ Development database updated: "+database+" ("+want+")\n" {
			t.Fatalf("seed of existing database = %q, want %q", got, want)
		}
		list := must(nil, "ac", "--all")
		if !strings.Contains(list, "TEMP ") || strings.Contains(list, "CASH ") || !strings.Contains(list, "INTER") {
			t.Fatalf("seed did not keep the data of the database:\n%s", list)
		}
		// One account was added by hand and one deleted, so the totals match.
		if !strings.HasSuffix(list, "\n"+strconv.Itoa(total)+" accounts\n") {
			t.Fatalf("list after seed, want %d accounts:\n%s", total, list)
		}
	}

	// --new-db starts again from the base data.
	if got := must(nil, "seed", "--new-db"); got != created {
		t.Fatalf("seed --new-db = %q, want %q", got, created)
	}
	if again := must(nil, "ac", "--all"); again != first {
		t.Fatalf("seed --new-db did not reset the database:\n%s\nwant\n%s", again, first)
	}
	if help := must(nil, "seed", "help"); !strings.Contains(help, "--new-db") {
		t.Fatalf("seed help lacks --new-db:\n%s", help)
	}
	if _, stderr, status := runBinary(t, binary, nil, "seed", "--bogus"); status != 2 || !strings.Contains(stderr, "-bogus") {
		t.Fatalf("seed --bogus: status %d, stderr %q", status, stderr)
	}
}

func TestDevBuildHonorsFinuxDB(t *testing.T) {
	database := filepath.Join(t.TempDir(), "finux-dev.db")
	other := filepath.Join(t.TempDir(), "other.db")
	binary := devBinary(t, database)
	env := []string{"FINUX_DB=" + other}
	if stdout, _, status := runBinary(t, binary, env, "init"); status != 0 || !strings.HasPrefix(stdout, "✓ Database created: "+other+"\n") {
		t.Fatalf("init with FINUX_DB = %q, status %d", stdout, status)
	}
	// Seed removes data, so it only works on the development database.
	if stdout, _, status := runBinary(t, binary, env, "seed"); status != 0 || !strings.Contains(stdout, database) {
		t.Fatalf("seed with FINUX_DB = %q, status %d; want the development database", stdout, status)
	}
	if stdout, _, _ := runBinary(t, binary, env, "ac"); stdout != "No accounts yet. Run 'finux ac add'.\n" {
		t.Fatalf("seed changed the FINUX_DB database:\n%s", stdout)
	}
	for _, args := range [][]string{{"seed", "help"}, {"seed", "--help"}, {"seed", "-h"}} {
		if stdout, _, status := runBinary(t, binary, nil, args...); status != 0 || !strings.Contains(stdout, "Usage: finux seed") {
			t.Fatalf("finux-dev %v = %q, status %d", args, stdout, status)
		}
	}
	if _, stderr, status := runBinary(t, binary, nil, "seed", "extra"); status != 2 || !strings.Contains(stderr, "unexpected argument: extra") {
		t.Fatalf("seed extra: status %d, stderr %q", status, stderr)
	}
}
