package budget

import (
	"reflect"
	"strings"
	"testing"
)

func valid() Budget {
	return Budget{Title: "Food", Code: "FOOD", Currency: "BRL", Limit: 80000, StartDate: "2026-11-01", Every: 1, Unit: Month}
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid budget: %v", err)
	}
	once := valid()
	once.Every, once.Unit, once.EndDate = 0, "", "2026-12-31"
	if err := once.Validate(); err != nil {
		t.Fatalf("one-time budget: %v", err)
	}
	for name, tc := range map[string]struct {
		change func(*Budget)
		want   string
	}{
		"no title":      {func(b *Budget) { b.Title = "  " }, "title is required"},
		"long title":    {func(b *Budget) { b.Title = strings.Repeat("a", MaxTitle+1) }, "title is too long"},
		"long text":     {func(b *Budget) { b.Description = strings.Repeat("a", MaxDescription+1) }, "description is too long"},
		"bad code":      {func(b *Budget) { b.Code = "A B" }, "code"},
		"bad currency":  {func(b *Budget) { b.Currency = "XYZ" }, "unknown currency"},
		"zero limit":    {func(b *Budget) { b.Limit = 0 }, "limit must be more than zero"},
		"no start":      {func(b *Budget) { b.StartDate = "" }, "invalid start date"},
		"bad start":     {func(b *Budget) { b.StartDate = "01/11/2026" }, "invalid start date"},
		"bad unit":      {func(b *Budget) { b.Unit = "decade" }, `unknown period "decade": use one of day, week, month, year`},
		"bad interval":  {func(b *Budget) { b.Every = -1 }, "period must be 1 or more"},
		"bad end":       {func(b *Budget) { b.EndDate = "soon" }, "invalid end date"},
		"end too early": {func(b *Budget) { b.EndDate = "2026-10-31" }, "end date is before the start date"},
		"no period":     {func(b *Budget) { b.Every, b.Unit = 0, "" }, "give a period or an end date"},
	} {
		budget := valid()
		tc.change(&budget)
		if err := budget.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", name, err, tc.want)
		}
	}
}

func TestParseEvery(t *testing.T) {
	for text, want := range map[string]struct {
		every int
		unit  string
	}{
		"month":    {1, Month},
		" Month ":  {1, Month},
		"monthly":  {1, Month},
		"weekly":   {1, Week},
		"daily":    {1, Day},
		"yearly":   {1, Year},
		"2 weeks":  {2, Week},
		"15 days":  {15, Day},
		"15days":   {15, Day},
		"3-months": {3, Month},
		"1 year":   {1, Year},
		"none":     {0, ""},
		"":         {0, ""},
	} {
		every, unit, err := ParseEvery(text)
		if err != nil || every != want.every || unit != want.unit {
			t.Errorf("ParseEvery(%q) = %d, %q, %v; want %d, %q", text, every, unit, err, want.every, want.unit)
		}
	}
	for _, text := range []string{"0 days", "fortnight", "2", "month 2", "-1 week", "99999 days"} {
		if _, _, err := ParseEvery(text); err == nil {
			t.Errorf("ParseEvery(%q): no error", text)
		}
	}
}

func TestFormatEvery(t *testing.T) {
	for want, b := range map[string]Budget{
		"every month":   {Every: 1, Unit: Month},
		"every 2 weeks": {Every: 2, Unit: Week},
		"every 15 days": {Every: 15, Unit: Day},
		"one time":      {},
	} {
		if got := b.Recurrence(); got != want {
			t.Errorf("Recurrence() = %q, want %q", got, want)
		}
	}
}

