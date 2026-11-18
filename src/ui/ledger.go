package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"finux/src/category"
	"finux/src/code"
	"finux/src/money"
	"finux/src/transaction"
)

const (
	// ledgerWidth is the widest the ledger gets.
	ledgerWidth = 96
	// ledgerRows is the most lines of days and transactions it shows.
	ledgerRows = 20
	// ledgerCurrencies is the most currencies the totals show.
	ledgerCurrencies = 3
)

type ledgerKeys struct {
	Up, Down, NextPage, PrevPage, Home, End, Select, Cancel key.Binding
}

var ledgerKeyMap = ledgerKeys{
	Up:       key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓", "move")),
	Down:     key.NewBinding(key.WithKeys("down")),
	NextPage: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgup/pgdn", "page")),
	PrevPage: key.NewBinding(key.WithKeys("pgup")),
	Home:     key.NewBinding(key.WithKeys("home"), key.WithHelp("home/end", "first/last")),
	End:      key.NewBinding(key.WithKeys("end")),
	Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
	Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
}

// Ledger is the interactive list of transactions. It is not the
// selector of the other modules: it reads like a bank statement. The
// transactions are in one line each under a heading for their day, the
// amounts are in one column, and the totals of the list are at the top.
// The search input reads the filters of transaction.ParseSearch.
type Ledger struct {
	title  string
	views  []transaction.View
	search textinput.Model
	// shown holds the indexes, in views, of the transactions the search
	// selects. cursor is a position in shown.
	shown  []int
	cursor int
	// top is the first line of the days and transactions on the screen.
	top           int
	width, height int
	// totalLines is the number of lines the totals have without a
	// search. The screen keeps them so it does not jump.
	totalLines int
	help       help.Model
	selected   int
	cancelled  bool
}

// NewLedger builds the list of transactions. views must be in the
// order to show, the newest date first.
func NewLedger(title string, views []transaction.View) Ledger {
	search := textinput.New()
	search.Prompt = "Search: "
	search.Placeholder = "title, #tag, @ACCOUNT, cat:CODE, 2026-11"
	search.CharLimit = 120
	search.SetWidth(60)
	search.Focus()
	l := Ledger{title: title, views: views, search: search, width: ledgerWidth, height: ledgerRows, help: help.New(), selected: -1}
	l.filter()
	l.totalLines = len(l.totals())
	return l
}

// filter shows the transactions the search text selects.
func (l *Ledger) filter() {
	query := transaction.ParseSearch(l.search.Value())
	l.shown = l.shown[:0:0]
	for index, view := range l.views {
		if query.Match(view) {
			l.shown = append(l.shown, index)
		}
	}
	l.cursor, l.top = 0, 0
}

// Init starts the cursor blink of the search input.
func (l Ledger) Init() tea.Cmd { return textinput.Blink }

// Query returns the search text.
func (l Ledger) Query() string { return l.search.Value() }

// Selected returns the index, in the views given to NewLedger, of the
// transaction the user picked.
func (l Ledger) Selected() (int, bool) { return l.selected, l.selected >= 0 }

// Cancelled reports whether the user left without a selection.
func (l Ledger) Cancelled() bool { return l.cancelled }

// The values of ledgerLine.row for a line that is not a transaction.
const (
	lineHeading = -1
	lineSpace   = -2
)

// ledgerLine is one line of the list: the transaction at the position
// row in shown, the heading of a day, or the empty line between two days.
type ledgerLine struct {
	day string
	row int
}

// lines returns the headings and the transactions in display order,
// with an empty line between two days.
func (l Ledger) lines() []ledgerLine {
	var lines []ledgerLine
	day := ""
	for row, index := range l.shown {
		if date := l.views[index].Date; date != day {
			if day != "" {
				lines = append(lines, ledgerLine{row: lineSpace})
			}
			day = date
			lines = append(lines, ledgerLine{day: day, row: lineHeading})
		}
		lines = append(lines, ledgerLine{day: day, row: row})
	}
	return lines
}

// window returns the lines on the screen. It always starts with the
// heading of a day: when the list is scrolled into a day, the heading of
// that day stays at the top. It never ends with a heading that has no
// transaction under it.
func (l Ledger) window(lines []ledgerLine) []ledgerLine {
	visible := lines[min(l.top, len(lines)):min(l.top+l.height, len(lines))]
	for len(visible) > 0 && visible[0].row == lineSpace {
		visible = visible[1:]
	}
	if len(visible) > 0 && visible[0].row >= 0 {
		visible = append([]ledgerLine{{day: visible[0].day, row: lineHeading}}, visible...)
		visible = visible[:min(len(visible), l.height)]
	}
	for len(visible) > 1 && visible[len(visible)-1].row < 0 {
		visible = visible[:len(visible)-1]
	}
	return visible
}

