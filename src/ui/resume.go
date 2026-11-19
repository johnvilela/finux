package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/schedule"
	"finux/src/transaction"
)

// topExpenses is the number of categories that a resume lists for one
// currency. The categories after them show as one line.
const topExpenses = 5

// Resume is the financial summary of one month and of the date of today.
type Resume struct {
	// Month is the month of the incomes and expenses, as YYYY-MM. Today
	// is the current date as YYYY-MM-DD.
	Month, Today string
	// Days is the number of days of the month up to today: all of them
	// for a month that ended, and 0 for a month that did not start.
	Days int
	// Flows holds the incomes and expenses of the month, one for each
	// currency that has them.
	Flows []ResumeFlow
	// Accounts, Bills, Budgets and Goals are as they are today. Bills
	// holds the bills that wait for a payment, the first due date first.
	// Budgets holds the budgets that have a period today.
	Accounts []account.Account
	Bills    []ResumeBill
	Budgets  []budget.Budget
	Goals    []goal.Goal
	// Scheduled holds the occurrences of the schedules that wait, up to
	// the date ScheduledUntil, the oldest first.
	Scheduled      []ResumeOccurrence
	ScheduledUntil string
}

// ResumeOccurrence is one occurrence of a schedule that is not paid and
// not skipped.
type ResumeOccurrence struct {
	View ScheduleView
	// Date is the date of the occurrence, as YYYY-MM-DD.
	Date string
}

// status returns "overdue", "today" or "upcoming".
func (o ResumeOccurrence) status(today string) string {
	switch {
	case o.Date < today:
		return "overdue"
	case o.Date == today:
		return "today"
	}
	return schedule.StatusUpcoming
}

// after returns the balance that each currency has after the scheduled
// occurrences of its accounts, for the currencies that have them. An
// occurrence of a credit card does not change a balance.
func (r Resume) after() (currencies []string, sums map[string]int64) {
	all, totals := r.totals()
	moved := map[string]bool{}
	for _, occurrence := range r.Scheduled {
		if !occurrence.View.OnCard {
			currency := occurrence.View.Account.Currency
			totals[currency] += occurrence.View.Amount()
			moved[currency] = true
		}
	}
	for _, currency := range all {
		if moved[currency] {
			currencies = append(currencies, currency)
		}
	}
	return currencies, totals
}

// ResumeFlow is the incomes and expenses of a month in one currency.
type ResumeFlow struct {
	Currency         string
	Income, Expenses int64
	// Categories holds the expenses of each category, the largest first.
	Categories []ResumeCategory
}

// Net returns the incomes less the expenses.
func (f ResumeFlow) Net() int64 { return f.Income - f.Expenses }

// top returns the categories to list. When there are more than
// topExpenses, the last line adds up the others.
func (f ResumeFlow) top() []ResumeCategory {
	if len(f.Categories) <= topExpenses {
		return f.Categories
	}
	rest := ResumeCategory{Category: category.Category{Code: "…", Name: fmt.Sprintf("%d others", len(f.Categories)-topExpenses+1)}}
	for _, other := range f.Categories[topExpenses-1:] {
		rest.Amount += other.Amount
	}
	return append(append([]ResumeCategory{}, f.Categories[:topExpenses-1]...), rest)
}

// percent returns the part of the expenses that an amount is, as a
// whole percentage.
func (f ResumeFlow) percent(amount int64) int {
	if f.Expenses <= 0 {
		return 0
	}
	return int(float64(amount) / float64(f.Expenses) * 100)
}

// ResumeCategory is the expenses of one category in a month.
type ResumeCategory struct {
	Category category.Category
	Amount   int64
}

// ResumeBill is a bill of a credit card that waits for a payment.
type ResumeBill struct {
	Card card.Card
	Bill card.Bill
}

// monthName shows a YYYY-MM month as "November 2026".
func monthName(month string) string {
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return month
	}
	return parsed.Format("January 2006")
}

// current reports whether the month of the resume is the month of today.
func (r Resume) current() bool { return strings.HasPrefix(r.Today, r.Month) }

