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

// colorWarning is the color of a bill that waits for its payment.
const colorWarning = "#F59E0B"

// CardEntries makes selector rows from credit cards. The value at the
// right is the available limit.
func CardEntries(cards []card.Card) []Entry {
	entries := make([]Entry, len(cards))
	for i, current := range cards {
		description := current.Description
		if current.Archived() {
			description = strings.TrimSpace("archived  " + description)
		}
		color, _ := account.ColorByKey(current.Color)
		entries[i] = Entry{
			Color: color.Hex, Code: current.Code, Name: current.Name, Description: description,
			Value: money.Format(current.Available(), current.Currency), Negative: current.Available() < 0,
		}
	}
	return entries
}

// CardTable renders the list of credit cards. Plain output has no
// colors and no borders, for pipes and files.
func CardTable(cards []card.Card, plain bool) string {
	if len(cards) == 0 {
		return "No credit cards yet. Run 'finux cc add'.\n"
	}
	headers := []string{"CODE", "NAME", "CLOSES", "DUE", "LIMIT", "USED", "AVAILABLE"}
	owned := false
	for _, current := range cards {
		owned = owned || !current.Mine()
	}
	// The owner column shows only when a card of another person is in
	// the list.
	if owned {
		headers = slices.Insert(headers, 2, "OWNER")
	}
	firstAmount, availableColumn := len(headers)-3, len(headers)-1
	rows := make([][]string, len(cards))
	for i, current := range cards {
		name := current.Name
		if current.Archived() {
			name += " (archived)"
		}
		label := current.Code
		if !plain {
			label = swatch(current.Color, "●") + " " + label
		}
		rows[i] = []string{
			label, name, strconv.Itoa(current.ClosingDay), strconv.Itoa(current.DueDay),
			money.Format(current.Limit, current.Currency), money.Format(current.Used, current.Currency),
			money.Format(current.Available(), current.Currency),
		}
		if owned {
			rows[i] = slices.Insert(rows[i], 2, current.Owner)
		}
	}
	count := fmt.Sprintf("%d credit cards", len(cards))
	if len(cards) == 1 {
		count = "1 credit card"
	}
	if plain {
		return plainTable(headers, rows, availableColumn) + count + "\n"
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
			case cards[row].Archived():
				return style.Foreground(colorDim)
			case column == availableColumn && cards[row].Available() < 0:
				return style.Foreground(lipgloss.Color(colorExpense))
			case column == 2 || column == 3:
				return style.Foreground(colorDim)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// statusStyle returns the style of a bill status: red for late, amber
// for closed, green for paid and no color for open.
func statusStyle(status string) lipgloss.Style {
	switch status {
	case card.StatusLate:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorExpense))
	case card.StatusClosed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarning))
	case card.StatusPaid:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorIncome))
	}
	return lipgloss.NewStyle()
}

// BillTable renders the bills of a credit card on the date today.
// Plain output has no colors and no borders, for pipes and files.
func BillTable(owner card.Card, bills []card.Bill, today string, plain bool) string {
	if len(bills) == 0 {
		return "No bills yet for " + owner.Code + ".\n"
	}
	headers := []string{"BILL", "CLOSES", "DUE", "STATUS", "PAID", "TOTAL"}
	const statusColumn, paidColumn, totalColumn = 3, 4, 5
	rows := make([][]string, len(bills))
	statuses := make([]string, len(bills))
	for i, bill := range bills {
		statuses[i] = bill.StatusOn(today)
		paid := "-"
		if bill.IsPaid() {
			paid = money.Format(bill.Paid, owner.Currency)
		}
		rows[i] = []string{
			card.FormatPeriod(bill.Period), transaction.FormatDate(bill.ClosesOn), transaction.FormatDate(bill.DueOn),
			statuses[i], paid, money.Format(bill.Total(), owner.Currency),
		}
	}
	count := fmt.Sprintf("%d bills", len(bills))
	if len(bills) == 1 {
		count = "1 bill"
	}
	if plain {
		return plainTable(headers, rows, totalColumn) + count + "\n"
	}
	rendered := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styleDim).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if column >= paidColumn {
				style = style.Align(lipgloss.Right)
			}
			switch {
			case row == table.HeaderRow:
				return style.Foreground(colorAccent).Bold(true)
			case column == statusColumn:
				return style.Inherit(statusStyle(statuses[row]))
			case column == 1 || column == 2 || column == paidColumn:
				return style.Foreground(colorDim)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+owner.Code+"  "+count) + "\n"
}

// CardDetails is what the details of a credit card show.
type CardDetails struct {
	Card card.Card
	// Bill is the open bill: the one that receives the transactions of
	// today. Transactions are its transactions, the newest first.
	Bill         card.Bill
	Transactions []transaction.View
	// Pending lists the bills that are closed or late.
	Pending []card.Bill
	// Today is the current date as YYYY-MM-DD.
	Today string
}

