package account

import (
	"errors"
	"testing"

	"finux/src/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(t.TempDir(), &Account{})
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func create(t *testing.T, store *Store, name, code string) Account {
	t.Helper()
	account := Account{Name: name, Code: code, Color: "blue", Currency: "BRL", OpeningBalance: 1000}
	if err := store.Create(&account); err != nil {
		t.Fatalf("create %s: %v", code, err)
	}
	return account
}

func TestStoreCreateAndFind(t *testing.T) {
	store := newStore(t)
	account := Account{Name: "  Banco Inter ", Description: " main ", Code: " inter ", Color: "orange", Currency: "BRL", OpeningBalance: -250}
	if err := store.Create(&account); err != nil {
		t.Fatal(err)
	}
	if account.ID == 0 || account.Name != "Banco Inter" || account.Code != "INTER" || account.Description != "main" {
		t.Fatalf("created account not normalized: %+v", account)
	}
	if account.CreatedAt.IsZero() || account.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", account)
	}
	for _, ref := range []string{"INTER", "inter", "1"} {
		got, err := store.Find(ref, false)
		if err != nil || got.ID != account.ID || got.OpeningBalance != -250 || got.Balance() != -250 {
			t.Fatalf("Find(%q) = %+v, %v", ref, got, err)
		}
	}
	for _, ref := range []string{"NOPE", "99", ""} {
		if _, err := store.Find(ref, false); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Find(%q) error = %v, want ErrNotFound", ref, err)
		}
	}
}

func TestStoreRejectsInvalidAndDuplicate(t *testing.T) {
	store := newStore(t)
	create(t, store, "Banco Inter", "INTER")
	duplicate := Account{Name: "Other", Code: "inter", Color: "red", Currency: "USD"}
	if err := store.Create(&duplicate); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code error = %v, want ErrCodeTaken", err)
	}
	invalid := Account{Name: "", Code: "X", Color: "red", Currency: "USD"}
	if err := store.Create(&invalid); err == nil {
		t.Fatal("invalid account created")
	}
	if taken, err := store.CodeTaken("INTER", 0); err != nil || !taken {
		t.Fatalf("CodeTaken(INTER) = %v, %v", taken, err)
	}
	if taken, err := store.CodeTaken("FREE", 0); err != nil || taken {
		t.Fatalf("CodeTaken(FREE) = %v, %v", taken, err)
	}
}

func TestStoreListOrdersByName(t *testing.T) {
	store := newStore(t)
	create(t, store, "nubank", "NUBAN")
	create(t, store, "Banco Inter", "INTER")
	create(t, store, "Wise", "WISE")
	accounts, err := store.List(false)
	if err != nil || len(accounts) != 3 {
		t.Fatalf("List = %v, %v", accounts, err)
	}
	if accounts[0].Code != "INTER" || accounts[1].Code != "NUBAN" || accounts[2].Code != "WISE" {
		t.Fatalf("order = %s %s %s", accounts[0].Code, accounts[1].Code, accounts[2].Code)
	}
}

func TestStoreUpdate(t *testing.T) {
	store := newStore(t)
	account := create(t, store, "Banco Inter", "INTER")
	other := create(t, store, "Nubank", "NUBAN")
	account.Name, account.Code, account.Description, account.Color = "Inter PJ", "intpj", "business", "green"
	account.Currency, account.OpeningBalance = "USD", 999999
	if err := store.Update(&account); err != nil {
		t.Fatal(err)
	}
	got, err := store.Find("INTPJ", false)
	if err != nil || got.Name != "Inter PJ" || got.Description != "business" || got.Color != "green" {
		t.Fatalf("after update = %+v, %v", got, err)
	}
	if got.Currency != "BRL" || got.OpeningBalance != 1000 {
		t.Fatalf("currency or opening balance changed: %+v", got)
	}
	if _, err := store.Find("INTER", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old code still found: %v", err)
	}
	other.Code = "INTPJ"
	if err := store.Update(&other); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("update to taken code error = %v, want ErrCodeTaken", err)
	}
	got.Name = "Same code"
	if err := store.Update(&got); err != nil {
		t.Fatalf("update that keeps its own code: %v", err)
	}
}

func TestStoreArchiveRestoreDelete(t *testing.T) {
	store := newStore(t)
	account := create(t, store, "Banco Inter", "INTER")
	create(t, store, "Nubank", "NUBAN")
	if err := store.Archive(account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find("INTER", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived account found: %v", err)
	}
	archived, err := store.Find("INTER", true)
	if err != nil || !archived.Archived() {
		t.Fatalf("Find with archived = %+v, %v", archived, err)
	}
	if active, _ := store.List(false); len(active) != 1 || active[0].Code != "NUBAN" {
		t.Fatalf("active list = %+v", active)
	}
	if all, _ := store.List(true); len(all) != 2 {
		t.Fatalf("full list has %d accounts, want 2", len(all))
	}
	if only, _ := store.ListArchived(); len(only) != 1 || only[0].Code != "INTER" {
		t.Fatalf("archived list = %+v", only)
	}
	reuse := Account{Name: "New", Code: "INTER", Color: "red", Currency: "USD"}
	if err := store.Create(&reuse); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("archived code reused: %v", err)
	}
	if err := store.Restore(account.ID); err != nil {
		t.Fatal(err)
	}
	if restored, err := store.Find("INTER", false); err != nil || restored.Archived() {
		t.Fatalf("after restore = %+v, %v", restored, err)
	}
	if err := store.Archive(account.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find("INTER", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hard-deleted account found: %v", err)
	}
	if err := store.Create(&reuse); err != nil {
		t.Fatalf("code free after hard delete: %v", err)
	}
	for name, action := range map[string]func(uint) error{"archive": store.Archive, "restore": store.Restore, "delete": store.Delete} {
		if err := action(4242); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s of missing id error = %v, want ErrNotFound", name, err)
		}
	}
}
