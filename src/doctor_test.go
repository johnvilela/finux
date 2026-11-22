package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctor(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	database := filepath.Join(home, "finux.db")

	stdout, stderr, status := finux(t, home, "config", "doctor")
	want := "✗ Database not found: " + database + "\n  Run 'finux init' to create it.\n"
	if status != 1 || stderr != "" || stdout != want {
		t.Fatalf("doctor without database: status %d, stderr %q, stdout %q; want %q", status, stderr, stdout, want)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("doctor created the database directory: %v", err)
	}

	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(database, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = finux(t, home, "config", "doctor")
	want = "✓ Database found: " + database + "\n✗ Migrations pending: table accounts, table categories, table cards, table bills, table goals, table budgets, table budget_categories, table schedules, table transactions, table settings, table telegram_users, table telegram_notices, table telegram_sessions\n  Run 'finux init' to apply them.\n"
	if status != 1 || stderr != "" || stdout != want {
		t.Fatalf("doctor with old database: status %d, stderr %q, stdout %q; want %q", status, stderr, stdout, want)
	}
	if info, err := os.Stat(database); err != nil || info.Size() != 0 {
		t.Fatalf("doctor changed the database: %v", err)
	}

	succeed(t, home, "init")
	if got := succeed(t, home, "config", "doctor"); got != "✓ Database found: "+database+"\n✓ Migrations up to date\n" {
		t.Fatalf("doctor with current database = %q", got)
	}
}

func TestDoctorUsage(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	want := succeed(t, home, "config", "doctor", "help")
	for _, text := range []string{"Usage: finux config doctor", "database", "migrations", "finux init"} {
		if !strings.Contains(want, text) {
			t.Errorf("doctor help lacks %q:\n%s", text, want)
		}
	}
	for _, flag := range []string{"--help", "-h"} {
		if got := succeed(t, home, "config", "doctor", flag); got != want {
			t.Errorf("finux doctor %s differs from 'finux doctor help'", flag)
		}
	}
	fail(t, home, 2, "unexpected argument: extra", "config", "doctor", "extra")
}