// pendingLine describes a bill that waits for its payment.
func pendingLine(owner card.Card, bill card.Bill, today string) (status, text string) {
	status = bill.StatusOn(today)
	return status, fmt.Sprintf("Bill %s is %s: %s, due %s", card.FormatPeriod(bill.Period), status,
		money.Format(bill.Total(), owner.Currency), transaction.FormatDate(bill.DueOn))
}

const carriedLabel = "Carried from the bill before"

// virtualLabel names the total of the transactions of a virtual card.
const virtualLabel = "Virtual card"

// offLimitLabel names the total of the transactions outside the limit
// of a card.
const offLimitLabel = "Off limit"

// CardDetailsView renders the details of one credit card and the
// transactions of its open bill. Plain output has no colors and no
// borders, for pipes and files.
func CardDetailsView(details CardDetails, plain bool) string {
	if !plain {
		return styledCardDetails(details)
	}
	current, bill := details.Card, details.Bill
	fields := [][2]string{
		{"ID", strconv.FormatUint(uint64(current.ID), 10)},
		{"Currency", current.Currency},
		{"Limit", money.Format(current.Limit, current.Currency)},
		{"Used", money.Format(current.Used, current.Currency)},
		{"Available", money.Format(current.Available(), current.Currency)},
		{"Closing", "day " + strconv.Itoa(current.ClosingDay)},
		{"Due", "day " + strconv.Itoa(current.DueDay)},
		{"Color", current.Color},
		{"Created", current.CreatedAt.Local().Format(timeLayout)},
		{"Updated", current.UpdatedAt.Local().Format(timeLayout)},
	}
	if current.OffLimit != 0 {
		fields = slices.Insert(fields, 5, [2]string{offLimitLabel, money.Format(current.OffLimit, current.Currency)})
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
		lines = append(lines, fmt.Sprintf("%-11s%s", field[0], field[1]))
	}
	lines = append(lines, "")
	for _, pending := range details.Pending {
		_, text := pendingLine(current, pending, details.Today)
		lines = append(lines, text)
	}
	if len(details.Pending) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, fmt.Sprintf("Open bill %s: closes %s, due %s", card.FormatPeriod(bill.Period),
		transaction.FormatDate(bill.ClosesOn), transaction.FormatDate(bill.DueOn)))
	out := strings.Join(lines, "\n") + "\n"
	if len(details.Transactions) == 0 {
		out += "No transactions in this bill.\n"
	} else {
		rows := make([][]string, len(details.Transactions))
		for i, view := range details.Transactions {
			rows[i] = []string{
				strconv.FormatUint(uint64(view.ID), 10), transaction.FormatDate(view.Date), marked(view.Title, view), view.Category.Code,
				money.Format(view.Amount, current.Currency),
			}
		}
		out += plainTable([]string{"ID", "DATE", "TITLE", "CATEGORY", "VALUE"}, rows, 4)
	}
	if virtual := virtualTotal(details.Transactions, current.Currency); virtual != "" {
		out += virtualLabel + "  " + virtual + "\n"
	}
	if bill.Carried != 0 {
		out += carriedLabel + "  " + money.Format(bill.Carried, current.Currency) + "\n"
	}
	if len(details.Transactions) > 0 || bill.Carried != 0 {
		out += "Total  " + money.Format(bill.Total(), current.Currency) + "\n"
	}
	return out
}

const (
	detailsWidth = 64
	barWidth     = 30
)

// limitBar shows the used part of the limit.
func limitBar(current card.Card, tint lipgloss.Style) string {
	filled := 0
	if current.Limit > 0 {
		filled = int(min(max(current.Used, 0), current.Limit) * barWidth / current.Limit)
	}
	if current.Used > 0 && filled == 0 {
		filled = 1
	}
	if current.Used > current.Limit {
		filled = barWidth
	}
	return tint.Render(strings.Repeat("█", filled)) + styleDim.Render(strings.Repeat("░", barWidth-filled))
}