func TestPeriodAt(t *testing.T) {
	for name, tc := range map[string]struct {
		budget Budget
		date   string
		want   Period
		none   bool
	}{
		"calendar month": {
			Budget{StartDate: "2026-11-01", Every: 1, Unit: Month}, "2026-11-18", Period{0, "2026-11-01", "2026-11-30"}, false},
		"second month": {
			Budget{StartDate: "2026-11-01", Every: 1, Unit: Month}, "2026-12-31", Period{1, "2026-12-01", "2026-12-31"}, false},
		"day 5 of the month": {
			Budget{StartDate: "2026-11-05", Every: 1, Unit: Month}, "2026-12-04", Period{0, "2026-11-05", "2026-12-04"}, false},
		"day 5, next period": {
			Budget{StartDate: "2026-11-05", Every: 1, Unit: Month}, "2026-12-05", Period{1, "2026-12-05", "2027-01-04"}, false},
		"day 31 goes to the last day": {
			Budget{StartDate: "2026-01-31", Every: 1, Unit: Month}, "2026-02-28", Period{1, "2026-02-28", "2026-03-30"}, false},
		"day 31 comes back": {
			Budget{StartDate: "2026-01-31", Every: 1, Unit: Month}, "2026-03-31", Period{2, "2026-03-31", "2026-04-29"}, false},
		"two weeks": {
			Budget{StartDate: "2026-11-02", Every: 2, Unit: Week}, "2026-11-16", Period{1, "2026-11-16", "2026-11-29"}, false},
		"fifteen days": {
			Budget{StartDate: "2026-11-01", Every: 15, Unit: Day}, "2026-11-15", Period{0, "2026-11-01", "2026-11-15"}, false},
		"quarter": {
			Budget{StartDate: "2026-01-01", Every: 3, Unit: Month}, "2026-11-18", Period{3, "2026-10-01", "2026-12-31"}, false},
		"year": {
			Budget{StartDate: "2024-02-29", Every: 1, Unit: Year}, "2025-03-01", Period{1, "2025-02-28", "2026-02-27"}, false},
		"end date cuts the last period": {
			Budget{StartDate: "2026-11-01", Every: 1, Unit: Month, EndDate: "2026-12-10"}, "2026-12-05", Period{1, "2026-12-01", "2026-12-10"}, false},
		"one time": {
			Budget{StartDate: "2026-11-10", EndDate: "2026-12-31"}, "2026-12-01", Period{0, "2026-11-10", "2026-12-31"}, false},
		"before the start": {
			Budget{StartDate: "2026-11-01", Every: 1, Unit: Month}, "2026-10-31", Period{}, true},
		"after the end": {
			Budget{StartDate: "2026-11-01", Every: 1, Unit: Month, EndDate: "2026-12-10"}, "2026-12-11", Period{}, true},
		"one time, after the end": {
			Budget{StartDate: "2026-11-10", EndDate: "2026-12-31"}, "2027-01-01", Period{}, true},
	} {
		got, ok := tc.budget.PeriodAt(tc.date)
		if ok == tc.none || got != tc.want {
			t.Errorf("%s: PeriodAt(%s) = %+v, %v; want %+v", name, tc.date, got, ok, tc.want)
		}
	}
}

func TestPeriods(t *testing.T) {
	monthly := Budget{StartDate: "2026-09-05", Every: 1, Unit: Month}
	want := []Period{
		{0, "2026-09-05", "2026-10-04"},
		{1, "2026-10-05", "2026-11-04"},
		{2, "2026-11-05", "2026-12-04"},
	}
	if got := monthly.Periods("2026-11-18"); !reflect.DeepEqual(got, want) {
		t.Errorf("Periods = %+v, want %+v", got, want)
	}
	if got := monthly.Periods("2026-09-04"); len(got) != 0 {
		t.Errorf("Periods before the start = %+v", got)
	}
	ended := Budget{StartDate: "2026-09-01", Every: 1, Unit: Month, EndDate: "2026-10-15"}
	want = []Period{{0, "2026-09-01", "2026-09-30"}, {1, "2026-10-01", "2026-10-15"}}
	if got := ended.Periods("2026-11-18"); !reflect.DeepEqual(got, want) {
		t.Errorf("Periods of an ended budget = %+v, want %+v", got, want)
	}
	once := Budget{StartDate: "2026-11-10", EndDate: "2026-12-31"}
	want = []Period{{0, "2026-11-10", "2026-12-31"}}
	if got := once.Periods("2027-05-01"); !reflect.DeepEqual(got, want) {
		t.Errorf("Periods of a one-time budget = %+v, want %+v", got, want)
	}
}

func TestFold(t *testing.T) {
	spent := map[string]int64{
		"2026-09-10": 30000,
		"2026-09-30": 20000, // September: 500.00
		"2026-10-01": 120000,
		"2026-10-20": -10000, // October: 1100.00, a refund removes 100.00
		"2026-11-03": -5000,  // November: below zero, counts as zero
		"2026-08-31": 99999,  // before the start
	}
	budget := Budget{Limit: 80000, StartDate: "2026-09-01", Every: 1, Unit: Month}
	periods := budget.Periods("2026-11-18")

	plain := budget.Fold(periods, spent)
	if len(plain) != 3 {
		t.Fatalf("usages = %+v", plain)
	}
	for i, want := range []Usage{
		{Period: periods[0], Limit: 80000, Spent: 50000},
		{Period: periods[1], Limit: 80000, Spent: 110000},
		{Period: periods[2], Limit: 80000, Spent: 0},
	} {
		if plain[i] != want {
			t.Errorf("no rollover, period %d = %+v, want %+v", i, plain[i], want)
		}
	}

	budget.Rollover = true
	rolled := budget.Fold(periods, spent)
	for i, want := range []Usage{
		{Period: periods[0], Limit: 80000, Spent: 50000},
		{Period: periods[1], Limit: 80000, Carried: 30000, Spent: 110000},
		{Period: periods[2], Limit: 80000, Carried: 0, Spent: 0},
	} {
		if rolled[i] != want {
			t.Errorf("rollover, period %d = %+v, want %+v", i, rolled[i], want)
		}
	}
	spent["2026-10-01"] = 150000 // October: 1400.00 of 1100.00
	rolled = budget.Fold(periods, spent)
	if rolled[2].Carried != -30000 || rolled[2].Available() != 50000 {
		t.Errorf("overspend not carried: %+v", rolled[2])
	}
}