// move puts the cursor on a row and scrolls so the row, and the heading
// of its day when it is the line above, are on the screen.
func (l *Ledger) move(row int) {
	if len(l.shown) == 0 {
		return
	}
	l.cursor = min(max(row, 0), len(l.shown)-1)
	lines := l.lines()
	at := 0
	for index, line := range lines {
		if line.row == l.cursor {
			at = index
		}
	}
	first := at
	if at > 0 && lines[at-1].row == lineHeading {
		first = at - 1
	}
	if first < l.top {
		l.top = first
	}
	if at >= l.top+l.height {
		l.top = at - l.height + 1
	}
	l.top = max(0, min(l.top, len(lines)-l.height))
	// The heading that stays at the top can push the row off the screen.
	onScreen := func() bool {
		for _, line := range l.window(lines) {
			if line.row == l.cursor {
				return true
			}
		}
		return false
	}
	for !onScreen() && l.top < at {
		l.top++
	}
}

// Update handles keys and window size changes.
func (l Ledger) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = min(msg.Width, ledgerWidth)
		// The lines around the list: title, search, totals, status, help
		// and the empty lines between them.
		chrome := 7 + l.totalLines
		l.height = min(ledgerRows, max(4, msg.Height-chrome))
		l.move(l.cursor)
		return l, nil
	case tea.KeyPressMsg:
		switch {
		case msg.String() == "ctrl+c":
			l.cancelled = true
			return l, tea.Quit
		case key.Matches(msg, ledgerKeyMap.Cancel):
			if l.search.Value() == "" {
				l.cancelled = true
				return l, tea.Quit
			}
			l.search.SetValue("")
			l.filter()
			return l, nil
		case key.Matches(msg, ledgerKeyMap.Select):
			if len(l.shown) > 0 {
				l.selected = l.shown[l.cursor]
				return l, tea.Quit
			}
			return l, nil
		case key.Matches(msg, ledgerKeyMap.Up):
			l.move(l.cursor - 1)
			return l, nil
		case key.Matches(msg, ledgerKeyMap.Down):
			l.move(l.cursor + 1)
			return l, nil
		case key.Matches(msg, ledgerKeyMap.PrevPage):
			l.move(l.cursor - l.height)
			return l, nil
		case key.Matches(msg, ledgerKeyMap.NextPage):
			l.move(l.cursor + l.height)
			return l, nil
		case key.Matches(msg, ledgerKeyMap.Home):
			l.move(0)
			return l, nil
		case key.Matches(msg, ledgerKeyMap.End):
			l.move(len(l.shown) - 1)
			return l, nil
		}
	}
	before := l.search.Value()
	var cmd tea.Cmd
	l.search, cmd = l.search.Update(msg)
	if l.search.Value() != before {
		l.filter()
	}
	return l, cmd
}

// totals returns one line for each currency that has incomes or
// expenses in the list: what came in, what went out and the difference.
// The sides of a transfer are not in the totals: the money stays yours.
func (l Ledger) totals() []string {
	in, out := map[string]int64{}, map[string]int64{}
	for _, index := range l.shown {
		view := l.views[index]
		switch view.Type {
		case transaction.Income:
			in[view.Account.Currency] += view.Amount
		case transaction.Expense:
			out[view.Account.Currency] -= view.Amount
		}
	}
	var lines []string
	for _, currency := range money.Codes() {
		if in[currency] == 0 && out[currency] == 0 || len(lines) == ledgerCurrencies {
			continue
		}
		net := in[currency] - out[currency]
		lines = append(lines, "  "+styleBold.Render(currency)+
			styleDim.Render("   in ")+typeStyle(transaction.Income).Render(money.Format(in[currency], currency))+
			styleDim.Render("   out ")+typeStyle(transaction.Expense).Render(money.Format(out[currency], currency))+
			styleDim.Render("   net ")+styleBold.Render(money.Format(net, currency)))
	}
	return lines
}

// ledgerAccount is the account column of a transaction. A transfer
// shows its two accounts and the direction of the money.
func ledgerAccount(view transaction.View) string {
	switch {
	case view.Other != nil && view.Outgoing():
		return view.Account.Code + " → " + view.Other.Code
	case view.Other != nil:
		return view.Account.Code + " ← " + view.Other.Code
	}
	return view.Account.Code
}

// cut makes text fit in width characters.
func cut(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	if width < 1 {
		return ""
	}
	return string([]rune(text)[:width-1]) + "…"
}

