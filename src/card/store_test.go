package card

import (
	"errors"
	"path/filepath"
	"testing"

	"finux/src/account"
	"finux/src/storage"

	"gorm.io/gorm"
)

func newStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &account.Account{}, &Card{}, &Bill{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db), db
}

func create(t *testing.T, store *Store, name, code string) Card {
	t.Helper()
	card := Card{Name: name, Code: code, Color: "blue", Currency: "BRL", Limit: 500000, ClosingDay: 25, DueDay: 5}
	if err := store.Create(&card); err != nil {
		t.Fatalf("create %s: %v", code, err)
	}
	return card
}

func TestStoreCreateFindUpdate(t *testing.T) {
	store, _ := newStore(t)
	card := Card{Name: "  Visa Gold ", Description: " main ", Code: " visa ", Color: "blue", Currency: "BRL", Limit: 500000, ClosingDay: 25, DueDay: 5}
	if err := store.Create(&card); err != nil {
		t.Fatal(err)
	}
	if card.ID == 0 || card.Name != "Visa Gold" || card.Code != "VISA" || card.Description != "main" {
		t.Fatalf("created card not normalized: %+v", card)
	}
	for _, ref := range []string{"VISA", "visa", "1"} {
		got, err := store.Find(ref, false)
		if err != nil || got.ID != card.ID || got.Limit != 500000 || got.Available() != 500000 {
			t.Fatalf("Find(%q) = %+v, %v", ref, got, err)
		}
	}
	if _, err := store.Find("NOPE", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find(NOPE) error = %v, want ErrNotFound", err)
	}

	card.Name, card.Limit, card.ClosingDay, card.DueDay, card.Currency = "Visa Platinum", 900000, 3, 10, "USD"
	if err := store.Update(&card); err != nil {
		t.Fatal(err)
	}
	got, err := store.Find("VISA", false)
	if err != nil || got.Name != "Visa Platinum" || got.Limit != 900000 || got.ClosingDay != 3 || got.DueDay != 10 {
		t.Fatalf("after update: %+v, %v", got, err)
	}
	if got.Currency != "BRL" {
		t.Fatalf("currency changed to %s", got.Currency)
	}
	card.DueDay = 40
	if err := store.Update(&card); err == nil {
		t.Fatal("Update accepts due day 40")
	}
}

func TestStoreCodeIsUniqueAcrossAccounts(t *testing.T) {
	store, db := newStore(t)
	create(t, store, "Visa", "VISA")
	duplicate := Card{Name: "Other", Code: "visa", Color: "red", Currency: "BRL", ClosingDay: 1, DueDay: 10}
	if err := store.Create(&duplicate); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate card code: error = %v, want ErrCodeTaken", err)
	}

	accounts := account.NewStore(db)
	inter := account.Account{Name: "Inter", Code: "INTER", Color: "orange", Currency: "BRL"}
	if err := accounts.Create(&inter); err != nil {
		t.Fatal(err)
	}
	sameAsAccount := Card{Name: "Inter card", Code: "INTER", Color: "red", Currency: "BRL", ClosingDay: 1, DueDay: 10}
	if err := store.Create(&sameAsAccount); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("card with the code of an account: error = %v, want ErrCodeTaken", err)
	}
	sameAsCard := account.Account{Name: "Visa account", Code: "VISA", Color: "red", Currency: "BRL"}
	if err := accounts.Create(&sameAsCard); !errors.Is(err, account.ErrCodeTaken) {
		t.Fatalf("account with the code of a card: error = %v, want ErrCodeTaken", err)
	}
}

func TestStoreArchiveRestoreDelete(t *testing.T) {
	store, _ := newStore(t)
	visa := create(t, store, "Visa", "VISA")
	create(t, store, "Master", "MC")
	if err := store.Archive(visa.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := store.List(false); len(active) != 1 || active[0].Code != "MC" {
		t.Fatalf("active cards = %+v", active)
	}
	if all, _ := store.List(true); len(all) != 2 {
		t.Fatalf("all cards = %d, want 2", len(all))
	}
	if archived, _ := store.ListArchived(); len(archived) != 1 || !archived[0].Archived() {
		t.Fatalf("archived cards = %+v", archived)
	}
	if _, err := store.Find("VISA", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived card found: %v", err)
	}
	if err := store.Restore(visa.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(visa.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find("VISA", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted card found: %v", err)
	}
	if err := store.Delete(visa.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
}

func TestStoreBills(t *testing.T) {
	store, _ := newStore(t)
	visa := create(t, store, "Visa", "VISA")

	current, err := store.Current(visa, "2026-11-17")
	if err != nil || current.ID != 0 || current.Period != "2026-12" || current.ClosesOn != "2026-11-25" || current.DueOn != "2026-12-05" {
		t.Fatalf("Current without a bill = %+v, %v", current, err)
	}

	first, err := store.BillFor(visa, "2026-11-17")
	if err != nil || first.ID == 0 || first.CardID != visa.ID || first.Period != "2026-12" || first.ClosesOn != "2026-11-25" || first.DueOn != "2026-12-05" {
		t.Fatalf("BillFor = %+v, %v", first, err)
	}
	again, err := store.BillFor(visa, "2026-11-01")
	if err != nil || again.ID != first.ID {
		t.Fatalf("second BillFor = %+v, %v, want bill %d", again, err, first.ID)
	}
	next, err := store.BillFor(visa, "2026-11-26")
	if err != nil || next.ID == first.ID || next.Period != "2027-01" {
		t.Fatalf("BillFor after closing = %+v, %v", next, err)
	}
	byPeriod, err := store.BillOf(visa, "2027-01")
	if err != nil || byPeriod.ID != next.ID {
		t.Fatalf("BillOf = %+v, %v", byPeriod, err)
	}

	// New days of the card do not change a bill that exists.
	visa.ClosingDay, visa.DueDay = 10, 20
	if err := store.Update(&visa); err != nil {
		t.Fatal(err)
	}
	kept, err := store.Bill(first.ID)
	if err != nil || kept.ClosesOn != "2026-11-25" || kept.DueOn != "2026-12-05" {
		t.Fatalf("bill after the card changed = %+v, %v", kept, err)
	}

	bills, err := store.Bills(visa.ID)
	if err != nil || len(bills) != 2 || bills[0].Period != "2027-01" || bills[1].Period != "2026-12" {
		t.Fatalf("Bills = %+v, %v", bills, err)
	}
	if current, err := store.Current(visa, "2026-11-17"); err != nil || current.ID != first.ID {
		t.Fatalf("Current = %+v, %v", current, err)
	}
	if _, err := store.Bill(99); !errors.Is(err, ErrBillNotFound) {
		t.Fatalf("Bill(99) error = %v, want ErrBillNotFound", err)
	}
}
