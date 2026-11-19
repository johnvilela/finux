package ui

import (
	"strings"
	"testing"

	"finux/src/goal"
	"finux/src/schedule"
	"finux/src/transaction"
)

// sampleSchedules returns a monthly bill with two due occurrences, an
// income that comes and a schedule for one time on a credit card that
// is done.
func sampleSchedules() []ScheduleView {
	accounts, categories := sampleAccounts(), sampleCategories()
	visa := accounts[1]
	visa.Code, visa.Name = "VISA", "Visa Gold"
	return []ScheduleView{
		{
			Schedule: schedule.Schedule{ID: 1, Title: "Rent", Description: "Apartment", Code: "RENT", Type: schedule.Expense, Value: 150000, AccountID: 1, StartDate: "2026-10-05", Every: 1, Unit: "month", Tags: "#fixed", Times: 12},
			Account:  accounts[0], Category: categories[0], Goal: &goal.Goal{Code: "TRIP", Title: "Trip"},
		},
		{
			Schedule: schedule.Schedule{ID: 2, Title: "Salary", Code: "SAL", Type: schedule.Income, Value: 400000, AccountID: 1, StartDate: "2026-11-30", Every: 1, Unit: "month", Auto: true},
			Account:  accounts[0], Category: categories[1],
		},
		{
			Schedule: schedule.Schedule{ID: 3, Title: "Dentist", Code: "DENT", Type: schedule.Expense, Value: 30000, CardID: 1, StartDate: "2026-11-10", Done: 1},
			Account:  visa, OnCard: true, Category: categories[0],
		},
	}
}

func TestScheduleTableAndEntries(t *testing.T) {
	views := sampleSchedules()
	const want = "" +
		"CODE  TITLE    REPEATS      NEXT        STATUS    ACCOUNT         VALUE\n" +
		"RENT  Rent     every month  05/10/2026  2 due     INTER    -R$ 1.500,00\n" +
		"SAL   Salary   every month  30/11/2026  upcoming  INTER     R$ 4.000,00\n" +
		"DENT  Dentist  one time     -           done      VISA       -R$ 300,00\n" +
		"3 schedules\n"
	if got := ScheduleTable(views, budgetToday, true); got != want {
		t.Fatalf("plain table =\n%s\nwant\n%s", got, want)
	}
	styled := ScheduleTable(views, budgetToday, false)
	if !strings.Contains(styled, "\x1b[") || !strings.Contains(styled, "╭") || !strings.Contains(plain(styled), "2 due") {
		t.Fatalf("styled table:\n%s", styled)
	}
	if got := ScheduleTable(views[:1], budgetToday, true); !strings.HasSuffix(got, "\n1 schedule\n") {
		t.Fatalf("singular count missing:\n%s", got)
	}
	if got := ScheduleTable(nil, budgetToday, true); got != "No scheduled transactions yet. Run 'finux sc add'.\n" {
		t.Fatalf("empty table = %q", got)
	}
	views[2].DeletedAt.Valid = true
	entries := ScheduleEntries(views, budgetToday)
	if len(entries) != 3 || entries[0].Code != "RENT" || entries[0].Value != "-R$ 1.500,00" || !entries[0].Negative || entries[1].Negative {
		t.Fatalf("entries = %+v", entries)
	}
	for i, want := range []string{"every month  2 due  05/10/2026", "every month  upcoming  30/11/2026", "archived  one time  done  -"} {
		if entries[i].Description != want {
			t.Errorf("description %d = %q, want %q", i, entries[i].Description, want)
		}
	}
}

func TestScheduleDetailsView(t *testing.T) {
	views := sampleSchedules()
	created := []transaction.View{sampleTransactions()[0]}
	details := ScheduleDetails{View: views[0], Transactions: created, Today: budgetToday}
	bare := ScheduleDetailsView(details, true)
	if strings.ContainsAny(bare, "╭│\x1b") {
		t.Fatalf("plain details have borders or colors:\n%s", bare)
	}
	for _, want := range []string{
		"RENT  Rent\nApartment\n", "ID          1\n", "Type        expense\n", "Value       R$ 1.500,00\n", "Account     INTER  Banco Inter\n",
		"Tags        #fixed\n", "Goal        TRIP  Trip\n", "Repeats     every month\n", "Times left  12\n", "Confirm     manual\n", "Status      2 due\n",
		"Due         05/10/2026, 05/11/2026\n", "Next        05/12/2026\n", "ID  DATE        TITLE          ACCOUNT      VALUE\n", "12  17/11/2026  Weekly market  INTER    -R$ 50,25\n",
	} {
		if !strings.Contains(bare, want) {
			t.Errorf("plain details lack %q:\n%s", want, bare)
		}
	}
	styled := plain(ScheduleDetailsView(details, false))
	for _, want := range []string{"RENT", "Rent", "EVERY MONTH  #1", "-R$ 1.500,00", "2 DUE", "Due         05/10/2026, 05/11/2026", "Next        05/12/2026", "Confirm  manual  ·  12 left", "TRANSACTIONS", "Weekly market"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled details lack %q:\n%s", want, styled)
		}
	}
	// A schedule with many due occurrences lists the first ones.
	daily := views[0]
	daily.Every, daily.Unit, daily.Times = 1, "day", 0
	if got := ScheduleDetailsView(ScheduleDetails{View: daily, Today: budgetToday}, true); !strings.Contains(got, "10/10/2026 and 39 more\n") || !strings.Contains(got, noScheduleTransactions) {
		t.Errorf("many due occurrences:\n%s", got)
	}
	automatic := ScheduleDetailsView(ScheduleDetails{View: views[1], Today: budgetToday}, true)
	if !strings.Contains(automatic, "Confirm     automatic\n") || strings.Contains(automatic, "Due ") || strings.Contains(automatic, "Times left") {
		t.Errorf("automatic schedule:\n%s", automatic)
	}
	done := ScheduleDetailsView(ScheduleDetails{View: views[2], Today: budgetToday}, true)
	if !strings.Contains(done, "Card        VISA  Visa Gold\n") || !strings.Contains(done, "Status      done\n") || strings.Contains(done, "Next") {
		t.Errorf("done schedule:\n%s", done)
	}
}

func TestResumeShowsTheScheduled(t *testing.T) {
	resume := sampleResume()
	views := sampleSchedules()
	resume.ScheduledUntil = "2026-11-30"
	resume.Scheduled = []ResumeOccurrence{{View: views[0], Date: "2026-10-05"}, {View: views[0], Date: "2026-11-18"}, {View: views[1], Date: "2026-11-30"}}
	out := ResumeView(resume, true)
	for _, want := range []string{
		"Scheduled until 30/11/2026\n", "05/10/2026  RENT  Rent    overdue   INTER    -R$ 1.500,00\n", "18/11/2026  RENT  Rent    today     INTER    -R$ 1.500,00\n",
		"30/11/2026  SAL   Salary  upcoming  INTER     R$ 4.000,00\n", "After BRL  R$ 2.224,51\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain resume lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "After USD") {
		t.Errorf("a currency without schedules has a balance after:\n%s", out)
	}
	styled := plain(ResumeView(resume, false))
	for _, want := range []string{"SCHEDULED UNTIL 30/11/2026", "overdue", "today", "Balance after, BRL", "R$ 2.224,51"} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled resume lacks %q:\n%s", want, styled)
		}
	}
	for _, line := range strings.Split(styled, "\n") {
		if width := len([]rune(line)); width > detailsWidth+8 {
			t.Errorf("line is %d wide: %q", width, line)
		}
	}
}
