package transaction

import (
	"reflect"
	"testing"

	"finux/src/goal"
)

// parts returns the parts of an installment purchase, the first one first.
func (f fixture) parts(t *testing.T, id uint) []Transaction {
	t.Helper()
	parts, err := f.store.Parts(id, All)
	if err != nil {
		t.Fatal(err)
	}
	return parts
}

// installments returns a draft of an expense in parts on an account.
func (f fixture) installments(title, accountCode string, total int64, count int) Draft {
	draft := f.draft(title, Expense, accountCode, "SHOP", total)
	draft.Installments = count
	return draft
}

// each returns one value of each part.
func each[T any](parts []Transaction, value func(Transaction) T) []T {
	values := make([]T, len(parts))
	for i, part := range parts {
		values[i] = value(part)
	}
	return values
}

func amounts(parts []Transaction) []int64 {
	return each(parts, func(t Transaction) int64 { return t.Amount })
}
func dates(parts []Transaction) []string {
	return each(parts, func(t Transaction) string { return t.Date })
}
func numbers(parts []Transaction) []int {
	return each(parts, func(t Transaction) int { return t.Part })
}

func (f fixture) periods(t *testing.T, parts []Transaction) []string {
	t.Helper()
	return each(parts, func(part Transaction) string { return f.bill(t, part.BillID).Period })
}

func want[T any](t *testing.T, name string, got, wanted T) {
	t.Helper()
	if !reflect.DeepEqual(got, wanted) {
		t.Fatalf("%s = %v, want %v", name, got, wanted)
	}
}

func (f fixture) count(t *testing.T) int {
	t.Helper()
	views, err := f.store.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return len(views)
}

func TestInstallmentHelpers(t *testing.T) {
	if got := (Transaction{}).Installment(); got != "" {
		t.Errorf("Installment of a normal transaction = %q", got)
	}
	if got := (Transaction{InstallmentID: 7, Part: 3, Parts: 10}).Installment(); got != "3/10" {
		t.Errorf("Installment = %q", got)
	}
	want(t, "split", split(1000, 3), []int64{334, 333, 333})
	want(t, "split", split(1200, 4), []int64{300, 300, 300, 300})
	for months, wanted := range map[int]string{0: "2027-01-31", 1: "2027-02-28", 2: "2027-03-31", 3: "2027-04-30", 13: "2028-02-29", -1: "2026-12-31"} {
		if got := monthsLater("2027-01-31", months); got != wanted {
			t.Errorf("monthsLater(2027-01-31, %d) = %q, want %q", months, got, wanted)
		}
	}
}

func TestUpdateKeepsTheBill(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	visa := f.visa(t)
	visa.ClosingDay = 10
	if err := f.cards.Update(&visa); err != nil {
		t.Fatal(err)
	}
	// With the new closing day the date gives another bill. A change
	// that does not move the transaction keeps its bill.
	draft := DraftOf(market)
	draft.Title = "Weekly market"
	updated, err := f.store.Update(market.ID, draft)
	if err != nil || updated.BillID != market.BillID {
		t.Fatalf("Update of the title: bill %d, %v; want bill %d", updated.BillID, err, market.BillID)
	}
	draft.Date = "2026-11-18"
	if updated, err = f.store.Update(market.ID, draft); err != nil || f.bill(t, updated.BillID).Period != "2027-01" {
		t.Fatalf("Update of the date: %+v, %v; want the bill 2027-01", updated, err)
	}
}

