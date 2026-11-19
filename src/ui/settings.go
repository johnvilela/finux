package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// SettingRow is one setting on the settings screen.
type SettingRow struct {
	Key, Title, Description string
	// Values lists the values the setting can have. Value is the current one.
	Values []string
	Value  string
}

// Settings is the interactive screen that changes the settings of Finux.
type Settings struct {
	rows        []SettingRow
	focus       int
	saved, done bool
}

// NewSettings builds the settings screen.
func NewSettings(rows []SettingRow) Settings {
	return Settings{rows: slices.Clone(rows)}
}

// Init does nothing; the screen has no timers.
func (s Settings) Init() tea.Cmd { return nil }

// Saved reports whether the user saved the settings.
func (s Settings) Saved() bool { return s.saved }

// Values returns the value of each setting, by key.
func (s Settings) Values() map[string]string {
	values := map[string]string{}
	for _, row := range s.rows {
		values[row.Key] = row.Value
	}
	return values
}

// move changes the value of the setting with the focus by a number of
// positions. around makes it go from the last value to the first.
func (s *Settings) move(step int, around bool) {
	row := &s.rows[s.focus]
	index := slices.Index(row.Values, row.Value) + step
	if around {
		index = (index + len(row.Values)) % len(row.Values)
	}
	row.Value = row.Values[min(max(index, 0), len(row.Values)-1)]
}

// Update handles a key.
func (s Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok || len(s.rows) == 0 {
		return s, nil
	}
	switch press.String() {
	case "up", "shift+tab", "k":
		s.focus = max(s.focus-1, 0)
	case "down", "tab", "j":
		s.focus = min(s.focus+1, len(s.rows)-1)
	case "left", "h":
		s.move(-1, false)
	case "right", "l":
		s.move(1, false)
	case "space", " ":
		s.move(1, true)
	case "enter", "ctrl+s":
		s.saved, s.done = true, true
		return s, tea.Quit
	case "esc", "q", "ctrl+c":
		s.done = true
		return s, tea.Quit
	}
	return s, nil
}

// View renders the screen. It is empty after the screen closes.
func (s Settings) View() tea.View {
	if s.done {
		return tea.NewView("")
	}
	const labelWidth = 22
	lines := []string{styleTitle.Render("Settings"), ""}
	for i, row := range s.rows {
		label := styleDim.Width(labelWidth).Render(row.Title)
		if i == s.focus {
			label = styleTitle.Width(labelWidth).Render(row.Title)
		}
		choices := make([]string, len(row.Values))
		for j, value := range row.Values {
			style := lipgloss.NewStyle().Padding(0, 1).Foreground(colorDim)
			switch {
			case value == row.Value && i == s.focus:
				style = style.Bold(true).Reverse(true).Foreground(colorAccent)
			case value == row.Value:
				style = style.Bold(true).UnsetForeground()
			}
			choices[j] = style.Render(value)
		}
		lines = append(lines, label+strings.Join(choices, " "))
	}
	lines = append(lines,
		"",
		styleDim.Width(formWidth-6).Render(s.rows[s.focus].Description),
		"",
		styleDim.Render("↑/↓ move • ←/→ or space change • enter save • esc cancel"),
	)
	return tea.NewView(styleBox.Width(formWidth).Render(strings.Join(lines, "\n")) + "\n")
}
