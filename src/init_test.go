package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	home := filepath.Join(t.TempDir(), "nested", "data")
	database := filepath.Join(home, "finux.db")
	fail(t, home, 1, "Run 'finux init'", "ac")
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("a command other than init created the database directory: %v", err)
	}
	if got := succeed(t, home, "init"); got != "✓ Database created: "+database+"\n" {
		t.Fatalf("first init = %q", got)
	}
	if _, err := os.Stat(database); err != nil {
		t.Fatalf("database file: %v", err)
	}
	succeed(t, home, "ac", "add", "--name", "Wise", "--currency", "USD")
	if got := succeed(t, home, "init"); got != "✓ Database up to date: "+database+"\n" {
		t.Fatalf("second init = %q", got)
	}
	if got := succeed(t, home, "ac"); !strings.Contains(got, "WISE") {
		t.Fatalf("init removed data:\n%s", got)
	}
}

func TestInitDefaultPath(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, status := finuxEnv(t, []string{"HOME=" + home}, "init")
	want := filepath.Join(home, ".finux", "finux.db")
	if status != 0 || stderr != "" || stdout != "✓ Database created: "+want+"\n" {
		t.Fatalf("init = %q, stderr %q, status %d", stdout, stderr, status)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("default database file: %v", err)
	}
}

func TestInitUsage(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	want := succeed(t, home, "init", "help")
	for _, text := range []string{"Usage: finux init", "FINUX_DB", "~/.finux/finux.db", "migrations"} {
		if !strings.Contains(want, text) {
			t.Errorf("init help lacks %q:\n%s", text, want)
		}
	}
	for _, flag := range []string{"--help", "-h"} {
		if got := succeed(t, home, "init", flag); got != want {
			t.Errorf("finux init %s differs from 'finux init help'", flag)
		}
	}
	fail(t, home, 2, "unexpected argument: extra", "init", "extra")
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("help or a usage error created the database directory: %v", err)
	}
}
