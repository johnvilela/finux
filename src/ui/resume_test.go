package ui

import (
	"strings"
	"testing"

	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
)

func sampleResume() Resume {
	flow := ResumeFlow{Currency: "BRL", Income: 300000, Expenses: 70000}
	for i, code := range []string{"A", "B", "C", "D", "E", "F", "G"} {
		flow.Categories = append(flow.Categories, ResumeCategory{Category: category.Category{Code: code, Name: "Name " + code, Color: "red"}, Amount: int64(7-i) * 2500})
	}
	return Resume{
		Month: "2026-11", Today: budgetToday, Days: 18,
		Flows:    []ResumeFlow{flow},
		Accounts: sampleAccounts(),
		Bills: []ResumeBill{{
			Card: card.Card{Code: "VISA", Color: "purple", Currency: "BRL"},
			Bill: card.Bill{Period: "2026-12", ClosesOn: "2026-11-25", DueOn: "2026-12-05", Spent: 10000},
		}},
		Budgets: sampleBudgets()[:2],
		Goals:   []goal.Goal{{Code: "TRIP", Title: "Trip to Japan", Currency: "BRL", Target: 1000000, Saved: 250000, DueDate: "2027-06-30"}},
	}
}

func TestResumeViewAddsUpTheSmallCategories(t *testing.T) {
	out := ResumeView(sampleResume(), true)
	for _, want := range []string{"A     Name A    25%  R$ 175,00\n", "D     Name D    14%  R$ 100,00\n", "…     3 others  21%  R$ 150,00\n", "Per day     R$ 38,88\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain resume lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Name E") {
		t.Errorf("plain resume lists more than %d categories:\n%s", topExpenses, out)
	}
}

func TestStyledResume(t *testing.T) {
	resume := sampleResume()
	out := plain(ResumeView(resume, false))
	for _, want := range []string{
		"RESUME   November 2026", "18/11/2026  ·  DAY 18 OF 30", "▲ Income", "R$ 3.000,00", "▼ Expenses", "Net", "R$ 2.300,00",
		"R$ 38,88 of expenses per day", "TOP EXPENSES IN BRL", "3 others", "ACCOUNTS", "Total BRL", "BILLS TO PAY", "12/2026  open    due 05/12/2026",
		"BUDGETS", "near     87%  R$ 700,00 of R$ 800,00", "GOALS", "active   25%  R$ 2.500,00 of R$ 10.000,00",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("styled resume lacks %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if width := len([]rune(line)); width > detailsWidth+8 {
			t.Errorf("line is %d wide: %q", width, line)
		}
	}
	resume.Flows, resume.Month, resume.Days = nil, "2026-12", 0
	if out := plain(ResumeView(resume, false)); !strings.Contains(out, noFlows) || strings.Contains(out, "DAY 18") {
		t.Errorf("styled resume of an empty month:\n%s", out)
	}
}
