package transaction

import (
	"strings"
	"testing"

	"finux/src/goal"
)

func (f fixture) goal(t *testing.T, code, kind, currency string) goal.Goal {
	t.Helper()
	created := goal.Goal{Title: "Goal " + code, Code: code, Type: kind, Currency: currency, Target: 100000}
	if err := f.goals.Create(&created); err != nil {
		t.Fatal(err)
	}
	return created
}

func (f fixture) saved(t *testing.T, code string) (saved, links int64) {
	t.Helper()
	found, err := f.goals.Find(code, true)
	if err != nil {
		t.Fatal(err)
	}
	return found.Saved, found.Links
}

func (f fixture) linked(title, kind, accountCode, categoryCode string, value int64, goalID uint) Draft {
	draft := f.draft(title, kind, accountCode, categoryCode, value)
	draft.GoalID = goalID
	return draft
}

func TestGoalLinks(t *testing.T) {
	f := newFixture(t)
	trip := f.goal(t, "TRIP", goal.Save, "BRL")
	debt := f.goal(t, "DEBT", goal.PayOff, "BRL")

	income := f.create(t, f.linked("Bonus", Income, "INTER", "SAL", 50000, trip.ID))
	if income.GoalID != trip.ID {
		t.Fatalf("created GoalID = %d, want %d", income.GoalID, trip.ID)
	}
	f.create(t, f.linked("Withdrawal", Expense, "INTER", "OTHER", 10000, trip.ID))
	if saved, links := f.saved(t, "TRIP"); saved != 40000 || links != 2 {
		t.Fatalf("TRIP saved %d, links %d; want 40000, 2", saved, links)
	}
	view, err := f.store.Find(income.ID)
	if err != nil || view.Goal == nil || view.Goal.Code != "TRIP" {
		t.Fatalf("view goal = %+v, %v", view.Goal, err)
	}
	plain := f.create(t, f.draft("Market", Expense, "INTER", "FOOD", 3000))
	if view, _ := f.store.Find(plain.ID); view.Goal != nil || view.GoalID != 0 {
		t.Fatalf("a transaction without a goal has the goal %+v", view.Goal)
	}

	// A purchase on a credit card links as an expense.
	draft := f.linked("Loan part", Expense, "", "OTHER", 30000, debt.ID)
	draft.AccountID, draft.CardID = 0, f.card["VISA"]
	f.create(t, draft)
	if saved, _ := f.saved(t, "DEBT"); saved != 30000 {
		t.Fatalf("DEBT saved %d, want 30000", saved)
	}
}

func TestGoalTransfer(t *testing.T) {
	f := newFixture(t)
	trip := f.goal(t, "TRIP", goal.Save, "BRL")
	draft := f.linked("To savings", Transfer, "INTER", "XFER", 20000, trip.ID)
	draft.ToAccountID = f.account["NUBAN"]
	created := f.create(t, draft)
	pair, err := f.store.Find(created.PairID)
	if err != nil || pair.GoalID != trip.ID || created.GoalID != trip.ID {
		t.Fatalf("goal of the two sides = %d and %d, %v; want %d", created.GoalID, pair.GoalID, err, trip.ID)
	}
	if saved, links := f.saved(t, "TRIP"); saved != 20000 || links != 1 {
		t.Fatalf("TRIP saved %d, links %d; want 20000, 1", saved, links)
	}
	// A change of one side changes the goal of the two sides.
	other := f.goal(t, "CAR", goal.Purchase, "BRL")
	change := DraftOf(pair.Transaction)
	change.GoalID = other.ID
	if _, err := f.store.Update(pair.ID, change); err != nil {
		t.Fatal(err)
	}
	if first, _ := f.store.Find(created.ID); first.GoalID != other.ID {
		t.Fatalf("other side has the goal %d, want %d", first.GoalID, other.ID)
	}
	if saved, _ := f.saved(t, "TRIP"); saved != 0 {
		t.Fatalf("TRIP saved %d after the move, want 0", saved)
	}
	if saved, links := f.saved(t, "CAR"); saved != 20000 || links != 1 {
		t.Fatalf("CAR saved %d, links %d; want 20000, 1", saved, links)
	}
}

