package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/storage"
	"finux/src/transaction"
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
	if _, err := account.NewStore(db).Find("WISE", false, today()); err != nil {
		t.Fatalf("setup removed an account: %v", err)
	}
}

func TestReseedCommand(t *testing.T) {
	// Only a development build has the reseed command.
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 2, "unsupported arguments: reseed", "reseed")

	database := filepath.Join(t.TempDir(), "finux-dev.db")
	binary := devBinary(t, database)
	reseed := func(args ...string) (stdout, stderr string, status int) {
		t.Helper()
		return runBinary(t, binary, nil, append([]string{"reseed"}, args...)...)
	}
	ok := func(args ...string) string {
		t.Helper()
		stdout, stderr, status := reseed(args...)
		if status != 0 || stderr != "" {
			t.Fatalf("finux-dev reseed %v: status %d, stderr %q, stdout %q", args, status, stderr, stdout)
		}
		return stdout
	}
	if _, stderr, status := reseed(); status != 1 || !strings.Contains(stderr, "Run 'finux init'") {
		t.Fatalf("reseed without a database: status %d, stderr %q", status, stderr)
	}
	if _, stderr, status := runBinary(t, binary, nil, "init"); status != 0 || stderr != "" {
		t.Fatalf("init: status %d, stderr %q", status, stderr)
	}
	if got := ok(); got != "✓ Base data is complete\n" {
		t.Fatalf("reseed of complete database = %q", got)
	}
	db, err := storage.Open(database)
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
	if got := ok(); got != "✓ Base categories added: 2 (FOOD, TAX)\n" {
		t.Fatalf("reseed with missing categories = %q", got)
	}
	if got := ok(); got != "✓ Base data is complete\n" {
		t.Fatalf("second reseed = %q", got)
	}

	want := ok("help")
	for _, text := range []string{"Usage: finux reseed", "base categories", "finux init"} {
		if !strings.Contains(want, text) {
			t.Errorf("reseed help lacks %q:\n%s", text, want)
		}
	}
	for _, flag := range []string{"--help", "-h"} {
		if got := ok(flag); got != want {
			t.Errorf("finux reseed %s differs from 'finux reseed help'", flag)
		}
	}
	if _, stderr, status := reseed("extra"); status != 2 || !strings.Contains(stderr, "unexpected argument: extra") {
		t.Fatalf("reseed extra: status %d, stderr %q", status, stderr)
	}
}

func TestSetupDatabaseAddsTheVirtualCardColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := setupDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// A database from before the virtual cards has no column for the mark.
	if err := db.Migrator().DropColumn(&transaction.Transaction{}, "virtual"); err != nil {
		t.Fatal(err)
	}
	if transactionsReady(db) {
		t.Fatal("a database without the column is ready")
	}
	if _, err := setupDatabase(path); err != nil {
		t.Fatal(err)
	}
	if !transactionsReady(db) {
		t.Fatal("the setup did not add the column")
	}
}

func TestSetupDatabaseAddsTheOffLimitColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := setupDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// A database from before the transactions outside the limit has no
	// column for the mark.
	if err := db.Migrator().DropColumn(&transaction.Transaction{}, "off_limit"); err != nil {
		t.Fatal(err)
	}
	if transactionsReady(db) {
		t.Fatal("a database without the column is ready")
	}
	if _, err := setupDatabase(path); err != nil {
		t.Fatal(err)
	}
	if !transactionsReady(db) {
		t.Fatal("the setup did not add the column")
	}
}

func TestSetupDatabaseAddsTheOwnerColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := setupDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// A database from before the owners has no column for them, and it
	// has accounts and credit cards.
	old := account.Account{Name: "Banco Inter", Code: "INTER", Color: "orange", Currency: "BRL"}
	if err := account.NewStore(db).Create(&old); err != nil {
		t.Fatal(err)
	}
	visa := card.Card{Name: "Visa", Code: "VISA", Color: "blue", Currency: "BRL", ClosingDay: 25, DueDay: 5}
	if err := card.NewStore(db).Create(&visa); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&account.Account{}, &card.Card{}} {
		if err := db.Migrator().DropColumn(model, "owner"); err != nil {
			t.Fatal(err)
		}
		if transactionsReady(db) {
			t.Fatalf("a database without the owner of %T is ready", model)
		}
		if _, err := setupDatabase(path); err != nil {
			t.Fatal(err)
		}
		if !transactionsReady(db) {
			t.Fatalf("the setup did not add the owner of %T", model)
		}
	}
	found, err := account.NewStore(db).Find("INTER", false, "2026-11-17")
	if err != nil || !found.Mine() {
		t.Fatalf("old account after the setup: %+v, %v", found, err)
	}
	if found, err := card.NewStore(db).Find("VISA", false); err != nil || !found.Mine() {
		t.Fatalf("old card after the setup: %+v, %v", found, err)
	}
	var mine int64
	if err := db.Table("accounts").Where("owner = ''").Count(&mine).Error; err != nil || mine != 1 {
		t.Fatalf("accounts with an empty owner = %d, %v", mine, err)
	}
}
