package budget

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"finux/src/storage"

	"gorm.io/gorm"
)

// movement is the part of the transactions table that budgets read.
type movement struct {
	ID         uint `gorm:"primaryKey"`
	Type       string
	Amount     int64
	CategoryID uint
	AccountID  uint
	CardID     uint
	Date       string
}

func (movement) TableName() string { return "transactions" }

// wallet and plastic are the parts of the accounts and cards tables that
// budgets read.
type wallet struct {
	ID       uint `gorm:"primaryKey"`
	Currency string
}

func (wallet) TableName() string { return "accounts" }

type plastic struct {
	ID       uint `gorm:"primaryKey"`
	Currency string
}

func (plastic) TableName() string { return "cards" }

// kind is the part of the categories table that budgets read.
type kind struct {
	ID    uint `gorm:"primaryKey"`
	Color string
}

func (kind) TableName() string { return "categories" }

const (
	today = "2026-11-18"

	// Food and fun are expense categories; salary is an income category.
	food, fun, salary = 1, 2, 3
	// Account 1 and card 1 are in BRL; account 2 and card 2 are in EUR.
	real, euro = 1, 2
)

func newStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Budget{}, &Link{}, &movement{}, &wallet{}, &plastic{}, &kind{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, rows := range []any{
		&[]wallet{{Currency: "BRL"}, {Currency: "EUR"}},
		&[]plastic{{Currency: "BRL"}, {Currency: "EUR"}},
		&[]kind{{Color: "red"}, {Color: "red"}, {Color: "green"}},
	} {
		if err := db.Create(rows).Error; err != nil {
			t.Fatal(err)
		}
	}
	return NewStore(db), db
}

func create(t *testing.T, store *Store, title, code string, categories ...uint) Budget {
	t.Helper()
	budget := Budget{Title: title, Code: code, Currency: "BRL", Limit: 80000, StartDate: "2026-10-01", Every: 1, Unit: Month, CategoryIDs: categories}
	if err := store.Create(&budget); err != nil {
		t.Fatalf("create %s: %v", code, err)
	}
	return budget
}

func TestStoreCreateFindUpdate(t *testing.T) {
	store, _ := newStore(t)
	budget := Budget{
		Title: "  Food ", Description: " market and restaurants ", Code: " food ", Currency: "BRL", Limit: 80000,
		StartDate: "2026-11-05", Every: 1, Unit: Month, CategoryIDs: []uint{fun, food, fun},
	}
	if err := store.Create(&budget); err != nil {
		t.Fatal(err)
	}
	if budget.ID == 0 || budget.Title != "Food" || budget.Code != "FOOD" || budget.Description != "market and restaurants" {
		t.Fatalf("created budget not normalized: %+v", budget)
	}
	for _, ref := range []string{"FOOD", "food", "1"} {
		found, err := store.Find(ref, false, today)
		if err != nil || found.ID != budget.ID || found.StartDate != "2026-11-05" || !reflect.DeepEqual(found.CategoryIDs, []uint{food, fun}) {
			t.Fatalf("Find(%q) = %+v, %v", ref, found, err)
		}
		if len(found.Usages) != 1 || found.Usages[0].From != "2026-11-05" || found.Usages[0].To != "2026-12-04" {
			t.Fatalf("Find(%q) usages = %+v", ref, found.Usages)
		}
	}
	if _, err := store.Find("NONE", true, today); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find(NONE) error = %v, want ErrNotFound", err)
	}

	budget.Title, budget.Code, budget.Limit, budget.Currency = "Fun", "FUN", 50000, "USD"
	budget.StartDate, budget.Every, budget.Unit, budget.EndDate, budget.Rollover = "2026-11-01", 0, "", "2026-12-31", true
	budget.CategoryIDs = []uint{fun}
	if err := store.Update(&budget); err != nil {
		t.Fatal(err)
	}
	found, err := store.Find("FUN", false, today)
	if err != nil || found.Title != "Fun" || found.Limit != 50000 || found.Every != 0 || found.Unit != "" || found.EndDate != "2026-12-31" || !found.Rollover {
		t.Fatalf("updated budget = %+v, %v", found, err)
	}
	if !reflect.DeepEqual(found.CategoryIDs, []uint{fun}) {
		t.Fatalf("updated categories = %v", found.CategoryIDs)
	}
	if found.Currency != "BRL" {
		t.Fatalf("Update changed the currency to %s", found.Currency)
	}
	// Update saves values that go back to zero.
	found.Rollover, found.CategoryIDs = false, nil
	if err := store.Update(&found); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.Find("FUN", false, today); again.Rollover || len(again.CategoryIDs) != 0 {
		t.Fatalf("zero values not saved: %+v", again)
	}

	other := Budget{Title: "Other", Code: "fun", Currency: "BRL", Limit: 1, StartDate: "2026-11-01", Every: 1, Unit: Week}
	if err := store.Create(&other); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code error = %v, want ErrCodeTaken", err)
	}
	if err := store.Create(&Budget{Title: "Bad", Code: "BAD", Currency: "BRL", StartDate: "2026-11-01", Every: 1, Unit: Week}); err == nil {
		t.Fatal("Create accepted a budget without a limit")
	}
}

