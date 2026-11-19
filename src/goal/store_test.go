package goal

import (
	"errors"
	"path/filepath"
	"testing"

	"finux/src/storage"

	"gorm.io/gorm"
)

// movement is the part of the transactions table that goals read.
type movement struct {
	ID     uint `gorm:"primaryKey"`
	Type   string
	Amount int64
	GoalID uint
}

func (movement) TableName() string { return "transactions" }

func newStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Goal{}, &movement{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db), db
}

func create(t *testing.T, store *Store, title, code, kind string) Goal {
	t.Helper()
	goal := Goal{Title: title, Code: code, Type: kind, Currency: "BRL", Target: 100000}
	if err := store.Create(&goal); err != nil {
		t.Fatalf("create %s: %v", code, err)
	}
	return goal
}

func TestStoreCreateFindUpdate(t *testing.T) {
	store, _ := newStore(t)
	goal := Goal{Title: "  Trip to Japan ", Description: " in 2027 ", Code: " trip ", Type: Save, Currency: "BRL", Target: 1000000, DueDate: "2027-06-30"}
	if err := store.Create(&goal); err != nil {
		t.Fatal(err)
	}
	if goal.ID == 0 || goal.Title != "Trip to Japan" || goal.Code != "TRIP" || goal.Description != "in 2027" {
		t.Fatalf("created goal not normalized: %+v", goal)
	}
	for _, ref := range []string{"TRIP", "trip", "1"} {
		found, err := store.Find(ref, false)
		if err != nil || found.ID != goal.ID || found.DueDate != "2027-06-30" {
			t.Fatalf("Find(%q) = %+v, %v", ref, found, err)
		}
	}
	if _, err := store.Find("NONE", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find(NONE) error = %v, want ErrNotFound", err)
	}

	goal.Title, goal.Code, goal.Type, goal.Target, goal.DueDate, goal.Currency = "Japan", "JAPAN", Purchase, 500000, "", "USD"
	if err := store.Update(&goal); err != nil {
		t.Fatal(err)
	}
	found, err := store.Find("JAPAN", false)
	if err != nil || found.Title != "Japan" || found.Type != Purchase || found.Target != 500000 || found.DueDate != "" {
		t.Fatalf("updated goal = %+v, %v", found, err)
	}
	if found.Currency != "BRL" {
		t.Fatalf("Update changed the currency to %s", found.Currency)
	}

	other := Goal{Title: "Other", Code: "japan", Type: Save, Currency: "BRL", Target: 1}
	if err := store.Create(&other); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code error = %v, want ErrCodeTaken", err)
	}
	if err := store.Create(&Goal{Title: "Bad", Code: "BAD", Type: Save, Currency: "BRL"}); err == nil {
		t.Fatal("Create accepted a goal without a target")
	}
}

func TestStoreArchiveRestoreDelete(t *testing.T) {
	store, _ := newStore(t)
	trip := create(t, store, "Trip", "TRIP", Save)
	create(t, store, "Car", "CAR", Purchase)
	if err := store.Archive(trip.ID); err != nil {
		t.Fatal(err)
	}
	if goals, _ := store.List(false); len(goals) != 1 || goals[0].Code != "CAR" {
		t.Fatalf("active goals = %+v", goals)
	}
	if goals, _ := store.List(true); len(goals) != 2 || goals[0].Code != "CAR" || !goals[1].Archived() {
		t.Fatalf("all goals = %+v", goals)
	}
	if goals, _ := store.ListArchived(); len(goals) != 1 || goals[0].Code != "TRIP" {
		t.Fatalf("archived goals = %+v", goals)
	}
	if _, err := store.Find("TRIP", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find of an archived goal = %v, want ErrNotFound", err)
	}
	// An archived goal keeps its code.
	if taken, _ := store.CodeTaken("trip", 0); !taken {
		t.Fatal("the code of an archived goal is free")
	}
	if err := store.Restore(trip.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(trip.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second restore = %v, want ErrNotFound", err)
	}
	if err := store.Delete(trip.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find("TRIP", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find after delete = %v, want ErrNotFound", err)
	}
	if err := store.Delete(trip.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestStoreProgress(t *testing.T) {
	store, db := newStore(t)
	trip := create(t, store, "Trip", "TRIP", Save)
	car := create(t, store, "Car", "CAR", Purchase)
	debt := create(t, store, "Debt", "DEBT", PayOff)
	empty := create(t, store, "Empty", "EMPTY", Save)
	rows := []movement{
		// Save: an income adds, an expense removes, a transfer adds one time.
		{Type: "income", Amount: 50000, GoalID: trip.ID},
		{Type: "expense", Amount: -10000, GoalID: trip.ID},
		{Type: "transfer", Amount: -20000, GoalID: trip.ID},
		{Type: "transfer", Amount: 20000, GoalID: trip.ID},
		// Purchase counts as save. More expenses than incomes is below zero.
		{Type: "income", Amount: 1000, GoalID: car.ID},
		{Type: "expense", Amount: -4000, GoalID: car.ID},
		// Pay off: an expense adds and an income removes.
		{Type: "expense", Amount: -70000, GoalID: debt.ID},
		{Type: "income", Amount: 5000, GoalID: debt.ID},
		{Type: "transfer", Amount: -1000, GoalID: debt.ID},
		{Type: "transfer", Amount: 1000, GoalID: debt.ID},
		// Not linked.
		{Type: "income", Amount: 999999},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		code  string
		saved int64
		links int64
	}{
		{"TRIP", 60000, 3}, {"CAR", -3000, 2}, {"DEBT", 66000, 3}, {"EMPTY", 0, 0},
	} {
		found, err := store.Find(tc.code, false)
		if err != nil || found.Saved != tc.saved || found.Links != tc.links {
			t.Errorf("%s: saved %d, links %d, %v; want %d, %d", tc.code, found.Saved, found.Links, err, tc.saved, tc.links)
		}
	}
	goals, err := store.List(false)
	if err != nil || len(goals) != 4 {
		t.Fatalf("List = %d goals, %v", len(goals), err)
	}
	for _, goal := range goals {
		if goal.Code == "DEBT" && goal.Saved != 66000 {
			t.Errorf("List: DEBT saved %d, want 66000", goal.Saved)
		}
	}
	_ = empty
}

// A database from before the transactions module has no transactions
// table. Goals then have no progress.
func TestStoreWithoutTransactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Goal{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	create(t, store, "Trip", "TRIP", Save)
	if goals, err := store.List(false); err != nil || len(goals) != 1 || goals[0].Saved != 0 {
		t.Fatalf("List = %+v, %v", goals, err)
	}
}