// View renders the ledger. It is empty after it closes.
func (l Ledger) View() tea.View {
	if l.selected >= 0 || l.cancelled {
		return tea.NewView("")
	}
	// The account and amount columns are as wide as their widest text.
	accountWidth, amountWidth := 0, 0
	for _, index := range l.shown {
		view := l.views[index]
		accountWidth = max(accountWidth, lipgloss.Width(ledgerAccount(view)))
		amountWidth = max(amountWidth, lipgloss.Width(money.Format(view.Amount, view.Account.Currency)))
	}
	// marker, indent, sign, text, category, account and amount, with
	// gaps of 2.
	textWidth := max(8, l.width-6-2-(code.MaxLength+2)-2-accountWidth-2-amountWidth-2)

	lines := l.lines()
	var list []string
	for _, line := range l.window(lines) {
		if line.row == lineSpace {
			list = append(list, "")
			continue
		}
		if line.row == lineHeading {
			day := transaction.FormatDate(line.day)
			if date, err := time.Parse("2006-01-02", line.day); err == nil {
				day += "  " + date.Weekday().String()
			}
			rule := strings.Repeat("─", max(0, l.width-4-lipgloss.Width(day)-1))
			list = append(list, "  "+styleBold.Render(day)+" "+styleDim.Render(rule))
			continue
		}
		list = append(list, l.row(l.views[l.shown[line.row]], line.row == l.cursor, textWidth, accountWidth, amountWidth))
	}
	if len(l.shown) == 0 {
		list = append(list, styleDim.Render("  No transactions match the search"))
	}
	for len(list) < l.height {
		list = append(list, "")
	}

	shown, total := len(l.shown), len(l.views)
	status := fmt.Sprintf("%d of %d transactions", shown, total)
	if shown == total {
		status = fmt.Sprintf("%d transactions", total)
		if total == 1 {
			status = "1 transaction"
		}
	}
	bindings := []key.Binding{ledgerKeyMap.Up, ledgerKeyMap.Select, ledgerKeyMap.Cancel}
	if len(lines) > l.height {
		bindings = []key.Binding{ledgerKeyMap.Up, ledgerKeyMap.NextPage, ledgerKeyMap.Home, ledgerKeyMap.Select, ledgerKeyMap.Cancel}
	}

	out := []string{"  " + styleTitle.Render(l.title), "  " + l.search.View(), ""}
	totals := l.totals()
	for len(totals) < l.totalLines {
		totals = append(totals, "")
	}
	out = append(out, totals...)
	if l.totalLines > 0 {
		out = append(out, "")
	}
	out = append(out, list...)
	out = append(out, "", "  "+styleDim.Render(status), "  "+l.help.ShortHelpView(bindings))
	fit := lipgloss.NewStyle().MaxWidth(l.width)
	for index, line := range out {
		out[index] = fit.Render(line)
	}
	return tea.NewView(strings.Join(out, "\n") + "\n")
}

// row renders one transaction: a sign for its type, the title and the
// tags, the category, the account and the amount.
func (l Ledger) row(view transaction.View, current bool, textWidth, accountWidth, amountWidth int) string {
	// The signs have fixed colors, the same as the category dots: the
	// colors of a terminal theme can make red and green look the same.
	sign := styleDim.Render("↔")
	switch view.Type {
	case transaction.Expense:
		sign = paint(colorExpense, "▼")
	case transaction.Income:
		sign = paint(colorIncome, "▲")
	}
	// The transaction is further in than the heading of its day.
	marker, titleStyle := "    ", lipgloss.NewStyle()
	if current {
		marker, titleStyle = styleAccent.Render("▌")+"   ", styleTitle
	}
	title := cut(view.Title, textWidth)
	text := titleStyle.Render(title)
	// The tags use the room the title leaves.
	if room := textWidth - lipgloss.Width(title) - 2; view.Tags != "" && room > 3 {
		text += "  " + styleDim.Render(cut(view.Tags, room))
	}
	text += strings.Repeat(" ", max(0, textWidth-lipgloss.Width(text)))

	color, _ := category.ColorByKey(view.Category.Color)
	group := paint(color.Hex, "●") + " " + view.Category.Code + strings.Repeat(" ", max(0, code.MaxLength-len(view.Category.Code)))
	account := ledgerAccount(view)
	account += strings.Repeat(" ", max(0, accountWidth-lipgloss.Width(account)))
	amount := money.Format(view.Amount, view.Account.Currency)
	amount = strings.Repeat(" ", max(0, amountWidth-lipgloss.Width(amount))) + typeStyle(view.Type).Render(amount)
	if view.Type == transaction.Transfer {
		amount = styleDim.Render(amount)
	}
	return marker + sign + " " + text + "  " + group + "  " + styleDim.Render(account) + "  " + amount
}
