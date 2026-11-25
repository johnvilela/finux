package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/money"
	"finux/src/transaction"
)

const timeLayout = "2006-01-02 15:04"

// AccountTable renders the list of accounts. Plain output has no colors
// and no borders, for pipes and files.
func AccountTable(accounts []account.Account, plain bool) string {
	if len(accounts) == 0 {
		return "No accounts yet. Run 'finux ac add'.\n"
	}
	headers := []string{"CODE", "NAME", "CURRENCY", "BALANCE"}
	owned := false
	for _, current := range accounts {
		owned = owned || !current.Mine()
	}
	// The owner column shows only when an account of another person is
	// in the list.
	if owned {
		headers = []string{"CODE", "NAME", "OWNER", "CURRENCY", "BALANCE"}
	}
	balanceColumn := len(headers) - 1
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
		if owned {
			rows[i] = slices.Insert(rows[i], 2, current.Owner)
		}
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
	if !current.Mine() {
		fields = slices.Insert(fields, 1, [2]string{"Owner", current.Owner})
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
	if !current.Mine() {
		header += "  " + styleDim.Render(current.Owner)
	}
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

// CategoryTable renders the list of categories. Plain output has no
// colors and no borders, for pipes and files.
func CategoryTable(categories []category.Category, plain bool) string {
	if len(categories) == 0 {
		return "No categories yet. Run 'finux ct add'.\n"
	}
	headers := []string{"CODE", "NAME", "TYPE"}
	const typeColumn = 2
	rows := make([][]string, len(categories))
	hexes := make([]string, len(categories))
	for i, current := range categories {
		color, _ := category.ColorByKey(current.Color)
		hexes[i] = color.Hex
		label := current.Code
		if !plain {
			label = paint(color.Hex, "●") + " " + label
		}
		rows[i] = []string{label, current.Name, current.Meaning()}
	}
	count := fmt.Sprintf("%d categories", len(categories))
	if len(categories) == 1 {
		count = "1 category"
	}
	if plain {
		return plainTable(headers, rows, -1) + count + "\n"
	}
	rendered := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styleDim).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			switch {
			case row == table.HeaderRow:
				return style.Foreground(colorAccent).Bold(true)
			case column == typeColumn:
				return style.Foreground(lipgloss.Color(hexes[row]))
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// CategoryCard renders the details of one category. It is simple for
// now; it will show more when transactions exist.
func CategoryCard(current category.Category, plain bool) string {
	id := strconv.FormatUint(uint64(current.ID), 10)
	created := current.CreatedAt.Local().Format(timeLayout)
	updated := current.UpdatedAt.Local().Format(timeLayout)
	if plain {
		lines := []string{current.Code + "  " + current.Name}
		if current.Description != "" {
			lines = append(lines, current.Description)
		}
		lines = append(lines, "")
		for _, field := range [][2]string{
			{"ID", id}, {"Type", current.Meaning()}, {"Color", current.Color}, {"Created", created}, {"Updated", updated},
		} {
			lines = append(lines, fmt.Sprintf("%-10s%s", field[0], field[1]))
		}
		return strings.Join(lines, "\n") + "\n"
	}

	tint := colorDim
	if color, ok := category.ColorByKey(current.Color); ok {
		tint = lipgloss.Color(color.Hex)
	}
	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(current.Code)
	header := badge + "  " + styleBold.Render(current.Name)
	number := styleDim.Render("#" + id)
	width := min(cardMaxWidth, max(cardMinWidth, lipgloss.Width(header)+2+lipgloss.Width(number)))
	header += strings.Repeat(" ", max(2, width-lipgloss.Width(header)-lipgloss.Width(number))) + number

	lines := []string{header}
	if current.Description != "" {
		lines = append(lines, styleDim.Italic(true).Width(width).Render(current.Description))
	}
	lines = append(lines,
		"",
		lipgloss.NewStyle().Foreground(tint).Render(current.Meaning()),
		"",
		styleDim.Render(strings.Repeat("─", width)),
		styleDim.Render("Created  "+created),
		styleDim.Render("Updated  "+updated),
	)
	return styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"
}

// virtualMark shows that a transaction was made with a virtual card.
const virtualMark = "[virtual]"

// titled returns the title of a transaction with its part, such as
// "Notebook 3/10", for a transaction in installments, and the mark of
// a virtual card.
func titled(view transaction.View) string {
	title := view.Title
	if part := view.Installment(); part != "" {
		title += " " + part
	}
	return marked(title, view)
}

// marked adds the mark of a virtual card to a title.
func marked(title string, view transaction.View) string {
	if view.Virtual {
		return title + " " + virtualMark
	}
	return title
}

// virtualTotal returns the sum and the number of the transactions made
// with a virtual card, as text such as "R$ 40,00 (1 transaction)". It
// is "" when there are none.
func virtualTotal(views []transaction.View, currency string) string {
	total, count := transaction.VirtualTotal(views)
	switch count {
	case 0:
		return ""
	case 1:
		return money.Format(total, currency) + " (1 transaction)"
	}
	return fmt.Sprintf("%s (%d transactions)", money.Format(total, currency), count)
}

// TransactionTable renders a list of transactions. Plain output has no
// colors and no borders, for pipes and files.
func TransactionTable(views []transaction.View, plain bool) string {
	if len(views) == 0 {
		return "No transactions yet. Run 'finux tx add'.\n"
	}
	headers := []string{"ID", "DATE", "TITLE", "CATEGORY", "ACCOUNT", "VALUE"}
	const categoryColumn, valueColumn = 3, 5
	rows := make([][]string, len(views))
	for i, view := range views {
		label := view.Category.Code
		if color, ok := category.ColorByKey(view.Category.Color); ok && !plain {
			label = paint(color.Hex, "●") + " " + label
		}
		rows[i] = []string{
			strconv.FormatUint(uint64(view.ID), 10), transaction.FormatDate(view.Date), titled(view), label,
			view.Account.Code, money.Format(view.Amount, view.Account.Currency),
		}
	}
	count := fmt.Sprintf("%d transactions", len(views))
	if len(views) == 1 {
		count = "1 transaction"
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
			case column == valueColumn:
				return style.Inherit(typeStyle(views[row].Type))
			case column == 0 || column == 1:
				return style.Foreground(colorDim)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// transferLine describes the other side of a transfer: "to WISE  Wise".
func transferLine(view transaction.View) (direction, account string) {
	direction = "from"
	if view.Outgoing() {
		direction = "to"
	}
	return direction, view.Other.Code + "  " + view.Other.Name
}

// TransactionCard renders the details of one transaction. Plain output
// has no colors and no borders, for pipes and files.
func TransactionCard(view transaction.View, plain bool) string {
	id := strconv.FormatUint(uint64(view.ID), 10)
	value := money.Format(view.Amount, view.Account.Currency)
	date := transaction.FormatDate(view.Date)
	created := view.CreatedAt.Local().Format(timeLayout)
	updated := view.UpdatedAt.Local().Format(timeLayout)
	// A transaction of a credit card shows the card in place of the
	// account, and its bill.
	purchase := view.CardID != 0
	place, period := "Account", ""
	if purchase {
		place = "Card"
	}
	if view.Bill != nil {
		period = card.FormatPeriod(view.Bill.Period)
	}
	part := view.Installment()
	if plain {
		fields := [][2]string{
			{"ID", id}, {"Type", view.Type}, {"Value", value},
			{place, view.Account.Code + "  " + view.Account.Name},
			{"Category", view.Category.Code + "  " + view.Category.Name},
			{"Date", date},
		}
		if view.Status != "" {
			fields = append(fields, [2]string{"Status", view.Status})
		}
		if part != "" {
			fields = append(fields, [2]string{"Part", part})
		}
		if purchase {
			fields = append(fields, [2]string{"Bill", period})
		}
		if view.Virtual {
			fields = append(fields, [2]string{"Virtual", "yes"})
		}
		if view.Tags != "" {
			fields = append(fields, [2]string{"Tags", view.Tags})
		}
		if view.Goal != nil {
			fields = append(fields, [2]string{"Goal", view.Goal.Code + "  " + view.Goal.Title})
		}
		if view.Schedule != nil {
			fields = append(fields, [2]string{"Schedule", view.Schedule.Code + "  " + view.Schedule.Title})
		}
		switch {
		case view.Type == transaction.Payment && view.Other != nil:
			fields = append(fields, [2]string{"Payment", "bill " + period + " of " + view.Other.Code + "  " + view.Other.Name})
		case view.Other != nil:
			direction, other := transferLine(view)
			fields = append(fields, [2]string{"Transfer", fmt.Sprintf("%s %s (transaction %d)", direction, other, view.PairID)})
		}
		fields = append(fields, [2]string{"Created", created}, [2]string{"Updated", updated})
		lines := []string{view.Title}
		if view.Description != "" {
			lines = append(lines, view.Description)
		}
		lines = append(lines, "")
		for _, field := range fields {
			lines = append(lines, fmt.Sprintf("%-10s%s", field[0], field[1]))
		}
		return strings.Join(lines, "\n") + "\n"
	}

	// Three levels: what it is (type and title), how much, then where
	// and when. The id and the record dates are dim: they matter least.
	// The card has the color of its type: green for an income, red for
	// an expense, no color for a transfer.
	tint := colorDim
	switch view.Type {
	case transaction.Expense:
		tint = lipgloss.Color(colorExpense)
	case transaction.Income:
		tint = lipgloss.Color(colorIncome)
	}
	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(strings.ToUpper(view.Type))
	number := styleDim.Render("#" + id)
	width := min(cardMaxWidth, max(cardMinWidth, lipgloss.Width(badge)+2+lipgloss.Width(view.Title)+2+lipgloss.Width(number)))
	title := view.Title
	if room := width - lipgloss.Width(badge) - 4 - lipgloss.Width(number); lipgloss.Width(title) > room {
		title = string([]rune(title)[:room-1]) + "…"
	}
	header := badge + "  " + styleBold.Render(title)
	header += strings.Repeat(" ", max(2, width-lipgloss.Width(header)-lipgloss.Width(number))) + number

	amount := typeStyle(view.Type).Bold(true).Render(value)
	currency := styleDim.Render(view.Account.Currency)
	lines := []string{header}
	if view.Description != "" {
		lines = append(lines, styleDim.Italic(true).Width(width).Render(view.Description))
	}
	lines = append(lines, "", amount+strings.Repeat(" ", max(2, width-lipgloss.Width(amount)-lipgloss.Width(currency)))+currency, "")

	row := func(label, text string) string { return styleDim.Width(10).Render(label) + text }
	categoryColor, _ := category.ColorByKey(view.Category.Color)
	lines = append(lines,
		row(place, swatch(view.Account.Color, "●")+" "+styleBold.Render(view.Account.Code)+"  "+view.Account.Name),
	)
	if view.Other != nil {
		label := "From"
		if view.Outgoing() {
			label = "To"
		}
		other := swatch(view.Other.Color, "●") + " " + styleBold.Render(view.Other.Code) + "  " + view.Other.Name
		if view.PairID != 0 {
			other += styleDim.Render("  #" + strconv.FormatUint(uint64(view.PairID), 10))
		}
		lines = append(lines, row(label, other))
	}
	if period != "" {
		lines = append(lines, row("Bill", period))
	}
	lines = append(lines,
		row("Category", paint(categoryColor.Hex, "●")+" "+styleBold.Render(view.Category.Code)+"  "+view.Category.Name),
		row("Date", date),
	)
	if view.Virtual {
		lines = append(lines, row("Virtual", "yes"))
	}
	if view.Status != "" {
		lines = append(lines, row("Status", view.Status))
	}
	if part != "" {
		lines = append(lines, row("Part", part))
	}
	if view.Tags != "" {
		lines = append(lines, row("Tags", lipgloss.NewStyle().Foreground(tint).Render(view.Tags)))
	}
	if view.Goal != nil {
		lines = append(lines, row("Goal", paint(colorIncome, "◎")+" "+styleBold.Render(view.Goal.Code)+"  "+view.Goal.Title))
	}
	if view.Schedule != nil {
		lines = append(lines, row("Schedule", styleBold.Render(view.Schedule.Code)+"  "+view.Schedule.Title))
	}
	lines = append(lines,
		"",
		styleDim.Render(strings.Repeat("─", width)),
		styleDim.Render("Created  "+created),
		styleDim.Render("Updated  "+updated),
	)
	return styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"
}
