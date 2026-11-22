package transaction

import (
	"testing"

	"finux/src/card"
)

func TestTransactionStatusFollowsTheDate(t *testing.T) {
	t.Setenv("FINUX_TODAY", "2026-11-20")
	f := newFixture(t)
	past := f.draft("Lunch", Expense, "INTER", "FOOD", 4500)
	past.Date = "2026-11-20"
	future := f.draft("Rent", Expense, "INTER", "OTHER", 90000)
	future.Date = "2026-11-21"
	done, waits := f.create(t, past), f.create(t, future)
	if done.Status != StatusCleared || waits.Status != StatusPending {
		t.Fatalf("statuses = %q and %q, want cleared and pending", done.Status, waits.Status)
	}
	if view, err := f.store.Find(waits.ID); err != nil || view.Status != StatusPending {
		t.Fatalf("stored status = %q, %v", view.Status, err)
	}

	if _, err := f.store.SyncStatuses("2026-11-21"); err != nil {
		t.Fatal(err)
	}
	if view, err := f.store.Find(waits.ID); err != nil || view.Status != StatusCleared {
		t.Fatalf("status on its date = %q, %v", view.Status, err)
	}

	// A new date changes the status at once.
	future.Date = "2026-12-01"
	if changed, err := f.store.Update(waits.ID, future); err != nil || changed.Status != StatusPending {
		t.Fatalf("status after the update = %q, %v", changed.Status, err)
	}
	if view, err := f.store.Find(waits.ID); err != nil || view.Status != StatusPending {
		t.Fatalf("stored status after the update = %q, %v", view.Status, err)
	}
}

func TestBillStatusIsStored(t *testing.T) {
	t.Setenv("FINUX_TODAY", "2026-11-20")
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	bill := f.bill(t, market.BillID)
	if bill.Status != card.StatusOpen {
		t.Fatalf("status of a new bill = %q, want open", bill.Status)
	}

	changes, err := f.store.SyncStatuses(bill.ClosesOn)
	if err != nil || len(changes) != 0 {
		t.Fatalf("SyncStatuses on the closing date = %+v, %v", changes, err)
	}
	changes, err = f.store.SyncStatuses(bill.DueOn)
	if err != nil || len(changes) != 1 || changes[0].From != card.StatusOpen || changes[0].To != card.StatusClosed || changes[0].Bill.ID != bill.ID {
		t.Fatalf("SyncStatuses on the due date = %+v, %v", changes, err)
	}
	if got := f.bill(t, bill.ID); got.Status != card.StatusClosed || got.Total() != 10000 {
		t.Fatalf("bill = %+v", got)
	}

	t.Setenv("FINUX_TODAY", "2027-01-30")
	changes, err = f.store.SyncStatuses("2027-01-30")
	if err != nil || len(changes) != 1 || changes[0].To != card.StatusLate {
		t.Fatalf("SyncStatuses after the due date = %+v, %v", changes, err)
	}
	if _, err := f.store.PayBill(bill.ID, f.account["INTER"], 0, "2027-01-30"); err != nil {
		t.Fatal(err)
	}
	if got := f.bill(t, bill.ID); got.Status != card.StatusPaid {
		t.Fatalf("status after the payment = %q", got.Status)
	}
	if err := f.store.Unpay(bill.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.bill(t, bill.ID); got.Status != card.StatusLate {
		t.Fatalf("status after the payment was deleted = %q", got.Status)
	}
}

func TestSyncStatusesFillsAnOlderDatabase(t *testing.T) {
	t.Setenv("FINUX_TODAY", "2026-11-20")
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	if err := f.store.db.Exec("UPDATE transactions SET status = NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.Exec("UPDATE bills SET status = NULL").Error; err != nil {
		t.Fatal(err)
	}
	changes, err := f.store.SyncStatuses("2026-11-20")
	if err != nil || len(changes) != 0 {
		t.Fatalf("SyncStatuses = %+v, %v: a status that was empty is not a change", changes, err)
	}
	if view, err := f.store.Find(market.ID); err != nil || view.Status != StatusCleared {
		t.Fatalf("transaction status = %q, %v", view.Status, err)
	}
	if got := f.bill(t, market.BillID); got.Status != card.StatusOpen {
		t.Fatalf("bill status = %q", got.Status)
	}
}
