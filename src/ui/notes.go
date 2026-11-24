package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/goal"
	"finux/src/note"
	"finux/src/transaction"
)

// NoteView is a note with the records it links to.
type NoteView struct {
	note.Note
	// Goal and Budget are nil when the note has none.
	Goal   *goal.Goal
	Budget *budget.Budget
	// Place is the account of the note. For a note of a credit card it
	// holds the name, the code and the color of the card, and OnCard is
	// true. It is nil when the note has none.
	Place  *account.Account
	OnCard bool
}

// links returns the codes of the records of the note, or "-".
func (v NoteView) links() string {
	var codes []string
	if v.Goal != nil {
		codes = append(codes, v.Goal.Code)
	}
	if v.Budget != nil {
		codes = append(codes, v.Budget.Code)
	}
	if v.Place != nil {
		codes = append(codes, v.Place.Code)
	}
	if len(codes) == 0 {
		return "-"
	}
	return strings.Join(codes, " ")
}

// noteRemind returns the reminder date of a note for a table, or "-".
func noteRemind(current note.Note) string {
	if current.RemindOn == "" {
		return "-"
	}
	return transaction.FormatDate(current.RemindOn)
}

// noteColor returns the color of the status of a note.
func noteColor(status string) string {
	switch status {
	case note.StatusDue:
		return colorWarning
	case note.StatusDone:
		return colorIncome
	}
	return colorNeutral
}

func dash(text string) string {
	if text == "" {
		return "-"
	}
	return text
}

func noteCount(count int) string {
	if count == 1 {
		return "1 note"
	}
	return fmt.Sprintf("%d notes", count)
}

// NoteEntries makes selector rows from notes.
func NoteEntries(views []NoteView, today string) []Entry {
	entries := make([]Entry, len(views))
	for i, view := range views {
		status := view.Status(today)
		description := status
		if view.RemindOn != "" {
			description += "  " + transaction.FormatDate(view.RemindOn)
		}
		if view.Text != "" {
			description += "  " + view.Text
		}
		entries[i] = Entry{
			Color: noteColor(status), Code: strconv.FormatUint(uint64(view.ID), 10), Name: view.Title,
			Description: description, Value: view.links(),
		}
	}
	return entries
}

// NoteTable renders notes as a table. Plain output has no colors and no
// borders, for pipes and files. filtered tells that a filter chose the
// notes, for the text of an empty list.
func NoteTable(views []NoteView, today string, filtered, plain bool) string {
	if len(views) == 0 {
		if filtered {
			return "No notes match the filter.\n"
		}
		return "No notes yet. Run 'finux nt add'.\n"
	}
	headers := []string{"ID", "TITLE", "REMIND", "STATUS", "LINKS", "TAGS"}
	const statusColumn = 3
	rows := make([][]string, len(views))
	statuses := make([]string, len(views))
	for i, view := range views {
		statuses[i] = view.Status(today)
		rows[i] = []string{strconv.FormatUint(uint64(view.ID), 10), view.Title, noteRemind(view.Note), statuses[i], view.links(), dash(view.Tags)}
	}
	count := noteCount(len(views))
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
			case statuses[row] == note.StatusDone:
				return style.Foreground(colorDim)
			case column == statusColumn:
				return style.Foreground(lipgloss.Color(noteColor(statuses[row])))
			case column != 1:
				return style.Foreground(colorDim)
			}
			return style
		}).String()
	return rendered + "\n" + styleDim.Render(" "+count) + "\n"
}

