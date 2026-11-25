package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// finux runs the CLI with its database in home. Input and output are not
// terminals, so output is plain and interactive screens do not open.
func finux(t *testing.T, home string, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	return finuxEnv(t, []string{"FINUX_DB=" + filepath.Join(home, "finux.db"), "HOME=" + home}, args...)
}

func finuxEnv(t *testing.T, env []string, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	command := exec.Command(finuxBinary(t), args...)
	command.Env = append([]string{"TERM=xterm-256color"}, env...)
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut
	if err := command.Run(); err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run finux %v: %v", args, err)
		}
		status = exit.ExitCode()
	}
	return out.String(), errOut.String(), status
}

func succeed(t *testing.T, home string, args ...string) string {
	t.Helper()
	stdout, stderr, status := finux(t, home, args...)
	if status != 0 || stderr != "" {
		t.Fatalf("finux %v: status %d, stderr %q, stdout %q", args, status, stderr, stdout)
	}
	return stdout
}

func fail(t *testing.T, home string, wantStatus int, wantStderr string, args ...string) {
	t.Helper()
	stdout, stderr, status := finux(t, home, args...)
	if status != wantStatus || stdout != "" || !strings.Contains(stderr, wantStderr) || !strings.HasPrefix(stderr, "finux: ") {
		t.Fatalf("finux %v: status %d, stdout %q, stderr %q; want status %d and stderr with %q", args, status, stdout, stderr, wantStatus, wantStderr)
	}
}

func TestAccountsLifecycle(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	succeed(t, home, "init")
	const empty = "No accounts yet. Run 'finux ac add'.\n"
	if got := succeed(t, home, "ac"); got != empty {
		t.Fatalf("empty list = %q", got)
	}

	got := succeed(t, home, "accounts", "add", "--name", "Banco Inter", "--currency", "brl", "--balance", "1234,56", "--color", "orange", "--description", "main account")
	if got != "✓ Account BINTE created\n" {
		t.Fatalf("add output = %q", got)
	}
	if got := succeed(t, home, "ac", "a", "--name", "Nubank", "--currency", "BRL", "--code", "nu"); got != "✓ Account NU created\n" {
		t.Fatalf("add with code output = %q", got)
	}

	const table = "" +
		"CODE   NAME         CURRENCY      BALANCE\n" +
		"BINTE  Banco Inter  BRL       R$ 1.234,56\n" +
		"NU     Nubank       BRL           R$ 0,00\n" +
		"2 accounts\n"
	for _, args := range [][]string{{"ac"}, {"ac", "list"}, {"accounts", "l"}, {"accounts"}} {
		if got := succeed(t, home, args...); got != table {
			t.Fatalf("finux %v =\n%s\nwant\n%s", args, got, table)
		}
	}

	for _, args := range [][]string{{"ac", "BINTE"}, {"accounts", "binte"}, {"ac", "1"}, {"ac", "show", "BINTE"}, {"ac", "show", "1"}} {
		reference := strings.Join(args, " ")
		card := succeed(t, home, args...)
		for _, want := range []string{"BINTE  Banco Inter\n", "main account\n", "ID        1\n", "Currency  BRL\n", "Balance   R$ 1.234,56\n", "Color     orange\n", "Created   ", "Updated   "} {
			if !strings.Contains(card, want) {
				t.Fatalf("show %s lacks %q:\n%s", reference, want, card)
			}
		}
	}

	if got := succeed(t, home, "ac", "e", "BINTE", "--name", "Inter PJ", "--code", "intpj", "--color", "green", "--description", ""); got != "✓ Account INTPJ updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	card := succeed(t, home, "ac", "INTPJ")
	if !strings.Contains(card, "INTPJ  Inter PJ\n") || !strings.Contains(card, "Color     green\n") || strings.Contains(card, "main account") || !strings.Contains(card, "Balance   R$ 1.234,56\n") {
		t.Fatalf("after edit:\n%s", card)
	}
	fail(t, home, 1, "account not found: BINTE", "ac", "BINTE")
	fail(t, home, 1, "account not found: ls", "ac", "ls")

	if got := succeed(t, home, "ac", "delete", "NU", "--yes"); got != "✓ Account NU archived\n" {
		t.Fatalf("delete output = %q", got)
	}
	if got := succeed(t, home, "ac"); strings.Contains(got, "Nubank") || !strings.Contains(got, "1 account\n") {
		t.Fatalf("archived account in list:\n%s", got)
	}
	if got := succeed(t, home, "ac", "--all"); !strings.Contains(got, "Nubank (archived)") || !strings.Contains(got, "2 accounts\n") {
		t.Fatalf("list --all:\n%s", got)
	}
	if got := succeed(t, home, "ac", "show", "NU"); !strings.Contains(got, "Status    archived\n") {
		t.Fatalf("show archived:\n%s", got)
	}
	fail(t, home, 1, "already archived", "ac", "delete", "NU", "--yes")
	fail(t, home, 1, "already in use", "ac", "add", "--name", "Other", "--currency", "USD", "--code", "NU")
	fail(t, home, 1, "account not found: NU", "ac", "edit", "NU", "--name", "X")

	if got := succeed(t, home, "ac", "r", "nu"); got != "✓ Account NU restored\n" {
		t.Fatalf("restore output = %q", got)
	}
	fail(t, home, 1, "not archived", "ac", "restore", "NU")
	if got := succeed(t, home, "ac"); !strings.Contains(got, "Nubank") {
		t.Fatalf("restored account not in list:\n%s", got)
	}

	if got := succeed(t, home, "ac", "d", "--hard", "--yes", "NU"); got != "✓ Account NU deleted permanently\n" {
		t.Fatalf("hard delete output = %q", got)
	}
	fail(t, home, 1, "account not found: NU", "ac", "show", "NU")
	succeed(t, home, "ac", "delete", "INTPJ", "--yes")
	if got := succeed(t, home, "ac", "delete", "INTPJ", "--hard", "--yes"); got != "✓ Account INTPJ deleted permanently\n" {
		t.Fatalf("hard delete of archived account = %q", got)
	}
	if got := succeed(t, home, "ac", "list", "--all"); got != empty {
		t.Fatalf("list after deletes = %q", got)
	}
}

