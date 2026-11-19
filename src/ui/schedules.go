package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"finux/src/account"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/schedule"
	"finux/src/transaction"
)

// ScheduleView is a schedule with the records it points to.
type ScheduleView struct {
	schedule.Schedule
	// Account is the account of the schedule. For a schedule of a credit
	// card it holds the name, code, color and currency of the card, and
	// OnCard is true.
	Account  account.Account
	OnCard   bool
	Category category.Category
	// Goal is the goal of the schedule, or nil.
	Goal *goal.Goal
}

// Amount returns the value of the schedule with the sign of its type:
// negative for an expense.
func (v ScheduleView) Amount() int64 {
	if v.Type == schedule.Expense {
		return -v.Value
	}
	return v.Value
}

// scheduleColor returns the color of the status of a schedule.
func scheduleColor(status string) string {
	switch status {
	case schedule.StatusDue:
		return colorWarning
	case schedule.StatusDone:
		return colorIncome
	}
	return colorNeutral
}

// scheduleStatus returns the status of a schedule for a list: "due",
// "3 due" for more than one occurrence, "upcoming" or "done".
func scheduleStatus(current schedule.Schedule, today string) string {
	status := current.Status(today)
	if pending := len(current.Pending(today)); pending > 1 {
		return fmt.Sprintf("%d %s", pending, status)
	}
	return status
}

// scheduleNext returns the date of the next occurrence, or "-".
func scheduleNext(current schedule.Schedule) string {
	if next, ok := current.Next(); ok {
		return transaction.FormatDate(next)
	}
	return "-"
}

// ScheduleEntries makes selector rows from schedules.
func ScheduleEntries(views []ScheduleView, today string) []Entry {
	entries := make([]Entry, len(views))
	for i, view := range views {
		description := view.Recurrence() + "  " + scheduleStatus(view.Schedule, today) + "  " + scheduleNext(view.Schedule)
		if view.Archived() {
			description = "archived  " + description
		}
		entries[i] = Entry{
			Color: scheduleColor(view.Status(today)), Code: view.Code, Name: view.Title, Description: description,
			Value: money.Format(view.Amount(), view.Account.Currency), Negative: view.Amount() < 0,
		}
	}
	return entries
}

const noSchedules = "No scheduled transactions yet. Run 'finux sc add'.\n"

