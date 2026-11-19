package transaction

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/storage"
)

// fixture is a database with accounts and the base categories.
type fixture struct {
	store      *Store
	accounts   *account.Store
	categories *category.Store
	cards      *card.Store
	goals      *goal.Store
	// ids by code
	account  map[string]uint
	category map[string]uint
	card     map[string]uint
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &account.Account{}, &category.Category{}, &card.Card{}, &card.Bill{}, &goal.Goal{}, &Transaction{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{
		store: NewStore(db), accounts: account.NewStore(db), categories: category.NewStore(db),
		cards: card.NewStore(db), goals: goal.NewStore(db),
		account: map[string]uint{}, category: map[string]uint{}, card: map[string]uint{},
	}
	for _, current := range []card.Card{
		{Name: "Visa Gold", Code: "VISA", Color: "blue", Currency: "BRL", Limit: 500000, ClosingDay: 25, DueDay: 5},
		{Name: "Amex", Code: "AMEX", Color: "teal", Currency: "USD", Limit: 200000, ClosingDay: 3, DueDay: 10},
	} {
		if err := f.cards.Create(&current); err != nil {
			t.Fatal(err)
		}
		f.card[current.Code] = current.ID
	}
	for _, current := range []account.Account{
		{Name: "Banco Inter", Code: "INTER", Color: "orange", Currency: "BRL", OpeningBalance: 100000},
		{Name: "Nubank", Code: "NUBAN", Color: "purple", Currency: "BRL"},
		{Name: "Wise", Code: "WISE", Color: "lime", Currency: "USD", OpeningBalance: 5000},
	} {
		if err := f.accounts.Create(&current); err != nil {
			t.Fatal(err)
		}
		f.account[current.Code] = current.ID
	}
	if _, err := f.categories.AddMissingBase(); err != nil {
		t.Fatal(err)
	}
	categories, err := f.categories.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range categories {
		f.category[current.Code] = current.ID
	}
	return f
}

func (f fixture) draft(title, kind, accountCode, categoryCode string, value int64) Draft {
	return Draft{Title: title, Type: kind, Value: value, AccountID: f.account[accountCode], CategoryID: f.category[categoryCode], Date: "2026-11-17"}
}

func (f fixture) create(t *testing.T, draft Draft) Transaction {
	t.Helper()
	created, err := f.store.Create(draft)
	if err != nil {
		t.Fatalf("Create(%+v): %v", draft, err)
	}
	return created
}

func (f fixture) balance(t *testing.T, code string) int64 {
	t.Helper()
	found, err := f.accounts.Find(code, true, fixtureToday)
	if err != nil {
		t.Fatal(err)
	}
	return found.Balance()
}

func TestStoreCreateAndFind(t *testing.T) {
	f := newFixture(t)
	draft := f.draft("  Weekly market ", Expense, "INTER", "FOOD", 5025)
	draft.Description, draft.Tags, draft.Date = " fruit ", "Market, #WEEKLY", "17/11/2026"
	created := f.create(t, draft)
	if created.ID == 0 || created.Title != "Weekly market" || created.Description != "fruit" || created.Amount != -5025 ||
		created.Tags != "#market #weekly" || created.Date != "2026-11-17" || created.PairID != 0 || created.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", created)
	}
	salary := f.create(t, f.draft("Salary", Income, "INTER", "SAL", 300000))
	if salary.Amount != 300000 {
		t.Fatalf("income amount = %d", salary.Amount)
	}

	view, err := f.store.Find(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != created.ID || view.Account.Code != "INTER" || view.Category.Code != "FOOD" || view.Other != nil {
		t.Fatalf("Find = %+v", view)
	}
	if _, err := f.store.Find(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find(999) error = %v", err)
	}
	// The balance of an account is its opening balance and its transactions.
	if got := f.balance(t, "INTER"); got != 100000-5025+300000 {
		t.Fatalf("balance = %d", got)
	}
	if got := f.balance(t, "NUBAN"); got != 0 {
		t.Fatalf("balance of account without transactions = %d", got)
	}
	accounts, err := f.accounts.List(false, fixtureToday)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range accounts {
		if current.Code == "INTER" && current.Balance() != 394975 {
			t.Fatalf("balance in list = %d", current.Balance())
		}
	}
}

func TestStoreCreateRules(t *testing.T) {
	f := newFixture(t)
	if err := f.accounts.Archive(f.account["NUBAN"]); err != nil {
		t.Fatal(err)
	}
	valid := f.draft("Market", Expense, "INTER", "FOOD", 100)
	for name, tc := range map[string]struct {
		change func(*Draft)
		want   string
	}{
		"no title":            {func(d *Draft) { d.Title = " " }, "title is required"},
		"long title":          {func(d *Draft) { d.Title = strings.Repeat("ã", MaxTitle+1) }, "title is too long"},
		"long description":    {func(d *Draft) { d.Description = strings.Repeat("a", MaxDescription+1) }, "description is too long"},
		"unknown type":        {func(d *Draft) { d.Type = "loan" }, "unknown type"},
		"zero value":          {func(d *Draft) { d.Value = 0 }, "more than zero"},
		"negative value":      {func(d *Draft) { d.Value = -5 }, "more than zero"},
		"bad date":            {func(d *Draft) { d.Date = "31/02/2026" }, "invalid date"},
		"bad tag":             {func(d *Draft) { d.Tags = "#a-b" }, "invalid tag"},
		"no account":          {func(d *Draft) { d.AccountID = 0 }, "account is required"},
		"unknown account":     {func(d *Draft) { d.AccountID = 999 }, "account not found"},
		"archived account":    {func(d *Draft) { d.AccountID = f.account["NUBAN"] }, "account not found"},
		"no category":         {func(d *Draft) { d.CategoryID = 0 }, "category is required"},
		"unknown category":    {func(d *Draft) { d.CategoryID = 999 }, "category not found"},
		"income category":     {func(d *Draft) { d.CategoryID = f.category["SAL"] }, "SAL"},
		"expense for income":  {func(d *Draft) { d.Type = Income }, "FOOD"},
		"destination":         {func(d *Draft) { d.ToAccountID = f.account["WISE"] }, "only a transfer"},
		"transfer, no to":     {func(d *Draft) { d.Type, d.CategoryID = Transfer, f.category["XFER"] }, "destination account is required"},
		"transfer, same":      {func(d *Draft) { d.Type, d.CategoryID, d.ToAccountID = Transfer, f.category["XFER"], d.AccountID }, "two different accounts"},
		"transfer, red":       {func(d *Draft) { d.Type, d.ToAccountID = Transfer, f.account["WISE"] }, "FOOD"},
		"transfer, archived":  {func(d *Draft) { d.Type, d.CategoryID, d.ToAccountID = Transfer, f.category["XFER"], f.account["NUBAN"] }, "account not found"},
		"transfer, no amount": {func(d *Draft) { d.Type, d.CategoryID, d.ToAccountID = Transfer, f.category["XFER"], f.account["WISE"] }, "received value is required"},
	} {
		draft := valid
		tc.change(&draft)
		if _, err := f.store.Create(draft); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", name, err, tc.want)
		}
	}
	if all, err := f.store.List(Filter{}); err != nil || len(all) != 0 {
		t.Fatalf("failed creates left %d transactions, %v", len(all), err)
	}
	// Grey categories accept all types.
	f.create(t, f.draft("Odd expense", Expense, "INTER", "OTHER", 1))
	f.create(t, f.draft("Odd income", Income, "INTER", "OTHER", 1))
	// The limits count characters, not bytes.
	long := f.draft(strings.Repeat("ã", MaxTitle), Expense, "INTER", "FOOD", 1)
	long.Description = strings.Repeat("é", MaxDescription)
	f.create(t, long)
}