// styledCardDetails shows a credit card in levels: the identity first,
// the available limit as the main value, the bills that wait for a
// payment, then the open bill with its transactions.
func styledCardDetails(details CardDetails) string {
	current, bill := details.Card, details.Bill
	tint := colorDim
	if color, ok := account.ColorByKey(current.Color); ok {
		tint = lipgloss.Color(color.Hex)
	}
	width := detailsWidth
	spread := func(left, right string) string {
		return left + strings.Repeat(" ", max(2, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
	}

	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(current.Code)
	header := badge + "  " + styleBold.Render(cut(current.Name, width-24))
	if current.Archived() {
		header += "  " + styleDim.Render("ARCHIVED")
	}
	lines := []string{spread(header, styleDim.Render("#"+strconv.FormatUint(uint64(current.ID), 10)))}
	if current.Description != "" {
		lines = append(lines, styleDim.Italic(true).Width(width).Render(current.Description))
	}

	amount := lipgloss.NewStyle().Bold(true)
	bar := lipgloss.NewStyle().Foreground(tint)
	if current.Available() < 0 {
		amount = amount.Foreground(lipgloss.Color(colorExpense))
		bar = bar.Foreground(lipgloss.Color(colorExpense))
	}
	lines = append(lines,
		"",
		styleDim.Render("AVAILABLE"),
		spread(amount.Render(money.Format(current.Available(), current.Currency)), styleDim.Render(current.Currency)),
		"",
		limitBar(current, bar)+"  "+money.Format(current.Used, current.Currency)+" used "+
			styleDim.Render("of "+money.Format(current.Limit, current.Currency)),
	)
	if current.OffLimit != 0 {
		lines = append(lines, styleDim.Render(money.Format(current.OffLimit, current.Currency)+" outside the limit"))
	}
	lines = append(lines,
		"",
		styleDim.Render(strings.Repeat("─", width)),
		styleDim.Render(fmt.Sprintf("Closes day %d  ·  Due day %d", current.ClosingDay, current.DueDay)),
		styleDim.Render("Created  "+current.CreatedAt.Local().Format(timeLayout)),
	)
	out := styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"

	// The lines below the box start under its text.
	const indent = "   "
	for _, pending := range details.Pending {
		status, text := pendingLine(current, pending, details.Today)
		out += indent + statusStyle(status).Render("● "+text) + "\n"
	}
	if len(details.Pending) > 0 {
		out += "\n"
	}
	out += indent + spread(styleTitle.Render("OPEN BILL "+card.FormatPeriod(bill.Period)),
		styleDim.Render("closes "+transaction.FormatDate(bill.ClosesOn)+"  ·  due "+transaction.FormatDate(bill.DueOn))) + "\n"
	out += indent + styleDim.Render(strings.Repeat("─", width)) + "\n"
	if len(details.Transactions) == 0 {
		out += indent + styleDim.Render("No transactions in this bill.") + "\n"
	}
	const dateWidth, groupWidth, amountWidth = 5, 7, 16
	textWidth := width - dateWidth - 2 - 2 - 2 - groupWidth - 2 - amountWidth
	for _, view := range details.Transactions {
		sign := paint(colorExpense, "▼")
		if view.Type == transaction.Income {
			sign = paint(colorIncome, "▲")
		}
		title := cut(view.Title, textWidth)
		text := title
		// The mark of a virtual card comes before the tags.
		if room := textWidth - lipgloss.Width(text) - 1; view.Virtual && room >= len("virtual") {
			text += " " + styleDim.Render("virtual")
		}
		if room := textWidth - lipgloss.Width(text) - 1; view.OffLimit && room >= len("off limit") {
			text += " " + styleDim.Render("off limit")
		}
		if room := textWidth - lipgloss.Width(text) - 2; view.Tags != "" && room > 3 {
			text += "  " + styleDim.Render(cut(view.Tags, room))
		}
		text += strings.Repeat(" ", max(0, textWidth-lipgloss.Width(text)))
		color, _ := category.ColorByKey(view.Category.Color)
		group := paint(color.Hex, "●") + " " + view.Category.Code
		group += strings.Repeat(" ", max(0, groupWidth-lipgloss.Width(group)))
		value := money.Format(view.Amount, current.Currency)
		value = strings.Repeat(" ", max(0, amountWidth-lipgloss.Width(value))) + typeStyle(view.Type).Render(value)
		date := transaction.FormatDate(view.Date)
		out += indent + styleDim.Render(date[:min(dateWidth, len(date))]) + "  " + sign + " " + text + "  " + group + "  " + value + "\n"
	}
	if virtual := virtualTotal(details.Transactions, current.Currency); virtual != "" {
		out += indent + spread(styleDim.Render(virtualLabel), styleDim.Render(virtual)) + "\n"
	}
	if bill.Carried != 0 {
		out += indent + spread(styleDim.Render(carriedLabel), money.Format(bill.Carried, current.Currency)) + "\n"
	}
	if len(details.Transactions) > 0 || bill.Carried != 0 {
		out += indent + styleDim.Render(strings.Repeat("─", width)) + "\n"
		out += indent + spread(styleBold.Render("Total"), styleBold.Render(money.Format(bill.Total(), current.Currency))) + "\n"
	}
	return out
}