// ScheduleTable renders schedules as a table. Plain output has no colors
// and no borders, for pipes and files.
func ScheduleTable(views []ScheduleView, today string, plain bool) string {
	if len(views) == 0 {
		return noSchedules
	}
	headers := []string{"CODE", "TITLE", "REPEATS", "NEXT", "STATUS", "ACCOUNT", "VALUE"}
	const statusColumn, valueColumn = 4, 6
	rows := make([][]string, len(views))
	for i, view := range views {
		title := view.Title
		if view.Archived() {
			title += " (archived)"
		}
		rows[i] = []string{
			view.Code, title, view.Recurrence(), scheduleNext(view.Schedule), scheduleStatus(view.Schedule, today),
			view.Account.Code, money.Format(view.Amount(), view.Account.Currency),
		}
	}
	count := fmt.Sprintf("%d schedules", len(views))
	if len(views) == 1 {
		count = "1 schedule"
	}
	if plain {
		return plainTable(headers, rows, valueColumn) + count + "\n"
	}
	rendered := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styleDim).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if column == valueColumn {
				style = style.Align(lipgloss.Right)
			}
			switch {
			case row == table.HeaderRow:
				return style.Foreground(colorAccent).Bold(true)
			case views[row].Archived():
				return style.Foreground(colorDim)
			case column == statusColumn:
				return style.Foreground(lipgloss.Color(scheduleColor(views[row].Status(today))))
			case column == valueColumn:
				return style.Foreground(typeStyle(views[row].Type).GetForeground())
			case column == 2 || column == 3:
				return style.Foreground(colorDim)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// ScheduleDetails is what the details of a schedule show.
type ScheduleDetails struct {
	View ScheduleView
	// Transactions are the transactions the schedule created, the newest
	// first.
	Transactions []transaction.View
	// Today is the current date as YYYY-MM-DD.
	Today string
	// AutomaticOff tells that the setting for automatic schedules is off:
	// an automatic schedule then waits as a manual one.
	AutomaticOff bool
}

// maxDueShown is the number of due dates that the details list.
const maxDueShown = 6

// due returns the dates of the occurrences that wait, in words.
func (d ScheduleDetails) due() string {
	if d.View.Archived() {
		return ""
	}
	pending := d.View.Pending(d.Today)
	shown := make([]string, 0, maxDueShown)
	for _, date := range pending[:min(len(pending), maxDueShown)] {
		shown = append(shown, transaction.FormatDate(date))
	}
	text := strings.Join(shown, ", ")
	if more := len(pending) - len(shown); more > 0 {
		text += fmt.Sprintf(" and %d more", more)
	}
	return text
}

// next returns the date of the first occurrence after today, or "".
func (d ScheduleDetails) next() string {
	later := d.View.Schedule
	later.Done += len(later.Pending(d.Today))
	if next, ok := later.Next(); ok {
		return transaction.FormatDate(next)
	}
	return ""
}

// status returns the status of the schedule in words.
func (d ScheduleDetails) status() string {
	status := scheduleStatus(d.View.Schedule, d.Today)
	if d.View.Archived() {
		status += ", archived"
	}
	return status
}

// confirm tells how an occurrence becomes a transaction.
func (d ScheduleDetails) confirm() string {
	switch {
	case d.View.Auto && d.AutomaticOff:
		return "automatic (off in 'finux config')"
	case d.View.Auto:
		return "automatic"
	}
	return "manual"
}

const noScheduleTransactions = "No transactions yet."

// ScheduleDetailsView renders a schedule, the occurrences that wait and
// the transactions it created.
func ScheduleDetailsView(details ScheduleDetails, plain bool) string {
	if !plain {
		return styledScheduleDetails(details)
	}
	view := details.View
	place := "Account"
	if view.OnCard {
		place = "Card"
	}
	fields := [][2]string{
		{"ID", strconv.FormatUint(uint64(view.ID), 10)},
		{"Type", view.Type},
		{"Value", money.Format(view.Value, view.Account.Currency)},
		{place, view.Account.Code + "  " + view.Account.Name},
		{"Category", view.Category.Code + "  " + view.Category.Name},
	}
	if view.Tags != "" {
		fields = append(fields, [2]string{"Tags", view.Tags})
	}
	if view.Goal != nil {
		fields = append(fields, [2]string{"Goal", view.Goal.Code + "  " + view.Goal.Title})
	}
	fields = append(fields, [2]string{"Repeats", view.Recurrence()})
	if view.EndDate != "" {
		fields = append(fields, [2]string{"Until", transaction.FormatDate(view.EndDate)})
	}
	if left := view.Left(); view.Recurring() && left >= 0 {
		fields = append(fields, [2]string{"Times left", strconv.Itoa(left)})
	}
	fields = append(fields, [2]string{"Confirm", details.confirm()}, [2]string{"Status", details.status()})
	if due := details.due(); due != "" {
		fields = append(fields, [2]string{"Due", due})
	}
	if next := details.next(); next != "" {
		fields = append(fields, [2]string{"Next", next})
	}
	fields = append(fields,
		[2]string{"Created", view.CreatedAt.Local().Format(timeLayout)},
		[2]string{"Updated", view.UpdatedAt.Local().Format(timeLayout)},
	)
	lines := []string{view.Code + "  " + view.Title}
	if view.Description != "" {
		lines = append(lines, view.Description)
	}
	lines = append(lines, "")
	for _, field := range fields {
		lines = append(lines, fmt.Sprintf("%-12s%s", field[0], field[1]))
	}
	out := strings.Join(lines, "\n") + "\n\n"
	if len(details.Transactions) == 0 {
		return out + noScheduleTransactions + "\n"
	}
	rows := make([][]string, len(details.Transactions))
	for i, created := range details.Transactions {
		rows[i] = []string{
			strconv.FormatUint(uint64(created.ID), 10), transaction.FormatDate(created.Date), created.Title,
			created.Account.Code, money.Format(created.Amount, created.Account.Currency),
		}
	}
	return out + plainTable([]string{"ID", "DATE", "TITLE", "ACCOUNT", "VALUE"}, rows, 4)
}

func styledScheduleDetails(details ScheduleDetails) string {
	view := details.View
	status := view.Status(details.Today)
	tint := lipgloss.Color(scheduleColor(status))
	width := detailsWidth
	spread := func(left, right string) string {
		return left + strings.Repeat(" ", max(2, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
	}
	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(view.Code)
	header := badge + "  " + styleBold.Render(cut(view.Title, width-34))
	if view.Archived() {
		header += "  " + styleDim.Render("ARCHIVED")
	}
	lines := []string{spread(header, styleDim.Render(strings.ToUpper(view.Recurrence())+"  #"+strconv.FormatUint(uint64(view.ID), 10)))}
	if view.Description != "" {
		lines = append(lines, styleDim.Italic(true).Width(width).Render(view.Description))
	}
	amount := typeStyle(view.Type).Bold(true).Render(money.Format(view.Amount(), view.Account.Currency))
	loud := lipgloss.NewStyle().Foreground(tint).Bold(true).Render(strings.ToUpper(details.status()))
	lines = append(lines, "", spread(amount, loud), "")

	row := func(label, text string) string { return styleDim.Width(12).Render(label) + text }
	place := "Account"
	if view.OnCard {
		place = "Card"
	}
	dot, _ := category.ColorByKey(view.Category.Color)
	lines = append(lines,
		row(place, swatch(view.Account.Color, "●")+" "+styleBold.Render(view.Account.Code)+"  "+view.Account.Name),
		row("Category", paint(dot.Hex, "●")+" "+styleBold.Render(view.Category.Code)+"  "+view.Category.Name),
	)
	if view.Tags != "" {
		lines = append(lines, row("Tags", view.Tags))
	}
	if view.Goal != nil {
		lines = append(lines, row("Goal", paint(colorIncome, "◎")+" "+styleBold.Render(view.Goal.Code)+"  "+view.Goal.Title))
	}
	if due := details.due(); due != "" {
		lines = append(lines, row("Due", lipgloss.NewStyle().Foreground(tint).Width(width-12).Render(due)))
	}
	if next := details.next(); next != "" {
		lines = append(lines, row("Next", next))
	}
	ends := "no end"
	switch left := view.Left(); {
	case !view.Recurring():
		ends = "one time"
	case view.EndDate != "" && left >= 0:
		ends = fmt.Sprintf("until %s  ·  %d left", transaction.FormatDate(view.EndDate), left)
	case left >= 0:
		ends = fmt.Sprintf("%d left", left)
	}
	lines = append(lines,
		"",
		styleDim.Render(strings.Repeat("─", width)),
		styleDim.Render("Confirm  "+details.confirm()+"  ·  "+ends),
		styleDim.Render("Created  "+view.CreatedAt.Local().Format(timeLayout)),
	)
	out := styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"

	const indent = "   "
	out += indent + styleTitle.Render("TRANSACTIONS") + "\n"
	out += indent + styleDim.Render(strings.Repeat("─", width)) + "\n"
	if len(details.Transactions) == 0 {
		return out + indent + styleDim.Render(noScheduleTransactions) + "\n"
	}
	for _, created := range details.Transactions {
		value := typeStyle(created.Type).Render(money.Format(created.Amount, created.Account.Currency))
		left := styleDim.Render(transaction.FormatDate(created.Date)) + "  " + cut(created.Title, 30) + "  " + styleDim.Render(created.Account.Code+"  #"+strconv.FormatUint(uint64(created.ID), 10))
		out += indent + spread(left, value) + "\n"
	}
	return out
}