func TestAccountsUsageErrors(t *testing.T) {
	home := t.TempDir()
	succeed(t, home, "init")
	succeed(t, home, "ac", "add", "--name", "Wise", "--currency", "USD", "--balance", "-10.5")
	if got := succeed(t, home, "ac", "show", "WISE"); !strings.Contains(got, "Balance   -$10.50\n") {
		t.Fatalf("negative balance:\n%s", got)
	}
	for name, tc := range map[string]struct {
		want string
		args []string
	}{
		"reference with argument":   {"unexpected argument", []string{"ac", "WISE", "extra"}},
		"unknown flag alone":        {"-bogus", []string{"ac", "--bogus"}},
		"unknown flag":              {"-currency", []string{"ac", "edit", "WISE", "--currency", "BRL"}},
		"add without currency":      {"--currency", []string{"ac", "add", "--name", "X"}},
		"add without name":          {"--name", []string{"ac", "add", "--currency", "USD"}},
		"add with nothing":          {"--name", []string{"ac", "add"}},
		"add empty name":            {"name", []string{"ac", "add", "--name", " ", "--currency", "USD"}},
		"add unknown currency":      {"JPY", []string{"ac", "add", "--name", "X", "--currency", "JPY"}},
		"add bad balance":           {"abc", []string{"ac", "add", "--name", "X", "--currency", "USD", "--balance", "abc"}},
		"add unknown color":         {"magenta", []string{"ac", "add", "--name", "X", "--currency", "USD", "--color", "magenta"}},
		"add digits-only code":      {"at least one letter", []string{"ac", "add", "--name", "X", "--currency", "USD", "--code", "12345"}},
		"add long code":             {"too long", []string{"ac", "add", "--name", "X", "--currency", "USD", "--code", "TOOLONG"}},
		"add name without letters":  {"--code", []string{"ac", "add", "--name", "123", "--currency", "USD"}},
		"add with argument":         {"unexpected argument", []string{"ac", "add", "extra", "--name", "X", "--currency", "USD"}},
		"show without reference":    {"code or id", []string{"ac", "show"}},
		"show two references":       {"unexpected argument", []string{"ac", "show", "WISE", "OTHER"}},
		"edit without reference":    {"code or id", []string{"ac", "edit", "--name", "X"}},
		"edit without options":      {"--name", []string{"ac", "edit", "WISE"}},
		"edit bad code":             {"letters A-Z", []string{"ac", "edit", "WISE", "--code", "a-b"}},
		"edit empty name":           {"name", []string{"ac", "edit", "WISE", "--name", ""}},
		"delete without yes":        {"--yes", []string{"ac", "delete", "WISE"}},
		"delete without reference":  {"code or id", []string{"ac", "delete", "--yes"}},
		"restore without reference": {"code or id", []string{"ac", "restore"}},
		"list with argument":        {"unexpected argument", []string{"ac", "list", "extra"}},
	} {
		t.Run(name, func(t *testing.T) {
			fail(t, home, 2, tc.want, tc.args...)
			_, stderr, _ := finux(t, home, tc.args...)
			if !strings.HasSuffix(stderr, "\nRun 'finux accounts --help' for usage.\n") {
				t.Fatalf("stderr has no usage hint: %q", stderr)
			}
		})
	}
	if got := succeed(t, home, "ac"); !strings.Contains(got, "1 account\n") || !strings.Contains(got, "WISE") {
		t.Fatalf("failed commands changed data:\n%s", got)
	}
}