func TestCreateInstallmentsOnAnAccount(t *testing.T) {
	f := newFixture(t)
	trip := f.goal(t, "TRIP", goal.Save, "BRL")
	draft := f.installments("Sofa", "INTER", 1000, 3)
	draft.GoalID, draft.Tags, draft.Description = trip.ID, "home", "three parts"
	first := f.create(t, draft)
	if first.Part != 1 || first.Parts != 3 || first.InstallmentID != first.ID || first.Amount != -334 {
		t.Fatalf("first part = %+v", first)
	}
	parts := f.parts(t, first.ID)
	want(t, "amounts", amounts(parts), []int64{-334, -333, -333})
	want(t, "dates", dates(parts), []string{"2026-11-17", "2026-12-17", "2027-01-17"})
	want(t, "numbers", numbers(parts), []int{1, 2, 3})
	for _, part := range parts {
		if part.InstallmentID != first.ID || part.Parts != 3 || part.Title != "Sofa" || part.GoalID != trip.ID || part.Tags != "#home" ||
			part.Description != "three parts" || part.Type != Expense || part.AccountID != f.account["INTER"] || part.BillID != 0 {
			t.Fatalf("part = %+v", part)
		}
	}
	// A part changes the balance on its date.
	for today, wanted := range map[string]int64{"2026-11-17": 100000 - 334, "2026-12-17": 100000 - 667, "2027-01-17": 99000} {
		if found, _ := f.accounts.Find("INTER", false, today); found.Balance() != wanted {
			t.Errorf("balance on %s = %d, want %d", today, found.Balance(), wanted)
		}
	}
	if saved, links := f.saved(t, "TRIP"); saved != -1000 || links != 3 {
		t.Errorf("goal: saved %d, links %d", saved, links)
	}

	// The parts keep the day of the first date. A short month uses its last day.
	draft = f.installments("Course", "INTER", 900, 3)
	draft.Type, draft.CategoryID, draft.Date = Income, f.category["SAL"], "2027-01-31"
	parts = f.parts(t, f.create(t, draft).ID)
	want(t, "dates from day 31", dates(parts), []string{"2027-01-31", "2027-02-28", "2027-03-31"})
	want(t, "income amounts", amounts(parts), []int64{300, 300, 300})

	// A purchase in progress has only its last parts. The value is the
	// total of all the parts, so the cents of the first part are not there.
	draft = f.installments("Phone", "INTER", 1205, 10)
	draft.From = 4
	fourth := f.create(t, draft)
	parts = f.parts(t, fourth.ID)
	if fourth.Part != 4 || fourth.InstallmentID != fourth.ID || len(parts) != 7 {
		t.Fatalf("purchase in progress: first %+v, %d parts", fourth, len(parts))
	}
	want(t, "numbers", numbers(parts), []int{4, 5, 6, 7, 8, 9, 10})
	want(t, "amounts", amounts(parts), []int64{-120, -120, -120, -120, -120, -120, -120})
	if parts[0].Date != "2026-11-17" || parts[6].Date != "2027-05-17" {
		t.Fatalf("dates = %v", dates(parts))
	}
}

func TestCreateInstallmentsChecksTheDraft(t *testing.T) {
	f := newFixture(t)
	before := f.count(t)
	for _, tc := range []struct {
		want   string
		change func(*Draft)
	}{
		{"installments must be from 2 to 120", func(d *Draft) { d.Installments = 121 }},
		{"installments must be from 2 to 120", func(d *Draft) { d.Installments = -1 }},
		{"first installment must be from 1 to 3", func(d *Draft) { d.From = 4 }},
		{"first installment must be from 1 to 3", func(d *Draft) { d.From = -1 }},
		{"first installment is only for a transaction in installments", func(d *Draft) { d.Installments, d.From = 0, 2 }},
		{"value is too small for 3 installments", func(d *Draft) { d.Value = 2 }},
		{"a transfer cannot have installments", func(d *Draft) {
			d.Type, d.ToAccountID, d.CategoryID = Transfer, f.account["NUBAN"], f.category["XFER"]
		}},
		{"goal not found", func(d *Draft) { d.GoalID = 99 }},
		{"category SAL", func(d *Draft) { d.CategoryID = f.category["SAL"] }},
	} {
		draft := f.installments("Sofa", "INTER", 1000, 3)
		tc.change(&draft)
		_, err := f.store.Create(draft)
		wantError(t, tc.want, err, tc.want)
	}
	if got := f.count(t); got != before {
		t.Fatalf("%d transactions after refused drafts, want %d", got, before)
	}
	// One installment is a normal transaction.
	draft := f.installments("Sofa", "INTER", 1000, 1)
	if created := f.create(t, draft); created.InstallmentID != 0 || created.Part != 0 || created.Parts != 0 || created.Amount != -1000 {
		t.Fatalf("one installment = %+v", created)
	}
}

func TestCreateInstallmentsOnACard(t *testing.T) {
	f := newFixture(t)
	visa := f.visa(t)
	visa.ClosingDay = 28
	if err := f.cards.Update(&visa); err != nil {
		t.Fatal(err)
	}
	draft := f.purchase("Notebook", "VISA", "SHOP", 300000, "2027-01-31")
	draft.Installments = 3
	parts := f.parts(t, f.create(t, draft).ID)
	want(t, "dates", dates(parts), []string{"2027-01-31", "2027-02-28", "2027-03-31"})
	// Each part goes to the bill after the one before, also when the
	// date of the part gives the same bill again.
	want(t, "bills", f.periods(t, parts), []string{"2027-03", "2027-04", "2027-05"})
	// All the parts use the limit from the first day.
	if used := f.visa(t).Used; used != 300000 {
		t.Fatalf("used = %d", used)
	}

	// A paid bill in the way stops all the parts.
	f = newFixture(t)
	january := f.create(t, f.purchase("Gift", "VISA", "SHOP", 5000, "2026-12-01"))
	if _, err := f.store.PayBill(january.BillID, f.account["INTER"], 0, "2026-12-26"); err != nil {
		t.Fatal(err)
	}
	before := f.count(t)
	draft = f.purchase("Notebook", "VISA", "SHOP", 300000, "2026-11-17")
	draft.Installments = 3
	_, err := f.store.Create(draft)
	wantError(t, "paid bill", err, "installment 2/3: bill 01/2027 of VISA is paid")
	if got := f.count(t); got != before {
		t.Fatalf("%d transactions after a refused purchase, want %d", got, before)
	}
	draft.Installments, draft.From, draft.Date = 3, 3, "2026-12-27"
	if third := f.create(t, draft); third.Part != 3 || f.bill(t, third.BillID).Period != "2027-02" {
		t.Fatalf("third part = %+v", third)
	}
}

