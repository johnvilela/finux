package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finux/src/account"
	"finux/src/category"
	"finux/src/storage"
)

func TestInit(t *testing.T) {
	home := filepath.Join(t.TempDir(), "nested", "data")
	database := filepath.Join(home, "finux.db")
	fail(t, home, 1, "Run 'finux init'", "ac")
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("a command other than init created the database directory: %v", err)
	}
	if got := succeed(t, home, "init"); got != "✓ Database created: "+database+"\n✓ Base categories added: 14\n" {
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
	if status != 0 || stderr != "" || stdout != "✓ Database created: "+want+"\n✓ Base categories added: 14\n" {
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

func TestSetupDatabaseAddsBaseCategoriesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "finux.db")
	report, err := setupDatabase(path)
	if err != nil || !report.created || report.baseCategories != 14 {
		t.Fatalf("first setup = %+v, %v; want created with 14 base categories", report, err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store := category.NewStore(db)
	food, err := store.Find("FOOD")
	if err != nil {
		t.Fatalf("base category FOOD: %v", err)
	}
	if err := store.Delete(food.ID); err != nil {
		t.Fatal(err)
	}
	report, err = setupDatabase(path)
	if err != nil || report.created || report.baseCategories != 0 {
		t.Fatalf("second setup = %+v, %v; want nothing new", report, err)
	}
	if list, _ := store.List(); len(list) != 13 {
		t.Fatalf("second setup brought a deleted category back: %d categories", len(list))
	}
}

func TestSetupDatabaseUpgradesOldDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finux.db")
	// A database from before the categories module has only accounts.
	if _, err := storage.Init(path, &account.Account{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	wise := account.Account{Name: "Wise", Code: "WISE", Color: "blue", Currency: "USD"}
	if err := account.NewStore(db).Create(&wise); err != nil {
		t.Fatal(err)
	}
	report, err := setupDatabase(path)
	if err != nil || report.created || report.baseCategories != 14 {
		t.Fatalf("setup of old database = %+v, %v; want 14 base categories", report, err)
	}
	if _, err := account.NewStore(db).Find("WISE", false); err != nil {
		t.Fatalf("setup removed an account: %v", err)
	}
}

func TestReseedCommand(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 1, "Run 'finux init'", "reseed")
	succeed(t, home, "init")
	if got := succeed(t, home, "reseed"); got != "✓ Base data is complete\n" {
		t.Fatalf("reseed of complete database = %q", got)
	}
	db, err := storage.Open(filepath.Join(home, "finux.db"))
	if err != nil {
		t.Fatal(err)
	}
	store := category.NewStore(db)
	for _, reference := range []string{"FOOD", "TAX"} {
		found, err := store.Find(reference)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Delete(found.ID); err != nil {
			t.Fatal(err)
		}
	}
	if got := succeed(t, home, "reseed"); got != "✓ Base categories added: 2 (FOOD, TAX)\n" {
		t.Fatalf("reseed with missing categories = %q", got)
	}
	if got := succeed(t, home, "reseed"); got != "✓ Base data is complete\n" {
		t.Fatalf("second reseed = %q", got)
	}

	want := succeed(t, home, "reseed", "help")
	for _, text := range []string{"Usage: finux reseed", "base categories", "finux init"} {
		if !strings.Contains(want, text) {
			t.Errorf("reseed help lacks %q:\n%s", text, want)
		}
	}
	for _, flag := range []string{"--help", "-h"} {
		if got := succeed(t, home, "reseed", flag); got != want {
			t.Errorf("finux reseed %s differs from 'finux reseed help'", flag)
		}
	}
	fail(t, home, 2, "unexpected argument: extra", "reseed", "extra")
}