func TestAccountsHelp(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	want := succeed(t, home, "accounts", "help")
	for _, text := range []string{"Usage: finux accounts [command]", "finux ac [command]", "list", "add", "show", "edit", "delete", "restore", "--name", "--code", "--description", "--currency", "--balance", "--color", "--hard", "--yes", "--all", "USD, EUR, BRL, BTC", "slate", "<code|id>", "list, l", "add, a", "edit, e", "delete, d", "restore, r"} {
		if !strings.Contains(want, text) {
			t.Errorf("help lacks %q:\n%s", text, want)
		}
	}
	for _, args := range [][]string{{"ac", "--help"}, {"ac", "-h"}, {"ac", "help"}, {"accounts", "-h"}, {"ac", "add", "--help"}, {"ac", "a", "help"}, {"ac", "show", "-h"}, {"ac", "l", "help"}} {
		if got := succeed(t, home, args...); got != want {
			t.Errorf("finux %v differs from 'finux accounts help'", args)
		}
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("help created the data directory: %v", err)
	}
}

func TestAccountsOfAnotherPerson(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	succeed(t, home, "init")
	succeed(t, home, "ac", "a", "--name", "Nubank", "--currency", "BRL", "--code", "NU", "--balance", "100")
	if got := succeed(t, home, "ac", "a", "--name", "Conta Ana", "--currency", "BRL", "--code", "ANA", "--balance", "50", "--owner", " Ana "); got != "✓ Account ANA created\n" {
		t.Fatalf("add with owner = %q", got)
	}
	const mine = "" +
		"CODE  NAME    CURRENCY    BALANCE\n" +
		"NU    Nubank  BRL       R$ 100,00\n" +
		"1 account\n"
	if got := succeed(t, home, "ac"); got != mine {
		t.Fatalf("list =\n%s\nwant\n%s", got, mine)
	}
	const all = "" +
		"CODE  NAME       OWNER  CURRENCY    BALANCE\n" +
		"ANA   Conta Ana  Ana    BRL        R$ 50,00\n" +
		"NU    Nubank            BRL       R$ 100,00\n" +
		"2 accounts\n"
	if got := succeed(t, home, "ac", "list", "--all"); got != all {
		t.Fatalf("list --all =\n%s\nwant\n%s", got, all)
	}
	if got := succeed(t, home, "ac", "ANA"); !strings.Contains(got, "Owner     Ana\n") {
		t.Fatalf("show lacks the owner:\n%s", got)
	}
	if got := succeed(t, home, "ac", "NU"); strings.Contains(got, "Owner") {
		t.Fatalf("show of my account has an owner:\n%s", got)
	}

	succeed(t, home, "ac", "e", "NU", "--owner", "Bia")
	if got := succeed(t, home, "ac"); got != "No accounts yet. Run 'finux ac add'.\n" {
		t.Fatalf("list with no account of mine = %q", got)
	}
	succeed(t, home, "ac", "e", "NU", "--owner", "")
	if got := succeed(t, home, "ac"); got != mine {
		t.Fatalf("list after the owner is cleared =\n%s", got)
	}
	fail(t, home, 2, "owner must have at most 40 characters", "ac", "e", "NU", "--owner", strings.Repeat("a", 41))
	if got := succeed(t, home, "ac", "help"); !strings.Contains(got, "--owner <name>") {
		t.Fatalf("help lacks --owner:\n%s", got)
	}
}
