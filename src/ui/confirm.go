package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Confirm is a yes/no prompt. The safe answer has the focus first.
type Confirm struct {
	question  string
	label     string
	danger    bool
	onConfirm bool
	confirmed bool
	done      bool
}

// NewConfirm builds a prompt. label names the confirm button. danger
// paints it red, for actions that cannot be undone.
func NewConfirm(question, label string, danger bool) Confirm {
	return Confirm{question: question, label: label, danger: danger}
}

// Init does nothing; the prompt has no timers.
func (c Confirm) Init() tea.Cmd { return nil }

// Confirmed reports whether the user chose the confirm button.
func (c Confirm) Confirmed() bool { return c.confirmed }

// Update handles a key.
func (c Confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch press.String() {
	case "left", "right", "tab", "shift+tab", "h", "l":
		c.onConfirm = !c.onConfirm
		return c, nil
	case "enter":
		c.confirmed = c.onConfirm
	case "y", "Y":
		c.confirmed = true
	case "n", "N", "esc", "q", "ctrl+c":
		c.confirmed = false
	default:
		return c, nil
	}
	c.done = true
	return c, tea.Quit
}

// View renders the prompt. It is empty after the prompt closes.
func (c Confirm) View() tea.View {
	if c.done {
		return tea.NewView("")
	}
	button := lipgloss.NewStyle().Padding(0, 2)
	cancel, confirm := button.Foreground(colorDim), button.Foreground(colorDim)
	focused := colorAccent
	if c.danger {
		focused = colorError
	}
	if c.onConfirm {
		confirm = button.Bold(true).Reverse(true).Foreground(focused)
	} else {
		cancel = button.Bold(true).Reverse(true).Foreground(colorAccent)
	}
	buttons := cancel.Render("Cancel") + "  " + confirm.Render(c.label)
	help := styleDim.Render("←/→ choose • enter confirm • y yes • n no")
	return tea.NewView(styleBox.Render(styleBold.Render(c.question)+"\n\n"+buttons+"\n\n"+help) + "\n")
}