func TestStoreTransfer(t *testing.T) {
	f := newFixture(t)
	draft := f.draft("To card", Transfer, "INTER", "XFER", 20000)
	draft.ToAccountID, draft.Tags = f.account["NUBAN"], "#card"
	out := f.create(t, draft)
	if out.Amount != -20000 || out.PairID == 0 || out.AccountID != f.account["INTER"] || out.Type != Transfer {
		t.Fatalf("outgoing side = %+v", out)
	}
	in, err := f.store.Find(out.PairID)
	if err != nil {
		t.Fatal(err)
	}
	if in.Amount != 20000 || in.PairID != out.ID || in.Account.Code != "NUBAN" || in.Type != Transfer || in.Title != "To card" ||
		in.Tags != "#card" || in.Date != out.Date || in.Category.Code != "XFER" || in.Other == nil || in.Other.Code != "INTER" {
		t.Fatalf("incoming side = %+v", in)
	}
	if view, _ := f.store.Find(out.ID); view.Other == nil || view.Other.Code != "NUBAN" {
		t.Fatalf("outgoing side has no other account: %+v", view)
	}
	if f.balance(t, "INTER") != 80000 || f.balance(t, "NUBAN") != 20000 {
		t.Fatalf("balances = %d, %d", f.balance(t, "INTER"), f.balance(t, "NUBAN"))
	}

	// Between currencies each side has its amount.
	abroad := f.draft("Travel money", Transfer, "INTER", "XFER", 55000)
	abroad.ToAccountID, abroad.Received = f.account["WISE"], 10000
	sent := f.create(t, abroad)
	received, _ := f.store.Find(sent.PairID)
	if sent.Amount != -55000 || received.Amount != 10000 || f.balance(t, "WISE") != 15000 || f.balance(t, "INTER") != 25000 {
		t.Fatalf("sent %d, received %d, balances %d and %d", sent.Amount, received.Amount, f.balance(t, "INTER"), f.balance(t, "WISE"))
	}
	// With one currency the two sides have one amount.
	wrong := f.draft("Wrong", Transfer, "INTER", "XFER", 100)
	wrong.ToAccountID, wrong.Received = f.account["NUBAN"], 90
	if _, err := f.store.Create(wrong); err == nil || !strings.Contains(err.Error(), "same currency") {
		t.Fatalf("different amounts in one currency: %v", err)
	}

	// A change of one side changes the other side.
	change := DraftOf(in.Transaction)
	change.Title, change.Value, change.Date, change.Tags, change.Description = "Card payment", 30000, "2026-12-01", "#bill", "december"
	updated, err := f.store.Update(in.ID, change)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := f.store.Find(out.ID)
	if updated.Amount != 30000 || other.Amount != -30000 || other.Title != "Card payment" || other.Date != "2026-12-01" ||
		other.Tags != "#bill" || other.Description != "december" {
		t.Fatalf("after update: this side %+v, other side %+v", updated, other.Transaction)
	}
	// Between currencies a new value changes only its side.
	change = DraftOf(sent)
	change.Value = 60000
	if _, err := f.store.Update(sent.ID, change); err != nil {
		t.Fatal(err)
	}
	if received, _ = f.store.Find(sent.PairID); received.Amount != 10000 {
		t.Fatalf("other currency side changed to %d", received.Amount)
	}
	for name, edit := range map[string]func(*Draft){
		"type":     func(d *Draft) { d.Type, d.CategoryID = Expense, f.category["FOOD"] },
		"account":  func(d *Draft) { d.AccountID = f.account["WISE"] },
		"category": func(d *Draft) { d.CategoryID = f.category["FOOD"] },
	} {
		change := DraftOf(out)
		edit(&change)
		if _, err := f.store.Update(out.ID, change); err == nil {
			t.Errorf("transfer accepted a change of %s", name)
		}
	}

	// Delete removes the two sides.
	if err := f.store.Delete(in.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Find(out.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other side after delete: %v", err)
	}
	if f.balance(t, "NUBAN") != 0 {
		t.Fatalf("balance after delete = %d", f.balance(t, "NUBAN"))
	}
}

func TestStoreUpdateAndDelete(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, f.draft("Market", Expense, "INTER", "FOOD", 5000))
	change := DraftOf(created)
	if change.Value != 5000 || change.Type != Expense || change.Title != "Market" {
		t.Fatalf("DraftOf = %+v", change)
	}
	change.Title, change.Type, change.CategoryID, change.Value, change.AccountID = "Refund", Income, f.category["OTHER"], 700, f.account["WISE"]
	change.Tags, change.Date, change.Description = "refund", "01/12/2026", "store credit"
	updated, err := f.store.Update(created.ID, change)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Title != "Refund" || updated.Amount != 700 || updated.AccountID != f.account["WISE"] ||
		updated.Tags != "#refund" || updated.Date != "2026-12-01" || updated.Description != "store credit" {
		t.Fatalf("updated = %+v", updated)
	}
	if f.balance(t, "INTER") != 100000 || f.balance(t, "WISE") != 5700 {
		t.Fatalf("balances after move = %d, %d", f.balance(t, "INTER"), f.balance(t, "WISE"))
	}
	for name, edit := range map[string]func(*Draft){
		"to transfer":    func(d *Draft) { d.Type, d.CategoryID, d.ToAccountID = Transfer, f.category["XFER"], f.account["INTER"] },
		"wrong category": func(d *Draft) { d.CategoryID = f.category["FOOD"] },
		"no title":       func(d *Draft) { d.Title = "" },
		"zero":           func(d *Draft) { d.Value = 0 },
	} {
		change := DraftOf(updated)
		edit(&change)
		if _, err := f.store.Update(updated.ID, change); err == nil {
			t.Errorf("update accepted: %s", name)
		}
	}
	if _, err := f.store.Update(999, DraftOf(updated)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update(999) error = %v", err)
	}
	// A transaction of an archived account can still change.
	if err := f.accounts.Archive(f.account["WISE"]); err != nil {
		t.Fatal(err)
	}
	change = DraftOf(updated)
	change.Title = "Refund 2"
	if _, err := f.store.Update(updated.ID, change); err != nil {
		t.Fatalf("update in archived account: %v", err)
	}
	if view, err := f.store.Find(updated.ID); err != nil || view.Account.Code != "WISE" || !view.Account.Archived() {
		t.Fatalf("view of archived account = %+v, %v", view, err)
	}

	if err := f.store.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Delete(created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
}

func TestStoreListAndSuggestions(t *testing.T) {
	f := newFixture(t)
	add := func(title, kind, accountCode, categoryCode, date, tags string) {
		draft := f.draft(title, kind, accountCode, categoryCode, 100)
		draft.Date, draft.Tags = date, tags
		f.create(t, draft)
	}
	add("Market", Expense, "INTER", "FOOD", "2026-11-17", "#market #weekly")
	add("Salary", Income, "INTER", "SAL", "2026-11-05", "")
	add("Rent", Expense, "NUBAN", "HOME", "2026-10-01", "#fixed")
	add("Market", Expense, "NUBAN", "FOOD", "2025-12-30", "#market")
	add("market", Expense, "INTER", "FOOD", "2026-11-17", "#Weekly")

	titles := func(filter Filter) string {
		t.Helper()
		views, err := f.store.List(filter)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, view := range views {
			names = append(names, view.Title+"@"+view.Account.Code)
		}
		return strings.Join(names, " ")
	}
	// Newest date first; in one day, the newest record first.
	if got := titles(Filter{}); got != "market@INTER Market@INTER Salary@INTER Rent@NUBAN Market@NUBAN" {
		t.Fatalf("order = %s", got)
	}
	for name, tc := range map[string]struct {
		filter Filter
		want   string
	}{
		"category": {Filter{Category: "HOME"}, "Rent@NUBAN"},
		"tag":      {Filter{Tags: []string{"#weekly"}}, "market@INTER Market@INTER"},
		"title":    {Filter{Title: "sal"}, "Salary@INTER"},
		"year":     {Filter{Period: "2025"}, "Market@NUBAN"},
		"month":    {Filter{Period: "2026-11"}, "market@INTER Market@INTER Salary@INTER"},
		"day":      {Filter{Period: "2026-10-01"}, "Rent@NUBAN"},
		"account":  {Filter{Account: "NUBAN"}, "Rent@NUBAN Market@NUBAN"},
		"type":     {Filter{Type: Income}, "Salary@INTER"},
		"none":     {Filter{Category: "TAX"}, ""},
	} {
		if got := titles(tc.filter); got != tc.want {
			t.Errorf("%s: %s, want %s", name, got, tc.want)
		}
	}

	// Titles: one entry for each title, the last one used first.
	names, err := f.store.Titles()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names, ","); got != "market,Rent,Salary" {
		t.Fatalf("Titles = %s", got)
	}
	tags, err := f.store.Tags()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tags, ","); got != "#fixed,#market,#weekly" {
		t.Fatalf("Tags = %s", got)
	}
}