// day returns the position of today in its month: "day 17 of 30".
func (r Resume) day() string {
	parsed, err := time.Parse("2006-01-02", r.Today)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("day %d of %d", parsed.Day(), parsed.AddDate(0, 1, -parsed.Day()).Day())
}

// totals returns the sum of the balances of the accounts of each
// currency, in the order the currencies first show.
func (r Resume) totals() (currencies []string, sums map[string]int64) {
	sums = map[string]int64{}
	for _, current := range r.Accounts {
		if _, seen := sums[current.Currency]; !seen {
			currencies = append(currencies, current.Currency)
		}
		sums[current.Currency] += current.Balance()
	}
	return currencies, sums
}

const noFlows = "No incomes or expenses in this month."

// ResumeView renders the resume. Plain output has no colors and no
// borders, for pipes and files.
func ResumeView(resume Resume, plain bool) string {
	if !plain {
		return styledResume(resume)
	}
	row := func(label, value string) string { return fmt.Sprintf("%-12s%s\n", label, value) }
	date := transaction.FormatDate(resume.Today)
	if resume.current() {
		date += " (" + resume.day() + ")"
	}
	out := "Resume of " + monthName(resume.Month) + "\n" + row("Today", date)
	if len(resume.Flows) == 0 {
		out += "\n" + noFlows + "\n"
	}
	for _, flow := range resume.Flows {
		format := func(amount int64) string { return money.Format(amount, flow.Currency) }
		out += "\nMonth in " + flow.Currency + "\n" + row("Income", format(flow.Income)) + row("Expenses", format(flow.Expenses)) + row("Net", format(flow.Net()))
		if resume.Days > 0 {
			out += row("Per day", format(flow.Expenses/int64(resume.Days)))
		}
		if len(flow.Categories) == 0 {
			continue
		}
		var rows [][]string
		for _, spent := range flow.top() {
			rows = append(rows, []string{spent.Category.Code, spent.Category.Name, strconv.Itoa(flow.percent(spent.Amount)) + "%", format(spent.Amount)})
		}
		out += "\nTop expenses in " + flow.Currency + "\n" + plainTable([]string{"CODE", "CATEGORY", "%", "AMOUNT"}, rows, 3)
	}
	if len(resume.Accounts) > 0 {
		rows := make([][]string, len(resume.Accounts))
		for i, current := range resume.Accounts {
			rows[i] = []string{current.Code, current.Name, money.Format(current.Balance(), current.Currency)}
		}
		out += "\nAccounts\n" + plainTable([]string{"CODE", "NAME", "BALANCE"}, rows, 2)
		currencies, sums := resume.totals()
		for _, currency := range currencies {
			out += "Total " + currency + "  " + money.Format(sums[currency], currency) + "\n"
		}
	}
	if len(resume.Scheduled) > 0 {
		rows := make([][]string, len(resume.Scheduled))
		for i, occurrence := range resume.Scheduled {
			view := occurrence.View
			rows[i] = []string{
				transaction.FormatDate(occurrence.Date), view.Code, view.Title, occurrence.status(resume.Today),
				view.Account.Code, money.Format(view.Amount(), view.Account.Currency),
			}
		}
		out += "\nScheduled until " + transaction.FormatDate(resume.ScheduledUntil) + "\n" +
			plainTable([]string{"DATE", "CODE", "TITLE", "STATUS", "ACCOUNT", "VALUE"}, rows, 5)
		currencies, sums := resume.after()
		for _, currency := range currencies {
			out += "After " + currency + "  " + money.Format(sums[currency], currency) + "\n"
		}
	}
	if len(resume.Bills) > 0 {
		rows := make([][]string, len(resume.Bills))
		for i, pending := range resume.Bills {
			rows[i] = []string{
				pending.Card.Code, card.FormatPeriod(pending.Bill.Period), pending.Bill.Status(resume.Today),
				transaction.FormatDate(pending.Bill.DueOn), money.Format(pending.Bill.Total(), pending.Card.Currency),
			}
		}
		out += "\nBills to pay\n" + plainTable([]string{"CARD", "BILL", "STATUS", "DUE", "TOTAL"}, rows, 4)
	}
	if len(resume.Budgets) > 0 {
		rows := make([][]string, len(resume.Budgets))
		for i, current := range resume.Budgets {
			usage := budgetUsage(current, resume.Today)
			rows[i] = []string{
				current.Code, current.Title, current.Status(resume.Today), strconv.Itoa(usage.Percent()) + "%",
				money.Format(usage.Spent, current.Currency), money.Format(usage.Available(), current.Currency),
			}
		}
		out += "\nBudgets\n" + plainTable([]string{"CODE", "TITLE", "STATUS", "%", "SPENT", "AVAILABLE"}, rows, 5)
	}
	if len(resume.Goals) > 0 {
		rows := make([][]string, len(resume.Goals))
		for i, current := range resume.Goals {
			rows[i] = []string{
				current.Code, current.Title, current.Status(resume.Today), goalDue(current), strconv.Itoa(current.Percent()) + "%",
				money.Format(current.Progress(), current.Currency), money.Format(current.Target, current.Currency),
			}
		}
		out += "\nGoals\n" + plainTable([]string{"CODE", "TITLE", "STATUS", "DUE", "%", "SAVED", "TARGET"}, rows, 6)
	}
	return out
}

