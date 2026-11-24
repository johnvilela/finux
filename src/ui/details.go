package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// DetailsAction is what the user chose on the details of a record.
type DetailsAction int

const (
	DetailsClose DetailsAction = iota
	DetailsEdit
	DetailsDelete
)

// Details shows the card of a record with the buttons that edit and
// delete it. Edit has the focus first: it is the safe action.
type Details struct {
	card     string
	onDelete bool
	action   DetailsAction
	done     bool
}

// NewDetails builds the screen for a rendered card.
func NewDetails(card string) Details { return Details{card: card} }

// Init does nothing; the screen has no timers.
func (d Details) Init() tea.Cmd { return nil }

// Action returns the button the user chose.
func (d Details) Action() DetailsAction { return d.action }

// Update handles a key.
func (d Details) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch press.String() {
	case "left", "right", "tab", "shift+tab", "h", "l":
		d.onDelete = !d.onDelete
		return d, nil
	case "enter":
		d.action = DetailsEdit
		if d.onDelete {
			d.action = DetailsDelete
		}
	case "e", "E":
		d.action = DetailsEdit
	case "d", "D":
		d.action = DetailsDelete
	case "esc", "q", "ctrl+c":
		d.action = DetailsClose
	default:
		return d, nil
	}
	d.done = true
	return d, tea.Quit
}

// View renders the card and the buttons. After the screen closes only
// the card stays, and the form of an edit takes its place.
func (d Details) View() tea.View {
	if d.done {
		if d.action == DetailsEdit {
			return tea.NewView("")
		}
		return tea.NewView(d.card)
	}
	// Delete is dim until it has the focus, so that it does not call
	// for attention.
	button := lipgloss.NewStyle().Padding(0, 2)
	edit, remove := button, button.Foreground(colorDim)
	if d.onDelete {
		remove = button.Bold(true).Reverse(true).Foreground(colorError)
	} else {
		edit = button.Bold(true).Reverse(true).Foreground(colorAccent)
	}
	buttons := "  " + edit.Render("Edit") + "  " + remove.Render("Delete")
	help := "  " + styleDim.Render("←/→ choose • enter select • e edit • d delete • esc close")
	return tea.NewView(d.card + "\n" + buttons + "\n\n" + help + "\n")
}
