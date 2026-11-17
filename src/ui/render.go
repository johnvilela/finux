package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"finux/src/account"
	"finux/src/money"
)

const timeLayout = "2006-01-02 15:04"

// AccountTable renders the list of accounts. Plain output has no colors
// and no borders, for pipes and files.
func AccountTable(accounts []account.Account, plain bool) string {
	if len(accounts) == 0 {
		return "No accounts yet. Run 'finux ac add'.\n"
	}
	headers := []string{"CODE", "NAME", "CURRENCY", "BALANCE"}
	const balanceColumn = 3
	rows := make([][]string, len(accounts))
	for i, current := range accounts {
		name := current.Name
		if current.Archived() {
			name += " (archived)"
		}
		code := current.Code
		if !plain {
			code = swatch(current.Color, "●") + " " + code
		}
		rows[i] = []string{code, name, current.Currency, money.Format(current.Balance(), current.Currency)}
	}
	count := fmt.Sprintf("%d accounts", len(accounts))
	if len(accounts) == 1 {
		count = "1 account"
	}
	if plain {
		return plainTable(headers, rows, balanceColumn) + count + "\n"
	}
	rendered := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styleDim).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if column == balanceColumn {
				style = style.Align(lipgloss.Right)
			}
			switch {
			case row == table.HeaderRow:
				return style.Foreground(colorAccent).Bold(true)
			case accounts[row].Archived():
				return style.Foreground(colorDim)
			case column == balanceColumn && accounts[row].Balance() < 0:
				return style.Foreground(colorError)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// plainTable aligns columns with spaces. The column rightAligned is
// aligned to the right; it must be the last column.
func plainTable(headers []string, rows [][]string, rightAligned int) string {
	widths := make([]int, len(headers))
	for _, row := range append([][]string{headers}, rows...) {
		for column, cell := range row {
			widths[column] = max(widths[column], lipgloss.Width(cell))
		}
	}
	var out strings.Builder
	for _, row := range append([][]string{headers}, rows...) {
		for column, cell := range row {
			padding := strings.Repeat(" ", widths[column]-lipgloss.Width(cell))
			switch {
			case column == rightAligned:
				out.WriteString(padding + cell)
			case column == len(row)-1:
				out.WriteString(cell)
			default:
				out.WriteString(cell + padding + "  ")
			}
		}
		out.WriteString("\n")
	}
	return out.String()
}

// AccountCard renders the details of one account. Plain output has no
// colors and no borders, for pipes and files.
func AccountCard(current account.Account, plain bool) string {
	if !plain {
		return styledCard(current)
	}
	fields := [][2]string{
		{"ID", strconv.FormatUint(uint64(current.ID), 10)},
		{"Currency", current.Currency},
		{"Balance", money.Format(current.Balance(), current.Currency)},
		{"Color", current.Color},
		{"Created", current.CreatedAt.Local().Format(timeLayout)},
		{"Updated", current.UpdatedAt.Local().Format(timeLayout)},
	}
	if current.Archived() {
		fields = append(fields, [2]string{"Status", "archived"})
	}
	lines := []string{current.Code + "  " + current.Name}
	if current.Description != "" {
		lines = append(lines, current.Description)
	}
	lines = append(lines, "")
	for _, field := range fields {
		lines = append(lines, fmt.Sprintf("%-10s%s", field[0], field[1]))
	}
	return strings.Join(lines, "\n") + "\n"
}

const (
	cardMinWidth = 38
	cardMaxWidth = 60
)

// styledCard shows an account in three levels: the identity (code and
// name) first, the balance as the main value, then the dates. The id and
// the dates are dim because they matter least.
func styledCard(current account.Account) string {
	tint := colorDim
	if color, ok := account.ColorByKey(current.Color); ok {
		tint = lipgloss.Color(color.Hex)
	}

	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(current.Code)
	header := badge + "  " + styleBold.Render(current.Name)
	if current.Archived() {
		header += "  " + styleDim.Render("ARCHIVED")
	}
	id := styleDim.Render("#" + strconv.FormatUint(uint64(current.ID), 10))
	width := min(cardMaxWidth, max(cardMinWidth, lipgloss.Width(header)+2+lipgloss.Width(id)))
	header += strings.Repeat(" ", max(2, width-lipgloss.Width(header)-lipgloss.Width(id))) + id

	amount := lipgloss.NewStyle().Bold(true)
	if current.Balance() < 0 {
		amount = amount.Foreground(colorError)
	}
	balance := amount.Render(money.Format(current.Balance(), current.Currency))
	currency := styleDim.Render(current.Currency)
	gap := max(2, width-lipgloss.Width(balance)-lipgloss.Width(currency))

	lines := []string{header}
	if current.Description != "" {
		lines = append(lines, styleDim.Italic(true).Width(width).Render(current.Description))
	}
	lines = append(lines,
		"",
		styleDim.Render("BALANCE"),
		balance+strings.Repeat(" ", gap)+currency,
		"",
		styleDim.Render(strings.Repeat("─", width)),
	)
	lines = append(lines,
		styleDim.Render("Created  "+current.CreatedAt.Local().Format(timeLayout)),
		styleDim.Render("Updated  "+current.UpdatedAt.Local().Format(timeLayout)),
	)
	return styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"
}
