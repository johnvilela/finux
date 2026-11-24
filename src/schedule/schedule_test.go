package schedule

import (
	"reflect"
	"strings"
	"testing"
)

func rent() Schedule {
	return Schedule{Title: "Rent", Code: "RENT", Type: Expense, Value: 150000, AccountID: 1, CategoryID: 2, StartDate: "2026-11-05", Every: 1, Unit: "month"}
}

func TestOccurrences(t *testing.T) {
	monthly := rent()
	if next, ok := monthly.Next(); !ok || next != "2026-11-05" {
		t.Fatalf("Next = %q, %v", next, ok)
	}
	if got := monthly.Pending("2026-11-04"); len(got) != 0 {
		t.Fatalf("Pending before the start = %v", got)
	}
	if got, want := monthly.Pending("2027-01-05"), []string{"2026-11-05", "2026-12-05", "2027-01-05"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending = %v, want %v", got, want)
	}
	// An occurrence that was paid or skipped is not pending.
	monthly.Done = 2
	if got, want := monthly.Pending("2027-02-28"), []string{"2027-01-05", "2027-02-05"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending after two = %v, want %v", got, want)
	}
	if monthly.Recurrence() != "every month" || monthly.Left() != -1 {
		t.Fatalf("recurrence %q, left %d", monthly.Recurrence(), monthly.Left())
	}

	// The day stays after a short month.
	last := rent()
	last.StartDate = "2027-01-31"
	if got, want := last.Pending("2027-03-31"), []string{"2027-01-31", "2027-02-28", "2027-03-31"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending from day 31 = %v, want %v", got, want)
	}
	weeks := rent()
	weeks.Every, weeks.Unit = 2, "week"
	if got, want := weeks.Pending("2026-12-05"), []string{"2026-11-05", "2026-11-19", "2026-12-03"}; !reflect.DeepEqual(got, want) || weeks.Recurrence() != "every 2 weeks" {
		t.Fatalf("Pending each 2 weeks = %v, %q", got, weeks.Recurrence())
	}

	// A number of times and an end date stop the schedule; the first one wins.
	times := rent()
	times.Times = 3
	if got := times.Pending("2030-01-01"); len(got) != 3 || times.Left() != 3 {
		t.Fatalf("3 times: pending %v, left %d", got, times.Left())
	}
	times.Done = 3
	if _, ok := times.Next(); ok || times.Left() != 0 || times.Status("2026-11-05") != StatusDone {
		t.Fatalf("after 3 times: left %d, status %q", times.Left(), times.Status("2026-11-05"))
	}
	until := rent()
	until.EndDate, until.Times = "2026-12-31", 5
	if got := until.Pending("2030-01-01"); len(got) != 2 || until.Left() != 2 {
		t.Fatalf("until: pending %v, left %d", got, until.Left())
	}

	once := rent()
	once.Every, once.Unit = 0, ""
	if got := once.Pending("2030-01-01"); len(got) != 1 || once.Recurrence() != "one time" || once.Left() != 1 {
		t.Fatalf("one time: pending %v, %q", got, once.Recurrence())
	}
	once.Done = 1
	if _, ok := once.Next(); ok {
		t.Fatal("a one-time schedule has a second occurrence")
	}
}

func TestStatus(t *testing.T) {
	current := rent()
	for today, want := range map[string]string{"2026-11-04": StatusUpcoming, "2026-11-05": StatusDue, "2026-12-20": StatusDue} {
		if got := current.Status(today); got != want {
			t.Errorf("Status(%s) = %q, want %q", today, got, want)
		}
	}
	if got := current.Overdue("2026-11-05"); got != 0 {
		t.Errorf("Overdue on the date = %d", got)
	}
	if got := current.Overdue("2026-12-20"); got != 2 {
		t.Errorf("Overdue = %d, want 2", got)
	}
}

func TestRebase(t *testing.T) {
	current := rent()
	current.Times, current.Done = 12, 4
	current.Rebase("2027-04-10")
	if current.StartDate != "2027-04-10" || current.Done != 0 || current.Times != 8 {
		t.Fatalf("rebased = %+v", current)
	}
	if next, _ := current.Next(); next != "2027-04-10" || current.Left() != 8 {
		t.Fatalf("next %q, left %d", next, current.Left())
	}
}

func TestValidate(t *testing.T) {
	if err := rent().Validate(); err != nil {
		t.Fatal(err)
	}
	// The user did not decide yet which account pays: the schedule has
	// only the currency of its value.
	undecided := rent()
	undecided.AccountID, undecided.Currency = 0, "BRL"
	if err := undecided.Validate(); err != nil {
		t.Fatalf("schedule without an account: %v", err)
	}
	for want, change := range map[string]func(*Schedule){
		"title is required":                func(s *Schedule) { s.Title = " " },
		"title is too long":                func(s *Schedule) { s.Title = strings.Repeat("a", 61) },
		"description is too long":          func(s *Schedule) { s.Description = strings.Repeat("a", 201) },
		"unknown type":                     func(s *Schedule) { s.Type = "transfer" },
		"value must be more than zero":     func(s *Schedule) { s.Value = 0 },
		"currency is required":             func(s *Schedule) { s.AccountID = 0 },
		"unknown currency":                 func(s *Schedule) { s.AccountID, s.Currency = 0, "XXX" },
		"automatic schedule needs":         func(s *Schedule) { s.AccountID, s.Currency, s.Auto = 0, "BRL", true },
		"not the two":                      func(s *Schedule) { s.CardID = 3 },
		"category is required":             func(s *Schedule) { s.CategoryID = 0 },
		"invalid date":                     func(s *Schedule) { s.StartDate = "05/11/2026" },
		"unknown period":                   func(s *Schedule) { s.Unit = "hour" },
		"end date is before":               func(s *Schedule) { s.EndDate = "2026-11-04" },
		"times must be from 0 to":          func(s *Schedule) { s.Times = -1 },
		"only for a schedule that repea":   func(s *Schedule) { s.Every, s.Unit, s.Times = 0, "", 3 },
		"only for a schedule that repeats": func(s *Schedule) { s.Every, s.Unit, s.EndDate = 0, "", "2027-01-01" },
	} {
		current := rent()
		change(&current)
		if err := current.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error = %v, want %q", err, want)
		}
	}
}
