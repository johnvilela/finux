package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"finux/src/budget"
	"finux/src/category"
	"finux/src/money"
	"finux/src/transaction"
)

// budgetColor returns the color of the status of a budget.
func budgetColor(status string) string {
	switch status {
	case budget.LevelOK:
		return colorIncome
	case budget.LevelNear:
		return colorWarning
	case budget.LevelOver:
		return colorExpense
	}
	return colorNeutral
}

// budgetUsage returns the period a list shows for a budget: the current
// one, the last one of a budget that ended, or the first one of a
// budget that did not start.
func budgetUsage(current budget.Budget, today string) budget.Usage {
	if usage, ok := current.Shown(today); ok {
		return usage
	}
	first, _ := current.PeriodAt(current.StartDate)
	return budget.Usage{Period: first, Limit: current.Limit}
}

// span returns the days of a period: "01/11/2026 to 30/11/2026".
func span(period budget.Period) string {
	return transaction.FormatDate(period.From) + " to " + transaction.FormatDate(period.To)
}

// BudgetEntries makes selector rows from budgets.
func BudgetEntries(budgets []budget.Budget, today string) []Entry {
	entries := make([]Entry, len(budgets))
	for i, current := range budgets {
		status := current.Status(today)
		description := current.Recurrence() + "  " + status
		if current.Archived() {
			description = "archived  " + description
		}
		if current.Description != "" {
			description += "  " + current.Description
		}
		usage := budgetUsage(current, today)
		entries[i] = Entry{
			Color: budgetColor(status), Code: current.Code, Name: current.Title, Description: description,
			Value: fmt.Sprintf("%d%% of %s", usage.Percent(), money.Format(usage.Available(), current.Currency)),
		}
	}
	return entries
}

