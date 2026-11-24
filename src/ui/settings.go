package ui

import (
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// SettingRow is one setting on the settings screen.
type SettingRow struct {
	Key, Title, Description string
	// Values lists the values the setting can have. Value is the current one.
	// A setting with no list is a text setting: the user writes its value.
	Values []string
	Value  string
	// Secret hides the value of a text setting, for example a token.
	Secret bool
	// Check makes the value of a text setting correct, or tells why it is
	// not. The screen does not save a value that is not correct.
	Check func(text string) (string, error)
	// Options gives the usual values of a text setting, for the values on
	// the screen now. The first one is the default: the setting has it when
	// its value is empty.
	Options func(values map[string]string) []string

	// start is the value when the screen opened.
	start string
}

func (r SettingRow) text() bool { return len(r.Values) == 0 }

// hidden reports whether the row shows a saved secret, which the user
// cannot see.
func (r SettingRow) hidden() bool { return r.Secret && r.Value != "" && r.Value == r.start }

// fresh empties a saved secret before the user writes: new text replaces
// a value that the user cannot see.
func (r *SettingRow) fresh() {
	if r.hidden() {
		r.Value = ""
	}
}

// Settings is the interactive screen that changes the settings of Finux.
type Settings struct {
	rows        []SettingRow
	focus       int
	saved, done bool
	// problem tells why the screen did not save.
	problem string
	// width is the width of the box: the columns of the terminal.
	width int
}

// NewSettings builds the settings screen.
func NewSettings(rows []SettingRow) Settings {
	s := Settings{rows: slices.Clone(rows), width: formWidth}
	for i := range s.rows {
		s.rows[i].start = s.rows[i].Value
	}
	return s
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

// options gives the usual values of a text setting.
func (s Settings) options(row SettingRow) []string {
	if row.Options == nil {
		return nil
	}
	return row.Options(s.Values())
}

// move changes the value of the setting with the focus by a number of
// positions. around makes it go from the last value to the first.
func (s *Settings) move(step int, around bool) {
	row := &s.rows[s.focus]
	if row.text() {
		// The first option is the default, which is the empty value.
		options := s.options(*row)
		if len(options) == 0 {
			return
		}
		index := max(slices.Index(options, row.Value), 0) + step
		if index = min(max(index, 0), len(options)-1); index == 0 {
			row.Value = ""
		} else {
			row.Value = options[index]
		}
		return
	}
	index := slices.Index(row.Values, row.Value) + step
	if around {
		index = (index + len(row.Values)) % len(row.Values)
	}
	row.Value = row.Values[min(max(index, 0), len(row.Values)-1)]
}

// save makes the text values correct. It moves the focus to the first
// one that is not, and tells why.
func (s *Settings) save() bool {
	for i := range s.rows {
		row := &s.rows[i]
		if !row.text() || row.Check == nil {
			continue
		}
		value, err := row.Check(strings.TrimSpace(row.Value))
		if err != nil {
			s.focus, s.problem = i, row.Title+": "+err.Error()
			return false
		}
		row.Value = value
	}
	return true
}

// write handles a key in a text setting. It reports false for a key that
// does not change the text.
func (s *Settings) write(press tea.KeyPressMsg) bool {
	row := &s.rows[s.focus]
	switch key := press.String(); {
	case key == "backspace" && row.hidden():
		row.Value = ""
	case key == "backspace":
		_, size := utf8.DecodeLastRuneInString(row.Value)
		row.Value = row.Value[:len(row.Value)-size]
	case key == "ctrl+u":
		row.Value = ""
	case key == "left" || key == "right" || key == "up" || key == "down":
		return false
	case press.Text != "" && press.Mod&^tea.ModShift == 0:
		row.fresh()
		row.Value += press.Text
	default:
		return false
	}
	s.problem = ""
	return true
}

// Update handles a key or a paste.
func (s Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(s.rows) == 0 {
		return s, nil
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		if row := &s.rows[s.focus]; row.text() {
			row.fresh()
			row.Value += strings.TrimSpace(paste.Content)
			s.problem = ""
		}
		return s, nil
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		s.width = size.Width
		return s, nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	key := press.String()
	if s.rows[s.focus].text() {
		if s.write(press) {
			return s, nil
		}
		// A letter writes text here, so only these keys do other things.
		if !slices.Contains([]string{"up", "down", "left", "right", "tab", "shift+tab", "enter", "ctrl+s", "esc", "ctrl+c"}, key) {
			return s, nil
		}
	}
	switch key {
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
		if !s.save() {
			return s, nil
		}
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
		if row.text() {
			lines = append(lines, label+s.field(row, i == s.focus))
			continue
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
	keys := "↑/↓ move • ←/→ or space change • enter save • esc cancel"
	if row := s.rows[s.focus]; row.text() {
		keys = "↑/↓ move • type or paste to change • ctrl+u clear • enter save • esc cancel"
		if len(s.options(row)) > 1 {
			keys = "↑/↓ move • ←/→ pick • type or paste • ctrl+u clear • enter save • esc cancel"
		}
	}
	lines = append(lines, "", styleDim.Width(s.width-6).Render(s.rows[s.focus].Description))
	if s.problem != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(colorError).Width(s.width-6).Render(s.problem))
	}
	lines = append(lines, "", styleDim.Render(keys))
	return tea.NewView(styleBox.Width(s.width).Render(strings.Join(lines, "\n")) + "\n")
}

// field renders the value of a text setting.
func (s Settings) field(row SettingRow, focus bool) string {
	style := lipgloss.NewStyle().Padding(0, 1)
	if focus {
		style = style.Bold(true).Reverse(true).Foreground(colorAccent)
	}
	value, dim := row.Value, false
	switch options := s.options(row); {
	case row.Secret && value == "":
		value, dim = "(not set)", true
	case row.hidden():
		value = "(set)"
	case row.Secret:
		// Only the end of a new secret shows, so the user can check it.
		runes := []rune(value)
		shown := min(4, len(runes)/2)
		value = strings.Repeat("•", min(len(runes)-shown, 12)) + string(runes[len(runes)-shown:])
	case value == "" && len(options) > 0:
		value, dim = options[0]+" (default)", true
	case value == "":
		value, dim = "(empty)", true
	}
	if dim && !focus {
		style = style.Foreground(colorDim)
	}
	return style.Render(value)
}
