package schedule

import (
	"errors"
	"path/filepath"
	"testing"

	"finux/src/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Schedule{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func create(t *testing.T, store *Store, current Schedule) Schedule {
	t.Helper()
	if err := store.Create(&current); err != nil {
		t.Fatal(err)
	}
	return current
}

func TestStoreLifecycle(t *testing.T) {
	store := newStore(t)
	draft := rent()
	draft.Title, draft.Code = "  Rent ", "rent"
	created := create(t, store, draft)
	if created.ID == 0 || created.Title != "Rent" || created.Code != "RENT" {
		t.Fatalf("created = %+v", created)
	}
	if err := store.Create(&Schedule{Title: "Other", Code: "RENT", Type: Expense, Value: 1, AccountID: 1, CategoryID: 1, StartDate: "2026-11-05"}); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code error = %v", err)
	}
	invalid := rent()
	invalid.Code, invalid.Value = "X", 0
	if err := store.Create(&invalid); err == nil {
		t.Fatal("Create accepted a value of zero")
	}
	for _, ref := range []string{"rent", "1"} {
		if found, err := store.Find(ref, false); err != nil || found.ID != created.ID {
			t.Fatalf("Find(%q) = %+v, %v", ref, found, err)
		}
	}
	if _, err := store.Find("NONE", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find(NONE) error = %v", err)
	}

	created.Title, created.Value, created.Every, created.Unit, created.Times, created.Auto, created.Tags = "Home rent", 160000, 2, "week", 6, true, "#home"
	created.CardID, created.AccountID, created.GoalID, created.CategoryID, created.EndDate = 4, 0, 7, 9, "2027-12-31"
	if err := store.Update(&created); err != nil {
		t.Fatal(err)
	}
	found, _ := store.Find("RENT", false)
	if found.Title != "Home rent" || found.Value != 160000 || found.Every != 2 || found.Unit != "week" || found.Times != 6 || !found.Auto ||
		found.Tags != "#home" || found.CardID != 4 || found.AccountID != 0 || found.GoalID != 7 || found.CategoryID != 9 || found.EndDate != "2027-12-31" {
		t.Fatalf("updated = %+v", found)
	}
	// An update can turn the options off again.
	found.Auto, found.Times, found.EndDate, found.GoalID, found.Tags = false, 0, "", 0, ""
	if err := store.Update(&found); err != nil {
		t.Fatal(err)
	}
	if found, _ = store.Find("RENT", false); found.Auto || found.Times != 0 || found.EndDate != "" || found.GoalID != 0 || found.Tags != "" {
		t.Fatalf("options stay after the update: %+v", found)
	}

	if err := store.Advance(found.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Advance(found.ID); err != nil {
		t.Fatal(err)
	}
	if found, _ = store.Find("RENT", false); found.Done != 2 {
		t.Fatalf("done = %d, want 2", found.Done)
	}
	if err := store.Advance(99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Advance(99) error = %v", err)
	}

	create(t, store, Schedule{Title: "dentist", Code: "DENT", Type: Expense, Value: 30000, AccountID: 1, CategoryID: 3, StartDate: "2026-12-10"})
	if all, err := store.List(false); err != nil || len(all) != 2 || all[0].Code != "DENT" || all[1].Code != "RENT" {
		t.Fatalf("List = %+v, %v", all, err)
	}
	if err := store.Archive(found.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := store.List(false); len(active) != 1 {
		t.Fatalf("active = %+v", active)
	}
	if all, _ := store.List(true); len(all) != 2 || !all[1].Archived() {
		t.Fatalf("all = %+v", all)
	}
	if archived, _ := store.ListArchived(); len(archived) != 1 || archived[0].Code != "RENT" {
		t.Fatalf("archived = %+v", archived)
	}
	if _, err := store.Find("RENT", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find of an archived schedule = %v", err)
	}
	if taken, _ := store.CodeTaken("rent", 0); !taken {
		t.Fatal("an archived schedule lost its code")
	}
	if err := store.Restore(found.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(found.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Restore = %v", err)
	}
	if err := store.Delete(found.ID); err != nil {
		t.Fatal(err)
	}
	if all, _ := store.List(true); len(all) != 1 {
		t.Fatalf("after delete = %+v", all)
	}
}

func TestStoreFollowsOtherRecords(t *testing.T) {
	store := newStore(t)
	onAccount := rent()
	onAccount.GoalID = 5
	onAccount = create(t, store, onAccount)
	onCard := rent()
	onCard.Code, onCard.AccountID, onCard.CardID, onCard.CategoryID = "NET", 0, 3, 8
	onCard = create(t, store, onCard)

	if codes, err := store.CodesByCategory(2); err != nil || len(codes) != 1 || codes[0] != "RENT" {
		t.Fatalf("CodesByCategory = %v, %v", codes, err)
	}
	if err := store.ClearGoal(5); err != nil {
		t.Fatal(err)
	}
	if found, _ := store.Find("RENT", false); found.GoalID != 0 {
		t.Fatalf("after the moves = %+v", found)
	}
	if found, _ := store.Find("NET", false); found.CategoryID != 8 {
		t.Fatalf("other schedule changed: %+v", found)
	}
	// A schedule goes with its account or its card, also when archived.
	if err := store.Archive(onCard.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteByCard(3); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteByAccount(99); err != nil {
		t.Fatal(err)
	}
	if all, _ := store.List(true); len(all) != 1 || all[0].ID != onAccount.ID {
		t.Fatalf("after the card delete = %+v", all)
	}
	if err := store.DeleteByAccount(1); err != nil {
		t.Fatal(err)
	}
	if all, _ := store.List(true); len(all) != 0 {
		t.Fatalf("after the account delete = %+v", all)
	}
}