func TestPartsAndDeleteParts(t *testing.T) {
	f := newFixture(t)
	normal := f.create(t, f.draft("Market", Expense, "INTER", "FOOD", 1000))
	if parts, err := f.store.Parts(normal.ID, One); err != nil || len(parts) != 1 || parts[0].ID != normal.ID {
		t.Fatalf("Parts of a normal transaction = %+v, %v", parts, err)
	}
	_, err := f.store.Parts(normal.ID, All)
	wantError(t, "Parts all", err, "is not an installment")
	_, err = f.store.DeleteParts(normal.ID, Next)
	wantError(t, "DeleteParts next", err, "is not an installment")
	if _, err := f.store.Parts(999, One); err != ErrNotFound {
		t.Fatalf("Parts(999) = %v", err)
	}

	parts := f.parts(t, f.create(t, f.installments("Sofa", "INTER", 1000, 5)).ID)
	if next, err := f.store.Parts(parts[2].ID, Next); err != nil || !reflect.DeepEqual(numbers(next), []int{3, 4, 5}) {
		t.Fatalf("Parts next = %v, %v", numbers(next), err)
	}
	if one, err := f.store.Parts(parts[2].ID, One); err != nil || !reflect.DeepEqual(numbers(one), []int{3}) {
		t.Fatalf("Parts one = %v, %v", numbers(one), err)
	}
	deleted, err := f.store.DeleteParts(parts[3].ID, Next)
	if err != nil {
		t.Fatal(err)
	}
	want(t, "deleted", deleted, []uint{parts[3].ID, parts[4].ID})
	// The first part can go; the others keep their group and numbers.
	if err := f.store.Delete(parts[0].ID); err != nil {
		t.Fatal(err)
	}
	want(t, "numbers after deletes", numbers(f.parts(t, parts[1].ID)), []int{2, 3})
	if deleted, err = f.store.DeleteParts(parts[2].ID, All); err != nil || len(deleted) != 2 {
		t.Fatalf("DeleteParts all = %v, %v", deleted, err)
	}
	if got := f.count(t); got != 1 {
		t.Fatalf("%d transactions left, want the normal one", got)
	}
}

