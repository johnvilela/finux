package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"finux/src/category"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/transaction"
)

// colorNeutral is the color of a goal that is active.
const colorNeutral = "#9CA3AF"

// goalColor returns the color of the status of a goal.
func goalColor(status string) string {
	switch status {
	case goal.StatusReached:
		return colorIncome
	case goal.StatusOverdue:
		return colorExpense
	}
	return colorNeutral
}

// goalTypeLabel returns the type of a goal as the forms show it.
func goalTypeLabel(kind string) string {
	if kind == goal.PayOff {
		return "pay off"
	}
	return kind
}

// goalDue returns the final date of a goal for a table, or "-".
func goalDue(current goal.Goal) string {
	if current.DueDate == "" {
		return "-"
	}
	return transaction.FormatDate(current.DueDate)
}

// GoalEntries makes selector rows from goals.
func GoalEntries(goals []goal.Goal, today string) []Entry {
	entries := make([]Entry, len(goals))
	for i, current := range goals {
		status := current.Status(today)
		description := current.Type + "  " + status
		if current.Archived() {
			description = "archived  " + description
		}
		if current.Description != "" {
			description += "  " + current.Description
		}
		entries[i] = Entry{
			Color: goalColor(status), Code: current.Code, Name: current.Title, Description: description,
			Value: fmt.Sprintf("%d%% of %s", current.Percent(), money.Format(current.Target, current.Currency)),
		}
	}
	return entries
}

