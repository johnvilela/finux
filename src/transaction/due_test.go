package transaction

import (
	"strings"
	"testing"
)

func TestTimingOf(t *testing.T) {
	for _, c := range []struct{ date, due, want string }{
		{"2026-11-17", "", ""},
		{"2026-11-17", "2026-11-17", TimingOnTime},
		{"2026-11-25", "2026-10-06", TimingLate},
		{"2026-11-01", "2026-11-05", TimingEarly},
	} {
		if got := TimingOf(c.date, c.due); got != c.want {
			t.Errorf("TimingOf(%q, %q) = %q, want %q", c.date, c.due, got, c.want)
		}
	}
	if got := (Transaction{Date: "2026-11-25", DueDate: "2026-10-06"}).DaysLate(); got != 50 {
		t.Errorf("DaysLate of a late payment = %d", got)
	}
	if got := (Transaction{Date: "2026-11-01", DueDate: "2026-11-05"}).DaysLate(); got != -4 {
		t.Errorf("DaysLate of an early payment = %d", got)
	}
	if got := (Transaction{Date: "2026-11-01"}).DaysLate(); got != 0 {
		t.Errorf("DaysLate without a due date = %d", got)
	}
}

// A transaction keeps the date it was planned for and the date it was
// paid on. The timing compares the two.
func TestStoreDueDate(t *testing.T) {
	f := newFixture(t)

	plain := f.create(t, f.draft("Bread", Expense, "INTER", "FOOD", 500))
	if plain.DueDate != "" || plain.Timing != "" {
		t.Fatalf("a transaction without a due date has %q, %q", plain.DueDate, plain.Timing)
	}

	draft := f.draft("Energy", Expense, "INTER", "HOME", 18700)
	draft.Date, draft.DueDate = "2026-11-25", "06/10/2026"
	late := f.create(t, draft)
	if late.Date != "2026-11-25" || late.DueDate != "2026-10-06" || late.Timing != TimingLate {
		t.Fatalf("late = %+v", late)
	}
	found, err := f.store.Find(late.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.DueDate != "2026-10-06" || found.Timing != TimingLate {
		t.Fatalf("stored = %q, %q", found.DueDate, found.Timing)
	}

	// A new paid date changes the timing. The due date stays.
	change := DraftOf(found.Transaction)
	change.Date = "2026-10-01"
	early, err := f.store.Update(late.ID, change)
	if err != nil {
		t.Fatal(err)
	}
	if early.DueDate != "2026-10-06" || early.Timing != TimingEarly {
		t.Fatalf("early = %q, %q", early.DueDate, early.Timing)
	}
	change.Date = "2026-10-06"
	if onTime, err := f.store.Update(late.ID, change); err != nil || onTime.Timing != TimingOnTime {
		t.Fatalf("on time = %q, %v", onTime.Timing, err)
	}

	// An empty due date takes the two away.
	change.DueDate = ""
	cleared, err := f.store.Update(late.ID, change)
	if err != nil {
		t.Fatal(err)
	}
	if found, err = f.store.Find(late.ID); err != nil || cleared.Timing != "" || found.DueDate != "" || found.Timing != "" {
		t.Fatalf("cleared = %q, %q, %v", found.DueDate, found.Timing, err)
	}

	// An edit can give a due date to a transaction that has none.
	change = DraftOf(plain)
	change.DueDate = "2026-11-10"
	if marked, err := f.store.Update(plain.ID, change); err != nil || marked.Timing != TimingLate {
		t.Fatalf("marked = %q, %v", marked.Timing, err)
	}

	draft.DueDate = "soon"
	if _, err := f.store.Create(draft); err == nil || !strings.Contains(err.Error(), "invalid date") {
		t.Fatalf("invalid due date: %v", err)
	}
	draft.DueDate, draft.Installments = "2026-10-06", 3
	if _, err := f.store.Create(draft); err == nil || !strings.Contains(err.Error(), "installments cannot have a due date") {
		t.Fatalf("due date with installments: %v", err)
	}
	transfer := f.draft("Move", Transfer, "INTER", "XFER", 1000)
	transfer.ToAccountID, transfer.DueDate = f.account["NUBAN"], "2026-10-06"
	if _, err := f.store.Create(transfer); err == nil || !strings.Contains(err.Error(), "a transfer cannot have a due date") {
		t.Fatalf("due date of a transfer: %v", err)
	}
}

// The due date changes no bill: a paid bill does not stop its change.
func TestStoreDueDateInAPaidBill(t *testing.T) {
	f := newFixture(t)
	draft := Draft{Title: "Gym", Type: Expense, Value: 9000, CardID: f.card["VISA"], CategoryID: f.category["HEALT"], Date: "2026-11-17"}
	created := f.create(t, draft)
	if _, err := f.store.PayBill(created.BillID, f.account["INTER"], 0, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	change := DraftOf(created)
	change.DueDate = "2026-11-10"
	if updated, err := f.store.Update(created.ID, change); err != nil || updated.Timing != TimingLate {
		t.Fatalf("due date in a paid bill = %q, %v", updated.Timing, err)
	}
}

func TestFilterTiming(t *testing.T) {
	late := View{Transaction: Transaction{Title: "Energy", Timing: TimingLate}}
	plain := View{Transaction: Transaction{Title: "Bread"}}
	if got := ParseSearch("timing:late energy"); got.Timing != "late" || got.Title != "energy" {
		t.Fatalf("timing token = %+v", got)
	}
	if !ParseSearch("timing:l").Match(late) || ParseSearch("timing:early").Match(late) || ParseSearch("timing:late").Match(plain) ||
		!ParseSearch("timing:").Match(plain) || (Filter{Timing: "lat"}).Match(late) || !(Filter{Timing: TimingLate}).Match(late) {
		t.Fatal("the timing token does not select a late transaction")
	}
	if got := (Filter{Timing: TimingLate, Period: "2026"}).String(); got != "timing:late 2026" {
		t.Fatalf("String with timing = %q", got)
	}
}