func TestGoalLinkRules(t *testing.T) {
	f := newFixture(t)
	trip := f.goal(t, "TRIP", goal.Save, "BRL")
	old := f.goal(t, "OLD", goal.Save, "BRL")
	linked := f.create(t, f.linked("Before", Income, "INTER", "SAL", 1000, old.ID))
	if err := f.goals.Archive(old.ID); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		draft Draft
		want  string
	}{
		"unknown goal":  {f.linked("A", Income, "INTER", "SAL", 1000, 999), "goal not found"},
		"archived goal": {f.linked("A", Income, "INTER", "SAL", 1000, old.ID), "goal OLD is archived"},
		"currency":      {f.linked("A", Income, "WISE", "SAL", 1000, trip.ID), "goal TRIP is in BRL, not in USD"},
	} {
		if _, err := f.store.Create(tc.draft); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", name, err, tc.want)
		}
	}

	// A transaction keeps the link to a goal that is archived now.
	change := DraftOf(linked)
	change.Title = "After"
	updated, err := f.store.Update(linked.ID, change)
	if err != nil || updated.GoalID != old.ID || updated.Title != "After" {
		t.Fatalf("update with an archived goal = %+v, %v", updated, err)
	}
	// It cannot go to an account of a different currency with its goal.
	change.AccountID = f.account["WISE"]
	if _, err := f.store.Update(linked.ID, change); err == nil || !strings.Contains(err.Error(), "goal OLD is in BRL, not in USD") {
		t.Fatalf("currency change error = %v", err)
	}
	// It can lose the link, and then it cannot get the archived goal back.
	change = DraftOf(linked)
	change.GoalID = 0
	if updated, err := f.store.Update(linked.ID, change); err != nil || updated.GoalID != 0 {
		t.Fatalf("unlink = %+v, %v", updated, err)
	}
	change.GoalID = old.ID
	if _, err := f.store.Update(linked.ID, change); err == nil || !strings.Contains(err.Error(), "goal OLD is archived") {
		t.Fatalf("link to an archived goal error = %v", err)
	}
	change.GoalID = trip.ID
	if updated, err := f.store.Update(linked.ID, change); err != nil || updated.GoalID != trip.ID {
		t.Fatalf("link to an active goal = %+v, %v", updated, err)
	}
}

func TestGoalFilterAndClear(t *testing.T) {
	f := newFixture(t)
	trip := f.goal(t, "TRIP", goal.Save, "BRL")
	car := f.goal(t, "CAR", goal.Purchase, "BRL")
	first := f.create(t, f.linked("One", Income, "INTER", "SAL", 1000, trip.ID))
	transfer := f.linked("Two", Transfer, "INTER", "XFER", 2000, trip.ID)
	transfer.ToAccountID = f.account["NUBAN"]
	f.create(t, transfer)
	f.create(t, f.linked("Three", Income, "INTER", "SAL", 3000, car.ID))
	f.create(t, f.draft("Four", Income, "INTER", "SAL", 4000))

	views, err := f.store.List(Filter{Goal: "trip"})
	if err != nil || len(views) != 3 {
		t.Fatalf("List by goal = %d transactions, %v; want 3", len(views), err)
	}
	if got := (Filter{Goal: "TRIP"}).String(); got != "goal:TRIP" {
		t.Errorf("Filter.String() = %q", got)
	}
	if parsed := ParseSearch("goal:tri one"); parsed.Goal != "tri" || parsed.Title != "one" {
		t.Errorf("ParseSearch = %+v", parsed)
	}
	if views, _ := f.store.List(ParseSearch("goal:tri")); len(views) != 3 {
		t.Errorf("search goal:tri = %d transactions, want 3", len(views))
	}
	byGoal, err := f.store.ByGoal(trip.ID)
	if err != nil || len(byGoal) != 3 {
		t.Fatalf("ByGoal = %d, %v; want 3", len(byGoal), err)
	}

	if err := f.store.ClearGoal(trip.ID); err != nil {
		t.Fatal(err)
	}
	if views, _ := f.store.List(Filter{Goal: "TRIP"}); len(views) != 0 {
		t.Fatalf("after ClearGoal, %d transactions have the goal", len(views))
	}
	if view, err := f.store.Find(first.ID); err != nil || view.GoalID != 0 || view.Title != "One" {
		t.Fatalf("after ClearGoal the transaction = %+v, %v", view.Transaction, err)
	}
	if views, _ := f.store.List(Filter{Goal: "CAR"}); len(views) != 1 {
		t.Fatalf("ClearGoal changed a different goal")
	}
	if views, _ := f.store.List(Filter{}); len(views) != 5 {
		t.Fatalf("ClearGoal deleted transactions: %d left, want 5", len(views))
	}
}

func TestPaymentHasNoGoal(t *testing.T) {
	f := newFixture(t)
	owner, err := f.cards.Find("VISA", false)
	if err != nil {
		t.Fatal(err)
	}
	purchase := f.draft("Fridge", Expense, "", "SHOP", 10000)
	purchase.AccountID, purchase.CardID, purchase.Date = 0, owner.ID, "2026-10-20"
	created := f.create(t, purchase)
	payment, err := f.store.PayBill(created.BillID, f.account["INTER"], 0, "2026-11-17")
	if err != nil || payment.GoalID != 0 {
		t.Fatalf("payment = %+v, %v", payment, err)
	}
}