// NoteCard renders the details of one note. Plain output has no colors
// and no borders, for pipes and files.
func NoteCard(view NoteView, today string, plain bool) string {
	id := strconv.FormatUint(uint64(view.ID), 10)
	status := view.Status(today)
	place := "Account"
	if view.OnCard {
		place = "Card"
	}
	var fields [][2]string
	if view.RemindOn != "" {
		fields = append(fields, [2]string{"Remind", transaction.FormatDate(view.RemindOn)})
	}
	if view.Tags != "" {
		fields = append(fields, [2]string{"Tags", view.Tags})
	}
	if view.Goal != nil {
		fields = append(fields, [2]string{"Goal", view.Goal.Code + "  " + view.Goal.Title})
	}
	if view.Budget != nil {
		fields = append(fields, [2]string{"Budget", view.Budget.Code + "  " + view.Budget.Title})
	}
	if view.Place != nil {
		fields = append(fields, [2]string{place, view.Place.Code + "  " + view.Place.Name})
	}
	created, updated := view.CreatedAt.Local().Format(timeLayout), view.UpdatedAt.Local().Format(timeLayout)
	if plain {
		lines := []string{view.Title}
		if view.Text != "" {
			lines = append(lines, view.Text)
		}
		lines = append(lines, "")
		all := append([][2]string{{"ID", id}, {"Status", status}}, fields...)
		all = append(all, [2]string{"Source", view.Source})
		if view.DoneAt != nil {
			all = append(all, [2]string{"Done", view.DoneAt.Local().Format(timeLayout)})
		}
		all = append(all, [2]string{"Created", created}, [2]string{"Updated", updated})
		for _, field := range all {
			lines = append(lines, fmt.Sprintf("%-10s%s", field[0], field[1]))
		}
		return strings.Join(lines, "\n") + "\n"
	}

	tint := lipgloss.Color(noteColor(status))
	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Foreground(lipgloss.Color("#000000")).Background(tint).Render(strings.ToUpper(status))
	number := styleDim.Render("#" + id)
	width := min(cardMaxWidth, max(cardMinWidth, lipgloss.Width(badge)+2+lipgloss.Width(view.Title)+2+lipgloss.Width(number)))
	header := badge + "  " + styleBold.Render(cut(view.Title, width-lipgloss.Width(badge)-4-lipgloss.Width(number)))
	header += strings.Repeat(" ", max(2, width-lipgloss.Width(header)-lipgloss.Width(number))) + number
	lines := []string{header}
	if view.Text != "" {
		lines = append(lines, "", lipgloss.NewStyle().Width(width).Render(view.Text))
	}
	if len(fields) > 0 {
		lines = append(lines, "")
	}
	for _, field := range fields {
		text := field[1]
		if field[0] == "Tags" {
			text = lipgloss.NewStyle().Foreground(tint).Render(text)
		}
		lines = append(lines, styleDim.Width(10).Render(field[0])+text)
	}
	lines = append(lines, "", styleDim.Render(strings.Repeat("─", width)))
	if view.DoneAt != nil {
		lines = append(lines, styleDim.Render("Done     "+view.DoneAt.Local().Format(timeLayout)))
	}
	lines = append(lines,
		styleDim.Render("Source   "+view.Source),
		styleDim.Render("Created  "+created),
		styleDim.Render("Updated  "+updated),
	)
	return styleBox.BorderForeground(tint).Width(width+6).Render(strings.Join(lines, "\n")) + "\n"
}

// NoteSection renders the notes of a record, for the end of its
// details. It is "" when the record has none.
func NoteSection(notes []note.Note, today string, plain bool) string {
	if len(notes) == 0 {
		return ""
	}
	rows := make([][]string, len(notes))
	for i, current := range notes {
		rows[i] = []string{strconv.FormatUint(uint64(current.ID), 10) + " ", current.Title, noteRemind(current), current.Status(today)}
	}
	if plain {
		return "\nNotes\n" + plainTable(rows[0], rows[1:], -1)
	}
	const indent = "   "
	out := indent + styleTitle.Render("NOTES") + "\n" + indent + styleDim.Render(strings.Repeat("─", detailsWidth)) + "\n"
	for i, current := range notes {
		status := current.Status(today)
		left := styleDim.Render("#"+strings.TrimSpace(rows[i][0])) + "  " + cut(current.Title, detailsWidth-30)
		right := styleDim.Render(noteRemind(current)) + "  " + paint(noteColor(status), status)
		out += indent + left + strings.Repeat(" ", max(2, detailsWidth-lipgloss.Width(left)-lipgloss.Width(right))) + right + "\n"
	}
	return out
}