func TestUsage(t *testing.T) {
	for name, tc := range map[string]struct {
		usage     Usage
		available int64
		remaining int64
		over      int64
		percent   int
		level     string
	}{
		"empty":           {Usage{Limit: 80000}, 80000, 80000, 0, 0, LevelOK},
		"under":           {Usage{Limit: 80000, Spent: 63999}, 80000, 16001, 0, 79, LevelOK},
		"near":            {Usage{Limit: 80000, Spent: 64000}, 80000, 16000, 0, 80, LevelNear},
		"at the limit":    {Usage{Limit: 80000, Spent: 80000}, 80000, 0, 0, 100, LevelNear},
		"over":            {Usage{Limit: 80000, Spent: 100000}, 80000, 0, 20000, 125, LevelOver},
		"carried":         {Usage{Limit: 80000, Carried: 20000, Spent: 90000}, 100000, 10000, 0, 90, LevelNear},
		"nothing left":    {Usage{Limit: 80000, Carried: -80000}, 0, 0, 0, 100, LevelNear},
		"starts in debt":  {Usage{Limit: 80000, Carried: -90000}, -10000, 0, 10000, 100, LevelOver},
		"less than zero":  {Usage{Limit: 80000, Carried: -90000, Spent: 100}, -10000, 0, 10100, 100, LevelOver},
		"largest amounts": {Usage{Limit: 1 << 62, Spent: 1 << 61}, 1 << 62, 1 << 61, 0, 50, LevelOK},
	} {
		u := tc.usage
		if u.Available() != tc.available || u.Remaining() != tc.remaining || u.Over() != tc.over || u.Percent() != tc.percent || u.Level() != tc.level {
			t.Errorf("%s: available %d, remaining %d, over %d, percent %d, level %s", name, u.Available(), u.Remaining(), u.Over(), u.Percent(), u.Level())
		}
	}
}

func TestStatus(t *testing.T) {
	budget := Budget{Limit: 80000, StartDate: "2026-11-01", Every: 1, Unit: Month, EndDate: "2026-12-31"}
	budget.Usages = budget.Fold(budget.Periods("2026-11-18"), map[string]int64{"2026-11-10": 70000})
	for today, want := range map[string]string{
		"2026-10-31": StatusUpcoming,
		"2026-11-18": LevelNear,
		"2027-01-01": StatusEnded,
	} {
		if got := budget.Status(today); got != want {
			t.Errorf("Status(%s) = %q, want %q", today, got, want)
		}
	}
	if usage, ok := budget.Current("2026-11-18"); !ok || usage.Spent != 70000 {
		t.Errorf("Current = %+v, %v", usage, ok)
	}
	if _, ok := budget.Current("2026-10-31"); ok {
		t.Error("an upcoming budget has a current period")
	}
	if usage, ok := budget.Shown("2027-01-01"); !ok || usage.From != "2026-11-01" {
		t.Errorf("Shown of an ended budget = %+v, %v", usage, ok)
	}
	if usage, ok := budget.UsageAt("2026-11-02"); !ok || usage.Index != 0 {
		t.Errorf("UsageAt = %+v, %v", usage, ok)
	}
	if _, ok := budget.UsageAt("2026-12-02"); ok {
		t.Error("UsageAt found a period that is not loaded")
	}
}

func TestDefaultStart(t *testing.T) {
	// 2026-11-18 is a Wednesday.
	for name, tc := range map[string]struct {
		every int
		unit  string
		want  string
	}{
		"month":     {1, Month, "2026-11-01"},
		"week":      {1, Week, "2026-11-16"},
		"year":      {1, Year, "2026-01-01"},
		"day":       {1, Day, "2026-11-18"},
		"two weeks": {2, Week, "2026-11-18"},
		"one time":  {0, "", "2026-11-18"},
	} {
		if got := DefaultStart(tc.every, tc.unit, "2026-11-18"); got != tc.want {
			t.Errorf("%s: DefaultStart = %s, want %s", name, got, tc.want)
		}
	}
}

func TestCounts(t *testing.T) {
	listed := Budget{Currency: "BRL", CategoryIDs: []uint{1, 3}}
	all := Budget{Currency: "BRL"}
	for name, tc := range map[string]struct {
		budget   Budget
		kind     string
		category uint
		income   bool
		currency string
		want     bool
	}{
		"expense of a listed category":   {listed, "expense", 1, false, "BRL", true},
		"income of a listed category":    {listed, "income", 3, true, "BRL", true},
		"another category":               {listed, "expense", 2, false, "BRL", false},
		"another currency":               {listed, "expense", 1, false, "USD", false},
		"transfer":                       {listed, "transfer", 1, false, "BRL", false},
		"payment":                        {listed, "payment", 1, false, "BRL", false},
		"all: expense":                   {all, "expense", 9, false, "BRL", true},
		"all: refund":                    {all, "income", 9, false, "BRL", true},
		"all: income category stays out": {all, "income", 9, true, "BRL", false},
	} {
		if got := tc.budget.Counts(tc.kind, tc.category, tc.income, tc.currency); got != tc.want {
			t.Errorf("%s: Counts = %v, want %v", name, got, tc.want)
		}
	}
}