func TestStoreCategoryRules(t *testing.T) {
	f := newFixture(t)
	f.create(t, f.draft("Market", Expense, "INTER", "FOOD", 100))
	f.create(t, f.draft("Lunch", Expense, "INTER", "FOOD", 100))
	f.create(t, f.draft("Found money", Income, "INTER", "OTHER", 100))
	food, other := f.category["FOOD"], f.category["OTHER"]

	if count, err := f.store.CountByCategory(food); err != nil || count != 2 {
		t.Fatalf("CountByCategory = %d, %v", count, err)
	}
	// Number of transactions that a new color of the category does not accept.
	for color, want := range map[string]int64{"red": 0, "grey": 0, "green": 2} {
		if got, err := f.store.Mismatched(food, color); err != nil || got != want {
			t.Errorf("Mismatched(FOOD, %s) = %d, %v; want %d", color, got, err, want)
		}
	}
	if got, _ := f.store.Mismatched(other, "red"); got != 1 {
		t.Fatalf("Mismatched(OTHER, red) = %d", got)
	}
	if err := f.store.MoveCategory(food, other); err != nil {
		t.Fatal(err)
	}
	if views, _ := f.store.List(Filter{Category: "OTHER"}); len(views) != 3 {
		t.Fatalf("OTHER has %d transactions after the move", len(views))
	}
	if count, _ := f.store.CountByCategory(food); count != 0 {
		t.Fatalf("FOOD still has %d transactions", count)
	}
}