func TestUpdateParts(t *testing.T) {
	f := newFixture(t)
	parts := f.parts(t, f.create(t, f.installments("Sofa", "INTER", 1000, 3)).ID)

	// A text change goes to each part of the scope. The values stay.
	draft := DraftOf(parts[1])
	draft.Title, draft.CategoryID, draft.Tags = "Couch", f.category["HOME"], "living"
	updated, err := f.store.UpdateParts(parts[1].ID, Next, draft, Changed{Title: true, Category: true, Tags: true})
	if err != nil || len(updated) != 2 {
		t.Fatalf("UpdateParts next = %+v, %v", updated, err)
	}
	parts = f.parts(t, parts[0].ID)
	want(t, "titles", each(parts, func(t Transaction) string { return t.Title }), []string{"Sofa", "Couch", "Couch"})
	want(t, "tags", each(parts, func(t Transaction) string { return t.Tags }), []string{"", "#living", "#living"})
	want(t, "amounts", amounts(parts), []int64{-334, -333, -333})
	if parts[0].CategoryID != f.category["SHOP"] || parts[2].CategoryID != f.category["HOME"] {
		t.Fatalf("categories = %d, %d", parts[0].CategoryID, parts[2].CategoryID)
	}
	// A draft field that is not marked as changed does not go to the parts.
	draft = DraftOf(parts[2])
	draft.Title, draft.Description = "Ignored", "A note"
	if _, err := f.store.UpdateParts(parts[2].ID, All, draft, Changed{Description: true}); err != nil {
		t.Fatal(err)
	}
	parts = f.parts(t, parts[0].ID)
	want(t, "titles", each(parts, func(t Transaction) string { return t.Title }), []string{"Sofa", "Couch", "Couch"})
	want(t, "dates", dates(parts), []string{"2026-11-17", "2026-12-17", "2027-01-17"})
	if parts[0].Description != "A note" || parts[2].Description != "A note" {
		t.Fatalf("descriptions = %q, %q", parts[0].Description, parts[2].Description)
	}

	// A value is the new total of the parts of the scope.
	draft = DraftOf(parts[1])
	draft.Value = 1001
	if _, err := f.store.UpdateParts(parts[1].ID, Next, draft, Changed{Value: true}); err != nil {
		t.Fatal(err)
	}
	want(t, "amounts", amounts(f.parts(t, parts[0].ID)), []int64{-334, -501, -500})

	// A date is the date of the first part of the scope. The others
	// follow by their numbers, also after a part is gone.
	if err := f.store.Delete(parts[1].ID); err != nil {
		t.Fatal(err)
	}
	draft = DraftOf(parts[2])
	draft.Date = "2027-01-31"
	if _, err := f.store.UpdateParts(parts[2].ID, All, draft, Changed{Date: true}); err != nil {
		t.Fatal(err)
	}
	want(t, "dates", dates(f.parts(t, parts[0].ID)), []string{"2027-01-31", "2027-03-31"})

	// An account or a card goes to each part of the scope.
	draft = DraftOf(parts[0])
	draft.AccountID, draft.CardID = 0, f.card["VISA"]
	if _, err := f.store.UpdateParts(parts[0].ID, All, draft, Changed{Place: true}); err != nil {
		t.Fatal(err)
	}
	parts = f.parts(t, parts[0].ID)
	want(t, "bills", f.periods(t, parts), []string{"2027-03", "2027-05"})
	if parts[0].AccountID != 0 || parts[1].CardID != f.card["VISA"] {
		t.Fatalf("parts on the card = %+v", parts)
	}

	normal := f.create(t, f.draft("Market", Expense, "INTER", "FOOD", 1000))
	_, err = f.store.UpdateParts(normal.ID, All, DraftOf(normal), Changed{Title: true})
	wantError(t, "UpdateParts of a normal transaction", err, "is not an installment")
	draft = DraftOf(parts[0])
	draft.Value = 1
	_, err = f.store.UpdateParts(parts[0].ID, All, draft, Changed{Value: true})
	wantError(t, "small total", err, "value is too small for 2 installments")
}

func TestPartsInAPaidBillAreLocked(t *testing.T) {
	f := newFixture(t)
	draft := f.purchase("Notebook", "VISA", "SHOP", 300000, "2026-11-17")
	draft.Installments = 3
	parts := f.parts(t, f.create(t, draft).ID)
	want(t, "bills", f.periods(t, parts), []string{"2026-12", "2027-01", "2027-02"})
	if _, err := f.store.PayBill(parts[0].BillID, f.account["INTER"], 0, "2026-11-26"); err != nil {
		t.Fatal(err)
	}
	change := DraftOf(parts[1])
	change.Title = "Laptop"
	_, err := f.store.UpdateParts(parts[1].ID, All, change, Changed{Title: true})
	wantError(t, "update all", err, "installment 1/3 (transaction 1): bill 12/2026 of VISA is paid")
	_, err = f.store.DeleteParts(parts[1].ID, All)
	wantError(t, "delete all", err, "installment 1/3 (transaction 1): bill 12/2026 of VISA is paid")
	// Nothing changed: an operation on a scope is all or nothing.
	parts = f.parts(t, parts[0].ID)
	want(t, "titles", each(parts, func(t Transaction) string { return t.Title }), []string{"Notebook", "Notebook", "Notebook"})

	// The parts after the paid bill can change. A change of the text
	// keeps their bills.
	if _, err := f.store.UpdateParts(parts[1].ID, Next, change, Changed{Title: true}); err != nil {
		t.Fatal(err)
	}
	parts = f.parts(t, parts[0].ID)
	want(t, "titles", each(parts, func(t Transaction) string { return t.Title }), []string{"Notebook", "Laptop", "Laptop"})
	want(t, "bills", f.periods(t, parts), []string{"2026-12", "2027-01", "2027-02"})
	// A date of the scope cannot put a part in the paid bill.
	change.Date = "2026-11-20"
	_, err = f.store.UpdateParts(parts[1].ID, Next, change, Changed{Date: true})
	wantError(t, "move into a paid bill", err, "installment 2/3 (transaction 2): bill 12/2026 of VISA is paid")
	change.Date = "2026-12-26"
	if _, err := f.store.UpdateParts(parts[1].ID, Next, change, Changed{Date: true}); err != nil {
		t.Fatal(err)
	}
	parts = f.parts(t, parts[0].ID)
	want(t, "bills after the move", f.periods(t, parts), []string{"2026-12", "2027-02", "2027-03"})
	if deleted, err := f.store.DeleteParts(parts[1].ID, Next); err != nil || len(deleted) != 2 {
		t.Fatalf("delete next = %v, %v", deleted, err)
	}
}