func TestStoreArchiveRestoreDelete(t *testing.T) {
	store, db := newStore(t)
	meals := create(t, store, "Meals", "MEALS", food)
	create(t, store, "Fun", "FUN", fun)
	if err := store.Archive(meals.ID); err != nil {
		t.Fatal(err)
	}
	if budgets, _ := store.List(false, today); len(budgets) != 1 || budgets[0].Code != "FUN" {
		t.Fatalf("active budgets = %+v", budgets)
	}
	if budgets, _ := store.List(true, today); len(budgets) != 2 || budgets[0].Code != "FUN" || !budgets[1].Archived() {
		t.Fatalf("all budgets = %+v", budgets)
	}
	if budgets, _ := store.ListArchived(today); len(budgets) != 1 || budgets[0].Code != "MEALS" || len(budgets[0].CategoryIDs) != 1 {
		t.Fatalf("archived budgets = %+v", budgets)
	}
	if _, err := store.Find("MEALS", false, today); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find of an archived budget = %v, want ErrNotFound", err)
	}
	if taken, _ := store.CodeTaken("meals", 0); !taken {
		t.Fatal("the code of an archived budget is free")
	}
	if err := store.Restore(meals.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(meals.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second restore = %v, want ErrNotFound", err)
	}
	if err := store.Delete(meals.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find("MEALS", true, today); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find after delete = %v, want ErrNotFound", err)
	}
	var links int64
	if db.Model(&Link{}).Where("budget_id = ?", meals.ID).Count(&links); links != 0 {
		t.Fatalf("Delete left %d links", links)
	}
	if err := store.Delete(meals.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestStoreUsages(t *testing.T) {
	store, db := newStore(t)
	meals := create(t, store, "Meals", "MEALS", food)
	both := create(t, store, "Both", "BOTH", food, fun)
	all := create(t, store, "All", "ALL")
	rows := []movement{
		// October.
		{Type: "expense", Amount: -30000, CategoryID: food, AccountID: real, Date: "2026-10-01"},
		{Type: "expense", Amount: -20000, CategoryID: fun, CardID: real, Date: "2026-10-31"},
		// November.
		{Type: "expense", Amount: -50000, CategoryID: food, AccountID: real, Date: "2026-11-01"},
		{Type: "expense", Amount: -25000, CategoryID: food, CardID: real, Date: "2026-11-18"},
		{Type: "income", Amount: 5000, CategoryID: food, AccountID: real, Date: "2026-11-10"},
		{Type: "expense", Amount: -4000, CategoryID: fun, AccountID: real, Date: "2026-11-30"},
		{Type: "income", Amount: 900000, CategoryID: salary, AccountID: real, Date: "2026-11-05"},
		// These never count: another currency, a transfer, a bill payment,
		// a date before the start.
		{Type: "expense", Amount: -77700, CategoryID: food, AccountID: euro, Date: "2026-11-02"},
		{Type: "expense", Amount: -77700, CategoryID: food, CardID: euro, Date: "2026-11-02"},
		{Type: "transfer", Amount: -11100, CategoryID: food, AccountID: real, Date: "2026-11-03"},
		{Type: "transfer", Amount: 11100, CategoryID: food, AccountID: real, Date: "2026-11-03"},
		{Type: "payment", Amount: -22200, CategoryID: food, AccountID: real, Date: "2026-11-04"},
		{Type: "expense", Amount: -33300, CategoryID: food, AccountID: real, Date: "2026-09-30"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		code              string
		october, november int64
	}{
		{"MEALS", 30000, 70000},
		{"BOTH", 50000, 74000},
		// All categories leaves out the income categories: the salary does
		// not pay for the expenses.
		{"ALL", 50000, 74000},
	} {
		found, err := store.Find(tc.code, false, today)
		if err != nil || len(found.Usages) != 2 {
			t.Fatalf("%s: usages = %+v, %v", tc.code, found.Usages, err)
		}
		if found.Usages[0].Spent != tc.october || found.Usages[1].Spent != tc.november {
			t.Errorf("%s: spent %d and %d, want %d and %d", tc.code, found.Usages[0].Spent, found.Usages[1].Spent, tc.october, tc.november)
		}
	}
	budgets, err := store.List(false, today)
	if err != nil || len(budgets) != 3 {
		t.Fatalf("List = %d budgets, %v", len(budgets), err)
	}
	for _, budget := range budgets {
		if usage, ok := budget.Current(today); budget.Code == "MEALS" && (!ok || usage.Spent != 70000 || usage.Level() != LevelNear) {
			t.Errorf("List: MEALS current = %+v, %v", usage, ok)
		}
	}

	meals.Rollover = true
	if err := store.Update(&meals); err != nil {
		t.Fatal(err)
	}
	found, _ := store.Find("MEALS", false, today)
	if found.Usages[1].Carried != 50000 || found.Usages[1].Available() != 130000 {
		t.Errorf("rollover: %+v", found.Usages[1])
	}
	// A date before the start gives a budget without periods.
	if early, err := store.Find("MEALS", false, "2026-09-30"); err != nil || len(early.Usages) != 0 || early.Status("2026-09-30") != StatusUpcoming {
		t.Errorf("before the start: %+v, %v", early.Usages, err)
	}
	_, _ = both, all
}

func TestStoreMatching(t *testing.T) {
	store, _ := newStore(t)
	create(t, store, "Meals", "MEALS", food)
	create(t, store, "Both", "BOTH", food, fun)
	create(t, store, "All", "ALL")
	archived := create(t, store, "Old", "OLD", food)
	if err := store.Archive(archived.ID); err != nil {
		t.Fatal(err)
	}
	euros := Budget{Title: "Euros", Code: "EUROS", Currency: "EUR", Limit: 1000, StartDate: "2026-10-01", Every: 1, Unit: Month}
	if err := store.Create(&euros); err != nil {
		t.Fatal(err)
	}
	once := Budget{Title: "Once", Code: "ONCE", Currency: "BRL", Limit: 1000, StartDate: "2026-12-01", EndDate: "2026-12-31", CategoryIDs: []uint{food}}
	if err := store.Create(&once); err != nil {
		t.Fatal(err)
	}
	codes := func(budgets []Budget) []string {
		var codes []string
		for _, budget := range budgets {
			codes = append(codes, budget.Code)
		}
		return codes
	}
	for name, tc := range map[string]struct {
		category uint
		currency string
		date     string
		want     []string
	}{
		"food":            {food, "BRL", "2026-11-18", []string{"ALL", "BOTH", "MEALS"}},
		"fun":             {fun, "BRL", "2026-11-18", []string{"ALL", "BOTH"}},
		"income category": {salary, "BRL", "2026-11-18", nil},
		"another money":   {food, "EUR", "2026-11-18", []string{"EUROS"}},
		"in december":     {food, "BRL", "2026-12-10", []string{"ALL", "BOTH", "MEALS", "ONCE"}},
		"before any":      {food, "BRL", "2026-09-30", nil},
		"no such balance": {food, "USD", "2026-11-18", nil},
	} {
		budgets, err := store.Matching(tc.category, tc.currency, tc.date)
		if err != nil || !reflect.DeepEqual(codes(budgets), tc.want) {
			t.Errorf("%s: Matching = %v, %v; want %v", name, codes(budgets), err, tc.want)
		}
		for _, budget := range budgets {
			if _, ok := budget.UsageAt(tc.date); !ok {
				t.Errorf("%s: %s has no period on %s", name, budget.Code, tc.date)
			}
		}
	}
}

func TestStoreCategories(t *testing.T) {
	store, _ := newStore(t)
	meals := create(t, store, "Meals", "MEALS", food)
	create(t, store, "Both", "BOTH", food, fun)
	create(t, store, "All", "ALL")
	old := create(t, store, "Old", "OLD", food)
	if err := store.Archive(old.ID); err != nil {
		t.Fatal(err)
	}
	// An archived budget counts too: it can come back.
	only, err := store.OnlyCategoryOf(food)
	if err != nil || len(only) != 2 || only[0].Code != "MEALS" || only[1].Code != "OLD" {
		t.Fatalf("OnlyCategoryOf(food) = %+v, %v", only, err)
	}
	if only, err := store.OnlyCategoryOf(fun); err != nil || len(only) != 0 {
		t.Fatalf("OnlyCategoryOf(fun) = %+v, %v", only, err)
	}
	if err := store.DropCategory(fun); err != nil {
		t.Fatal(err)
	}
	if both, _ := store.Find("BOTH", false, today); !reflect.DeepEqual(both.CategoryIDs, []uint{food}) {
		t.Fatalf("after DropCategory: %v", both.CategoryIDs)
	}
	if found, _ := store.Find("MEALS", false, today); !reflect.DeepEqual(found.CategoryIDs, []uint{food}) {
		t.Fatalf("DropCategory changed another category: %v", found.CategoryIDs)
	}
	_ = meals
}

// A database from before the transactions module has no transactions
// table. Budgets then spent nothing.
func TestStoreWithoutTransactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Budget{}, &Link{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	create(t, store, "Meals", "MEALS", food)
	budgets, err := store.List(false, today)
	if err != nil || len(budgets) != 1 || len(budgets[0].Usages) != 2 || budgets[0].Usages[1].Spent != 0 {
		t.Fatalf("List = %+v, %v", budgets, err)
	}
}