func TestStorePurgeAccount(t *testing.T) {
	f := newFixture(t)
	f.create(t, f.draft("Market", Expense, "INTER", "FOOD", 100))
	f.create(t, f.draft("Salary", Income, "INTER", "SAL", 100))
	kept := f.create(t, f.draft("Coffee", Expense, "NUBAN", "FOOD", 300))
	out := f.draft("To card", Transfer, "INTER", "XFER", 20000)
	out.ToAccountID, out.Description = f.account["NUBAN"], "monthly"
	sent := f.create(t, out)
	back := f.draft("From abroad", Transfer, "WISE", "XFER", 1000)
	back.ToAccountID, back.Received = f.account["INTER"], 5500
	returned := f.create(t, back)

	if plain, transfers, err := f.store.AccountUsage(f.account["INTER"]); err != nil || plain != 2 || transfers != 2 {
		t.Fatalf("AccountUsage = %d, %d, %v", plain, transfers, err)
	}
	inter, _ := f.accounts.Find("INTER", true, fixtureToday)
	other, _ := f.categories.Find("OTHER")
	if err := f.store.PurgeAccount(inter, other); err != nil {
		t.Fatal(err)
	}
	views, err := f.store.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 3 {
		t.Fatalf("%d transactions left, want 3: %+v", len(views), views)
	}
	// The side in the other account stays as an income or an expense.
	income, err := f.store.Find(sent.PairID)
	if err != nil {
		t.Fatal(err)
	}
	if income.Type != Income || income.Amount != 20000 || income.PairID != 0 || income.Category.Code != "OTHER" || income.Other != nil ||
		income.Description != "monthly. Transfer from INTER (deleted account)" {
		t.Fatalf("incoming side = %+v", income.Transaction)
	}
	expense, err := f.store.Find(returned.ID)
	if err != nil {
		t.Fatal(err)
	}
	if expense.Type != Expense || expense.Amount != -1000 || expense.PairID != 0 || expense.Category.Code != "OTHER" ||
		expense.Description != "Transfer to INTER (deleted account)" {
		t.Fatalf("outgoing side = %+v", expense.Transaction)
	}
	if _, err := f.store.Find(kept.ID); err != nil {
		t.Fatalf("transaction of another account: %v", err)
	}
	if f.balance(t, "NUBAN") != 19700 || f.balance(t, "WISE") != 4000 {
		t.Fatalf("balances changed: %d, %d", f.balance(t, "NUBAN"), f.balance(t, "WISE"))
	}

	// The note never makes the description longer than the limit.
	long := f.draft("Long", Transfer, "NUBAN", "XFER", 100)
	long.ToAccountID, long.Received, long.Description = f.account["WISE"], 20, strings.Repeat("é", MaxDescription)
	side := f.create(t, long)
	nubank, _ := f.accounts.Find("NUBAN", true, fixtureToday)
	if err := f.store.PurgeAccount(nubank, other); err != nil {
		t.Fatal(err)
	}
	left, err := f.store.Find(side.PairID)
	if err != nil {
		t.Fatal(err)
	}
	if got := []rune(left.Description); len(got) != MaxDescription || !strings.HasSuffix(left.Description, "…. Transfer from NUBAN (deleted account)") {
		t.Fatalf("long description = %d characters: %q", len(got), left.Description)
	}
}