// BudgetTable renders budgets as a table. Plain output has no colors and
// no borders, for pipes and files.
func BudgetTable(budgets []budget.Budget, today string, plain bool) string {
	if len(budgets) == 0 {
		return "No budgets yet. Run 'finux bg add'.\n"
	}
	headers := []string{"CODE", "TITLE", "PERIOD", "UNTIL", "STATUS", "SPENT", "%", "AVAILABLE"}
	const statusColumn, firstAmount, lastColumn = 4, 5, 7
	rows := make([][]string, len(budgets))
	statuses := make([]string, len(budgets))
	for i, current := range budgets {
		statuses[i] = current.Status(today)
		title := current.Title
		if current.Archived() {
			title += " (archived)"
		}
		usage := budgetUsage(current, today)
		rows[i] = []string{
			current.Code, title, current.Recurrence(), transaction.FormatDate(usage.To), statuses[i],
			money.Format(usage.Spent, current.Currency), strconv.Itoa(usage.Percent()) + "%",
			money.Format(usage.Available(), current.Currency),
		}
	}
	count := fmt.Sprintf("%d budgets", len(budgets))
	if len(budgets) == 1 {
		count = "1 budget"
	}
	if plain {
		return plainTable(headers, rows, lastColumn) + count + "\n"
	}
	rendered := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styleDim).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if column >= firstAmount {
				style = style.Align(lipgloss.Right)
			}
			switch {
			case row == table.HeaderRow:
				return style.Foreground(colorAccent).Bold(true)
			case budgets[row].Archived():
				return style.Foreground(colorDim)
			case column == statusColumn:
				return style.Foreground(lipgloss.Color(budgetColor(statuses[row])))
			case column == 2 || column == 3:
				return style.Foreground(colorDim)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// BudgetDetails is what the details of a budget show.
type BudgetDetails struct {
	Budget budget.Budget
	// Usage is the period to show. It is nil for a budget that did not
	// start.
	Usage *budget.Usage
	// Categories are the categories of the budget. No categories means
	// all categories.
	Categories []category.Category
	// Transactions are the transactions that count in the period, the
	// newest first.
	Transactions []transaction.View
	// History holds the periods before the one to show, the newest first.
	History []budget.Usage
	// Today is the current date as YYYY-MM-DD.
	Today string
	// Scheduled is what the schedules of the categories of the budget
	// still take from the current period. It is 0 for another period.
	Scheduled int64
}

// status returns the status of the budget, or the level of the period to
// show when it is not the period of today.
func (d BudgetDetails) status() string {
	if d.Usage != nil && !d.current() && d.Today > d.Usage.To && d.Budget.Status(d.Today) != budget.StatusEnded {
		return d.Usage.Level()
	}
	return d.Budget.Status(d.Today)
}

// current reports whether the period to show is the one of today.
func (d BudgetDetails) current() bool {
	return d.Usage != nil && d.Today >= d.Usage.From && d.Today <= d.Usage.To
}

// heading returns the title of the period to show.
func (d BudgetDetails) heading() string {
	if d.current() {
		return "Current period: " + span(d.Usage.Period)
	}
	return "Period: " + span(d.Usage.Period)
}

// categories returns the codes of the categories, or "all".
func (d BudgetDetails) categories() string {
	if len(d.Categories) == 0 {
		return "all"
	}
	codes := make([]string, len(d.Categories))
	for i, current := range d.Categories {
		codes[i] = current.Code
	}
	return strings.Join(codes, ", ")
}

const noCounted = "No transactions count in this period."

// starts returns the message of a budget that did not start.
func (d BudgetDetails) starts() string {
	return "The budget starts on " + transaction.FormatDate(d.Budget.StartDate) + "."
}

// yesNo returns a flag in words.
func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// BudgetDetailsView renders a budget with one period, the transactions
// that count in it and the periods before it.
func BudgetDetailsView(details BudgetDetails, plain bool) string {
	if !plain {
		return styledBudgetDetails(details)
	}
	current := details.Budget
	format := func(amount int64) string { return money.Format(amount, current.Currency) }
	fields := [][2]string{
		{"ID", strconv.FormatUint(uint64(current.ID), 10)},
		{"Currency", current.Currency},
		{"Limit", format(current.Limit)},
		{"Period", current.Recurrence()},
		{"Start", transaction.FormatDate(current.StartDate)},
	}
	if current.EndDate != "" {
		fields = append(fields, [2]string{"End", transaction.FormatDate(current.EndDate)})
	}
	status := details.status()
	if current.Archived() {
		status += ", archived"
	}
	fields = append(fields,
		[2]string{"Rollover", yesNo(current.Rollover)},
		[2]string{"Categories", details.categories()},
		[2]string{"Status", status},
		[2]string{"Created", current.CreatedAt.Local().Format(timeLayout)},
		[2]string{"Updated", current.UpdatedAt.Local().Format(timeLayout)},
	)
	lines := []string{current.Code + "  " + current.Title}
	if current.Description != "" {
		lines = append(lines, current.Description)
	}
	lines = append(lines, "")
	row := func(field [2]string) string { return fmt.Sprintf("%-12s%s", field[0], field[1]) }
	for _, field := range fields {
		lines = append(lines, row(field))
	}
	lines = append(lines, "")
	if details.Usage == nil {
		return strings.Join(append(lines, details.starts()), "\n") + "\n"
	}
	usage := *details.Usage
	lines = append(lines, details.heading())
	if current.Rollover {
		lines = append(lines, row([2]string{"Carried", format(usage.Carried)}))
	}
	lines = append(lines,
		row([2]string{"Available", format(usage.Available())}),
		row([2]string{"Spent", fmt.Sprintf("%s (%d%%)", format(usage.Spent), usage.Percent())}),
	)
	if details.Scheduled > 0 {
		lines = append(lines, row([2]string{"Scheduled", format(details.Scheduled)}))
	}
	if usage.Over() > 0 {
		lines = append(lines, row([2]string{"Over", format(usage.Over())}))
	} else {
		lines = append(lines, row([2]string{"Remaining", format(usage.Remaining())}))
	}
	out := strings.Join(lines, "\n") + "\n\n"
	if len(details.Transactions) == 0 {
		out += noCounted + "\n"
	} else {
		rows := make([][]string, len(details.Transactions))
		for i, view := range details.Transactions {
			rows[i] = []string{
				strconv.FormatUint(uint64(view.ID), 10), transaction.FormatDate(view.Date), view.Title, view.Type,
				view.Category.Code, view.Account.Code, format(-view.Amount),
			}
		}
		out += plainTable([]string{"ID", "DATE", "TITLE", "TYPE", "CATEGORY", "ACCOUNT", "AMOUNT"}, rows, 6)
	}
	if len(details.History) == 0 {
		return out
	}
	rows := make([][]string, len(details.History))
	for i, past := range details.History {
		rows[i] = []string{span(past.Period), past.Level(), format(past.Spent), strconv.Itoa(past.Percent()) + "%", format(past.Available())}
	}
	return out + "\nHistory\n" + plainTable([]string{"PERIOD", "STATUS", "SPENT", "%", "AVAILABLE"}, rows, 4)
}

// usageBar draws the spent part of the amount of a period.
func usageBar(usage budget.Usage, tint lipgloss.Style) string {
	filled := min(usage.Percent(), 100) * barWidth / 100
	if usage.Spent > 0 && filled == 0 {
		filled = 1
	}
	return tint.Render(strings.Repeat("█", filled)) + styleDim.Render(strings.Repeat("░", barWidth-filled))
}

func styledBudgetDetails(details BudgetDetails) string {
	current := details.Budget
	status := details.status()
	tint := lipgloss.Color(budgetColor(status))
	width := detailsWidth
	format := func(amount int64) string { return money.Format(amount, current.Currency) }
	spread := func(left, right string) string {
		return left + strings.Repeat(" ", max(2, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
	}

	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(current.Code)
	header := badge + "  " + styleBold.Render(cut(current.Title, width-34))
	if current.Archived() {
		header += "  " + styleDim.Render("ARCHIVED")
	}
	lines := []string{spread(header, styleDim.Render(strings.ToUpper(current.Recurrence())+"  #"+strconv.FormatUint(uint64(current.ID), 10)))}
	if current.Description != "" {
		lines = append(lines, styleDim.Italic(true).Width(width).Render(current.Description))
	}
	loud := lipgloss.NewStyle().Foreground(tint).Bold(true).Render(strings.ToUpper(status))
	rollover := "no rollover"
	if current.Rollover {
		rollover = "rollover"
	}
	footer := []string{
		styleDim.Render(strings.Repeat("─", width)),
		styleDim.Render("Categories  " + cut(details.categories(), width-12)),
		styleDim.Render("Limit       " + format(current.Limit) + "  ·  " + rollover),
		styleDim.Render("Created     " + current.CreatedAt.Local().Format(timeLayout)),
	}
	if details.Usage == nil {
		lines = append(lines, "", spread(styleDim.Render(details.starts()), loud), "")
		return styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(append(lines, footer...), "\n")) + "\n"
	}
	usage := *details.Usage
	left := format(usage.Remaining()) + " left"
	if usage.Over() > 0 {
		left = format(usage.Over()) + " over"
	}
	if current.Rollover {
		left += "  ·  " + signedMoney(usage.Carried, current.Currency) + " carried"
	}
	if details.Scheduled > 0 {
		left += "  ·  " + format(details.Scheduled) + " scheduled"
	}
	lines = append(lines,
		"",
		styleDim.Render("SPENT  ·  "+strings.TrimPrefix(strings.TrimPrefix(details.heading(), "Current period: "), "Period: ")),
		spread(lipgloss.NewStyle().Bold(true).Render(format(usage.Spent)), loud),
		"",
		usageBar(usage, lipgloss.NewStyle().Foreground(tint))+"  "+fmt.Sprintf("%d%% ", usage.Percent())+
			styleDim.Render("of "+format(usage.Available())),
		"",
		styleDim.Render(left),
	)
	out := styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(append(lines, footer...), "\n")) + "\n"

	const indent = "   "
	out += indent + styleTitle.Render("TRANSACTIONS OF THE PERIOD") + "\n"
	out += indent + styleDim.Render(strings.Repeat("─", width)) + "\n"
	if len(details.Transactions) == 0 {
		out += indent + styleDim.Render(noCounted) + "\n"
	}
	const dateWidth, groupWidth, amountWidth = 10, 7, 16
	textWidth := width - dateWidth - 2 - 2 - 2 - groupWidth - 2 - amountWidth
	for _, view := range details.Transactions {
		// An expense takes from the budget; an income gives back.
		sign, color := paint(colorExpense, "▼"), colorExpense
		if view.Amount > 0 {
			sign, color = paint(colorIncome, "▲"), colorIncome
		}
		title := cut(view.Title, textWidth)
		title += strings.Repeat(" ", max(0, textWidth-lipgloss.Width(title)))
		dot, _ := category.ColorByKey(view.Category.Color)
		group := paint(dot.Hex, "●") + " " + view.Category.Code
		group += strings.Repeat(" ", max(0, groupWidth-lipgloss.Width(group)))
		value := format(-view.Amount)
		value = strings.Repeat(" ", max(0, amountWidth-lipgloss.Width(value))) + paint(color, value)
		out += indent + styleDim.Render(transaction.FormatDate(view.Date)) + "  " + sign + " " + title + "  " + group + "  " + value + "\n"
	}
	if len(details.History) == 0 {
		return out
	}
	out += "\n" + indent + styleTitle.Render("HISTORY") + "\n"
	out += indent + styleDim.Render(strings.Repeat("─", width)) + "\n"
	for _, past := range details.History {
		level := past.Level()
		label := level + strings.Repeat(" ", max(0, 5-len(level)))
		amounts := fmt.Sprintf("%s of %s  %4d%%", format(past.Spent), format(past.Available()), past.Percent())
		line := styleDim.Render(span(past.Period)) + "  " + paint(budgetColor(level), label)
		out += indent + spread(line, amounts) + "\n"
	}
	return out
}