func styledResume(resume Resume) string {
	const indent = "   "
	width := detailsWidth
	spread := func(left, right string) string {
		return left + strings.Repeat(" ", max(2, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
	}
	// pad makes text a column of a fixed width.
	pad := func(text string, size int) string {
		text = cut(text, size)
		return text + strings.Repeat(" ", size-lipgloss.Width(text))
	}
	bold := lipgloss.NewStyle().Bold(true)

	badge := bold.Padding(0, 1).Foreground(lipgloss.Color("#000000")).Background(colorAccent).Render("RESUME")
	date := transaction.FormatDate(resume.Today)
	if resume.current() {
		date += "  ·  " + strings.ToUpper(resume.day())
	}
	lines := []string{spread(badge+"  "+styleBold.Render(monthName(resume.Month)), styleDim.Render(date))}
	if len(resume.Flows) == 0 {
		lines = append(lines, "", styleDim.Render(noFlows))
	}
	for _, flow := range resume.Flows {
		format := func(amount int64) string { return money.Format(amount, flow.Currency) }
		tint := colorIncome
		if flow.Net() < 0 {
			tint = colorExpense
		}
		lines = append(lines,
			"",
			styleDim.Render(flow.Currency),
			spread(paint(colorIncome, "▲")+" Income", format(flow.Income)),
			spread(paint(colorExpense, "▼")+" Expenses", format(flow.Expenses)),
			styleDim.Render(strings.Repeat("─", width)),
			spread(bold.Render("  Net"), bold.Foreground(lipgloss.Color(tint)).Render(format(flow.Net()))),
		)
		if resume.Days > 0 {
			lines = append(lines, styleDim.Render("  "+format(flow.Expenses/int64(resume.Days))+" of expenses per day"))
		}
	}
	out := styleBox.BorderForeground(colorAccent).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"

	section := func(title string) {
		out += "\n" + indent + styleTitle.Render(title) + "\n" + indent + styleDim.Render(strings.Repeat("─", width)) + "\n"
	}
	const codeWidth, nameWidth, shareWidth, statusWidth, percentWidth = 5, 14, 16, 8, 4
	percent := func(value int) string { return fmt.Sprintf("%*d%%", percentWidth-1, value) }
	// progress is the line of a budget or a goal. The title takes the
	// width that the amounts leave.
	progress := func(tint, code, title, status string, share int, amount, total string) string {
		right := amount + styleDim.Render(" of "+total)
		titleWidth := max(width-2-codeWidth-2-2-statusWidth-percentWidth-2-lipgloss.Width(right), 4)
		left := paint(tint, "●") + " " + pad(code, codeWidth) + "  " + pad(title, titleWidth) + "  " + paint(tint, pad(status, statusWidth)) + percent(share)
		return indent + spread(left, right) + "\n"
	}
	for _, flow := range resume.Flows {
		if len(flow.Categories) == 0 {
			continue
		}
		section("TOP EXPENSES IN " + flow.Currency)
		for _, spent := range flow.top() {
			dot, _ := category.ColorByKey(spent.Category.Color)
			share := flow.percent(spent.Amount)
			filled := max(share*shareWidth/100, 1)
			bar := paint(colorExpense, strings.Repeat("█", filled)) + styleDim.Render(strings.Repeat("░", shareWidth-filled))
			left := paint(dot.Hex, "●") + " " + pad(spent.Category.Code, codeWidth) + "  " + pad(spent.Category.Name, nameWidth) + "  " + bar + " " + styleDim.Render(percent(share))
			out += indent + spread(left, money.Format(spent.Amount, flow.Currency)) + "\n"
		}
	}
	if len(resume.Accounts) > 0 {
		section("ACCOUNTS")
		for _, current := range resume.Accounts {
			balance := money.Format(current.Balance(), current.Currency)
			if current.Balance() < 0 {
				balance = styleError.Render(balance)
			}
			out += indent + spread(swatch(current.Color, "●")+" "+pad(current.Code, codeWidth)+"  "+cut(current.Name, 30), balance) + "\n"
		}
		currencies, sums := resume.totals()
		for _, currency := range currencies {
			out += indent + spread(styleDim.Render("  Total "+currency), bold.Render(money.Format(sums[currency], currency))) + "\n"
		}
	}
	if len(resume.Scheduled) > 0 {
		section("SCHEDULED UNTIL " + transaction.FormatDate(resume.ScheduledUntil))
		for _, occurrence := range resume.Scheduled {
			view, status := occurrence.View, occurrence.status(resume.Today)
			tint := colorNeutral
			if status != schedule.StatusUpcoming {
				tint = colorWarning
			}
			left := styleDim.Render(transaction.FormatDate(occurrence.Date)) + "  " + pad(view.Code, codeWidth) + "  " + pad(view.Title, 16) + "  " +
				paint(tint, pad(status, statusWidth)) + "  " + styleDim.Render(view.Account.Code)
			out += indent + spread(left, typeStyle(view.Type).Render(money.Format(view.Amount(), view.Account.Currency))) + "\n"
		}
		currencies, sums := resume.after()
		for _, currency := range currencies {
			out += indent + spread(styleDim.Render("  Balance after, "+currency), bold.Render(money.Format(sums[currency], currency))) + "\n"
		}
	}
	if len(resume.Bills) > 0 {
		section("BILLS TO PAY")
		for _, pending := range resume.Bills {
			status := pending.Bill.Status(resume.Today)
			left := swatch(pending.Card.Color, "●") + " " + pad(pending.Card.Code, codeWidth) + "  " + card.FormatPeriod(pending.Bill.Period) + "  " +
				statusStyle(status).Render(pad(status, statusWidth)) + styleDim.Render("due "+transaction.FormatDate(pending.Bill.DueOn))
			out += indent + spread(left, money.Format(pending.Bill.Total(), pending.Card.Currency)) + "\n"
		}
	}
	if len(resume.Budgets) > 0 {
		section("BUDGETS")
		for _, current := range resume.Budgets {
			status, usage := current.Status(resume.Today), budgetUsage(current, resume.Today)
			out += progress(budgetColor(status), current.Code, current.Title, status, usage.Percent(),
				money.Format(usage.Spent, current.Currency), money.Format(usage.Available(), current.Currency))
		}
	}
	if len(resume.Goals) > 0 {
		section("GOALS")
		for _, current := range resume.Goals {
			status := current.Status(resume.Today)
			out += progress(goalColor(status), current.Code, current.Title, status, current.Percent(),
				money.Format(current.Progress(), current.Currency), money.Format(current.Target, current.Currency))
		}
	}
	return out
}