// fixtureToday is the date of today that the balance helper uses. It is
// after all the dates of the tests.
const fixtureToday = "2026-12-31"

func TestBalanceCountsUpToToday(t *testing.T) {
	f := newFixture(t)
	f.create(t, f.draft("Market", Expense, "INTER", "FOOD", 1000))
	later := f.draft("Rent", Expense, "INTER", "HOME", 5000)
	later.Date = "2026-11-18"
	f.create(t, later)
	for today, want := range map[string]int64{"2026-11-16": 100000, "2026-11-17": 99000, "2026-11-18": 94000} {
		found, err := f.accounts.Find("INTER", false, today)
		if err != nil || found.Balance() != want {
			t.Errorf("Find on %s: balance = %d, %v; want %d", today, found.Balance(), err, want)
		}
		accounts, err := f.accounts.List(false, today)
		if err != nil || accounts[0].Code != "INTER" || accounts[0].Balance() != want {
			t.Errorf("List on %s: %+v, %v; want %d", today, accounts, err, want)
		}
	}
	if err := f.accounts.Archive(f.account["INTER"]); err != nil {
		t.Fatal(err)
	}
	if archived, err := f.accounts.ListArchived("2026-11-17"); err != nil || len(archived) != 1 || archived[0].Balance() != 99000 {
		t.Errorf("ListArchived = %+v, %v", archived, err)
	}
}