// GoalTable renders goals as a table. Plain output has no colors and no
// borders, for pipes and files.
func GoalTable(goals []goal.Goal, today string, plain bool) string {
	if len(goals) == 0 {
		return "No goals yet. Run 'finux gl add'.\n"
	}
	headers := []string{"CODE", "TITLE", "TYPE", "DUE", "STATUS", "PROGRESS", "%", "TARGET"}
	const statusColumn, firstAmount, targetColumn = 4, 5, 7
	rows := make([][]string, len(goals))
	statuses := make([]string, len(goals))
	for i, current := range goals {
		statuses[i] = current.Status(today)
		title := current.Title
		if current.Archived() {
			title += " (archived)"
		}
		rows[i] = []string{
			current.Code, title, current.Type, goalDue(current), statuses[i],
			money.Format(current.Progress(), current.Currency), strconv.Itoa(current.Percent()) + "%",
			money.Format(current.Target, current.Currency),
		}
	}
	count := fmt.Sprintf("%d goals", len(goals))
	if len(goals) == 1 {
		count = "1 goal"
	}
	if plain {
		return plainTable(headers, rows, targetColumn) + count + "\n"
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
			case goals[row].Archived():
				return style.Foreground(colorDim)
			case column == statusColumn:
				return style.Foreground(lipgloss.Color(goalColor(statuses[row])))
			case column == 2 || column == 3:
				return style.Foreground(colorDim)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// GoalDetails is what the details of a goal show.
type GoalDetails struct {
	Goal goal.Goal
	// Transactions are the transactions linked to the goal, the newest
	// first. A transfer has its two sides here; the details show one.
	Transactions []transaction.View
	// Today is the current date as YYYY-MM-DD.
	Today string
}

// effects returns the linked transactions that count, each with what it
// adds to the goal.
func (d GoalDetails) effects() (views []transaction.View, effects []int64) {
	for _, view := range d.Transactions {
		if effect := d.Goal.Effect(view.Type, view.Amount); effect != 0 {
			views, effects = append(views, view), append(effects, effect)
		}
	}
	return views, effects
}

// signedMoney formats an effect with its sign: "+R$ 10,00".
func signedMoney(amount int64, currency string) string {
	if amount > 0 {
		return "+" + money.Format(amount, currency)
	}
	return money.Format(amount, currency)
}

const noLinks = "No transactions linked to this goal."

// GoalDetailsView renders a goal with its progress and its linked
// transactions.
func GoalDetailsView(details GoalDetails, plain bool) string {
	if !plain {
		return styledGoalDetails(details)
	}
	current := details.Goal
	fields := [][2]string{
		{"ID", strconv.FormatUint(uint64(current.ID), 10)},
		{"Type", current.Type},
		{"Currency", current.Currency},
		{"Target", money.Format(current.Target, current.Currency)},
		{"Progress", fmt.Sprintf("%s (%d%%)", money.Format(current.Progress(), current.Currency), current.Percent())},
		{"Remaining", money.Format(current.Remaining(), current.Currency)},
	}
	if current.DueDate != "" {
		fields = append(fields, [2]string{"Due", transaction.FormatDate(current.DueDate)})
	}
	status := current.Status(details.Today)
	if current.Archived() {
		status += ", archived"
	}
	fields = append(fields,
		[2]string{"Status", status},
		[2]string{"Created", current.CreatedAt.Local().Format(timeLayout)},
		[2]string{"Updated", current.UpdatedAt.Local().Format(timeLayout)},
	)
	lines := []string{current.Code + "  " + current.Title}
	if current.Description != "" {
		lines = append(lines, current.Description)
	}
	lines = append(lines, "")
	for _, field := range fields {
		lines = append(lines, fmt.Sprintf("%-11s%s", field[0], field[1]))
	}
	out := strings.Join(lines, "\n") + "\n\n"
	views, effects := details.effects()
	if len(views) == 0 {
		return out + noLinks + "\n"
	}
	rows := make([][]string, len(views))
	for i, view := range views {
		rows[i] = []string{
			strconv.FormatUint(uint64(view.ID), 10), transaction.FormatDate(view.Date), view.Title, view.Type,
			view.Account.Code, signedMoney(effects[i], current.Currency),
		}
	}
	return out + plainTable([]string{"ID", "DATE", "TITLE", "TYPE", "ACCOUNT", "EFFECT"}, rows, 5)
}

// progressBar draws the part of the target a goal has.
func progressBar(current goal.Goal, tint lipgloss.Style) string {
	filled := current.Percent() * barWidth / 100
	if current.Progress() > 0 && filled == 0 {
		filled = 1
	}
	return tint.Render(strings.Repeat("█", filled)) + styleDim.Render(strings.Repeat("░", barWidth-filled))
}

func styledGoalDetails(details GoalDetails) string {
	current := details.Goal
	status := current.Status(details.Today)
	tint := lipgloss.Color(goalColor(status))
	width := detailsWidth
	spread := func(left, right string) string {
		return left + strings.Repeat(" ", max(2, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
	}

	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(current.Code)
	header := badge + "  " + styleBold.Render(cut(current.Title, width-30))
	if current.Archived() {
		header += "  " + styleDim.Render("ARCHIVED")
	}
	lines := []string{spread(header, styleDim.Render(strings.ToUpper(goalTypeLabel(current.Type))+"  #"+strconv.FormatUint(uint64(current.ID), 10)))}
	if current.Description != "" {
		lines = append(lines, styleDim.Italic(true).Width(width).Render(current.Description))
	}
	due := "no final date"
	if current.DueDate != "" {
		due = "until " + transaction.FormatDate(current.DueDate)
	}
	lines = append(lines,
		"",
		styleDim.Render("PROGRESS"),
		spread(lipgloss.NewStyle().Bold(true).Render(money.Format(current.Progress(), current.Currency)),
			lipgloss.NewStyle().Foreground(tint).Bold(true).Render(strings.ToUpper(status))),
		"",
		progressBar(current, lipgloss.NewStyle().Foreground(tint))+"  "+fmt.Sprintf("%d%% ", current.Percent())+
			styleDim.Render("of "+money.Format(current.Target, current.Currency)),
		"",
		styleDim.Render(strings.Repeat("─", width)),
		styleDim.Render(money.Format(current.Remaining(), current.Currency)+" to go  ·  "+due),
		styleDim.Render("Created  "+current.CreatedAt.Local().Format(timeLayout)),
	)
	out := styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"

	const indent = "   "
	out += indent + styleTitle.Render("LINKED TRANSACTIONS") + "\n"
	out += indent + styleDim.Render(strings.Repeat("─", width)) + "\n"
	views, effects := details.effects()
	if len(views) == 0 {
		return out + indent + styleDim.Render(noLinks) + "\n"
	}
	const dateWidth, groupWidth, amountWidth = 10, 7, 16
	textWidth := width - dateWidth - 2 - 2 - 2 - groupWidth - 2 - amountWidth
	for i, view := range views {
		sign, color := paint(colorIncome, "▲"), colorIncome
		if effects[i] < 0 {
			sign, color = paint(colorExpense, "▼"), colorExpense
		}
		title := cut(view.Title, textWidth)
		title += strings.Repeat(" ", max(0, textWidth-lipgloss.Width(title)))
		dot, _ := category.ColorByKey(view.Category.Color)
		group := paint(dot.Hex, "●") + " " + view.Category.Code
		group += strings.Repeat(" ", max(0, groupWidth-lipgloss.Width(group)))
		value := signedMoney(effects[i], current.Currency)
		value = strings.Repeat(" ", max(0, amountWidth-lipgloss.Width(value))) + paint(color, value)
		out += indent + styleDim.Render(transaction.FormatDate(view.Date)) + "  " + sign + " " + title + "  " + group + "  " + value + "\n"
	}
	return out
}
