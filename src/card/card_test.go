package card

import "testing"

func TestCardValidate(t *testing.T) {
	valid := Card{Name: "Visa Gold", Code: "VISA", Color: "blue", Currency: "BRL", Limit: 500000, ClosingDay: 25, DueDay: 5}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid card: %v", err)
	}
	for name, change := range map[string]func(*Card){
		"empty name":       func(c *Card) { c.Name = "  " },
		"bad code":         func(c *Card) { c.Code = "visa!" },
		"unknown color":    func(c *Card) { c.Color = "magenta" },
		"unknown currency": func(c *Card) { c.Currency = "JPY" },
		"negative limit":   func(c *Card) { c.Limit = -1 },
		"closing day 0":    func(c *Card) { c.ClosingDay = 0 },
		"closing day 32":   func(c *Card) { c.ClosingDay = 32 },
		"due day 0":        func(c *Card) { c.DueDay = 0 },
		"due day 32":       func(c *Card) { c.DueDay = 32 },
	} {
		card := valid
		change(&card)
		if err := card.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
}

func TestCardAvailable(t *testing.T) {
	card := Card{Limit: 500000, Used: 120000}
	if got := card.Available(); got != 380000 {
		t.Fatalf("Available() = %d, want 380000", got)
	}
}

func TestCycle(t *testing.T) {
	for _, test := range []struct {
		closing, due                  int
		date, period, closesOn, dueOn string
	}{
		// Due day before the closing day: the bill is due in the next month.
		{25, 5, "2026-11-17", "2026-12", "2026-11-25", "2026-12-05"},
		{25, 5, "2026-11-25", "2026-12", "2026-11-25", "2026-12-05"},
		{25, 5, "2026-11-26", "2027-01", "2026-12-25", "2027-01-05"},
		{25, 5, "2026-12-26", "2027-02", "2027-01-25", "2027-02-05"},
		// Due day after the closing day: same month.
		{3, 10, "2026-11-02", "2026-11", "2026-11-03", "2026-11-10"},
		{3, 10, "2026-11-04", "2026-12", "2026-12-03", "2026-12-10"},
		// Short months use their last day.
		{31, 10, "2027-02-28", "2027-03", "2027-02-28", "2027-03-10"},
		{31, 10, "2027-03-01", "2027-04", "2027-03-31", "2027-04-10"},
		{10, 31, "2027-02-05", "2027-02", "2027-02-10", "2027-02-28"},
		// The same day closes and is due one month later.
		{15, 15, "2026-11-15", "2026-12", "2026-11-15", "2026-12-15"},
	} {
		card := Card{ClosingDay: test.closing, DueDay: test.due}
		period, closesOn, dueOn := card.Cycle(test.date)
		if period != test.period || closesOn != test.closesOn || dueOn != test.dueOn {
			t.Errorf("closing %d due %d: Cycle(%s) = %s %s %s, want %s %s %s", test.closing, test.due, test.date,
				period, closesOn, dueOn, test.period, test.closesOn, test.dueOn)
		}
		// The period gives the same dates back.
		if gotCloses, gotDue := card.CycleOf(test.period); gotCloses != test.closesOn || gotDue != test.dueOn {
			t.Errorf("closing %d due %d: CycleOf(%s) = %s %s, want %s %s", test.closing, test.due, test.period,
				gotCloses, gotDue, test.closesOn, test.dueOn)
		}
	}
}

func TestPeriods(t *testing.T) {
	for period, want := range map[string]string{"2026-11": "2026-12", "2026-12": "2027-01"} {
		if got := NextPeriod(period); got != want {
			t.Errorf("NextPeriod(%s) = %s, want %s", period, got, want)
		}
	}
	if got := FormatPeriod("2026-12"); got != "12/2026" {
		t.Errorf("FormatPeriod = %q", got)
	}
	for text, want := range map[string]string{"12/2026": "2026-12", "1/2027": "2027-01", "2026-12": "2026-12", "2026/3": "2026-03"} {
		if got, err := ParsePeriod(text); err != nil || got != want {
			t.Errorf("ParsePeriod(%q) = %q, %v, want %q", text, got, err, want)
		}
	}
	for _, text := range []string{"", "2026", "13/2026", "25/12/2026", "december"} {
		if _, err := ParsePeriod(text); err == nil {
			t.Errorf("ParsePeriod(%q) = nil error", text)
		}
	}
}

func TestBillTotalAndStatus(t *testing.T) {
	bill := Bill{ClosesOn: "2026-11-25", DueOn: "2026-12-05", Carried: 2000, Spent: 10000}
	if got := bill.Total(); got != 12000 {
		t.Fatalf("Total() = %d, want 12000", got)
	}
	for today, want := range map[string]string{
		"2026-11-01": StatusOpen,
		"2026-11-25": StatusOpen,
		"2026-11-26": StatusClosed,
		"2026-12-05": StatusClosed,
		"2026-12-06": StatusLate,
	} {
		if got := bill.Status(today); got != want {
			t.Errorf("Status(%s) = %s, want %s", today, got, want)
		}
	}
	paid := bill
	paid.PaidOn = "2026-12-01"
	if got := paid.Status("2027-01-01"); got != StatusPaid {
		t.Errorf("paid bill: Status = %s", got)
	}
	// A bill that owes nothing is never late.
	empty := Bill{ClosesOn: "2026-11-25", DueOn: "2026-12-05", Spent: -500}
	if got := empty.Status("2027-01-01"); got != StatusClosed {
		t.Errorf("bill without debt: Status = %s, want %s", got, StatusClosed)
	}
}
