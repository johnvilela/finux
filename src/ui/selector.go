package ui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"finux/src/account"
	"finux/src/category"
	"finux/src/code"
	"finux/src/money"
)

const (
	selectorWidth = 64
	// selectorPage is the number of records the selector shows at a time.
	selectorPage = 5
)

// Entry is one row of a Selector.
type Entry struct {
	// Color is the hex color of the dot before the code, or "".
	Color string
	// Code, Name and Description identify the record. The search looks at
	// the code and the name.
	Code, Name, Description string
	// Value is the text at the right edge, for example a balance.
	Value string
	// Negative paints Value as a negative amount.
	Negative bool
}

func (e Entry) FilterValue() string { return e.Code + " " + e.Name }

// AccountEntries makes selector rows from accounts.
func AccountEntries(accounts []account.Account) []Entry {
	entries := make([]Entry, len(accounts))
	for i, current := range accounts {
		description := current.Description
		if current.Archived() {
			description = strings.TrimSpace("archived  " + description)
		}
		color, _ := account.ColorByKey(current.Color)
		entries[i] = Entry{
			Color: color.Hex, Code: current.Code, Name: current.Name, Description: description,
			Value: money.Format(current.Balance(), current.Currency), Negative: current.Balance() < 0,
		}
	}
	return entries
}

// CategoryEntries makes selector rows from categories.
func CategoryEntries(categories []category.Category) []Entry {
	entries := make([]Entry, len(categories))
	for i, current := range categories {
		color, _ := category.ColorByKey(current.Color)
		entries[i] = Entry{Color: color.Hex, Code: current.Code, Name: current.Name, Description: current.Description, Value: color.Meaning}
	}
	return entries
}

// indexedEntry is an Entry with its position in the list the caller gave.
type indexedEntry struct {
	Entry
	index int
}

type entryDelegate struct{}

func (entryDelegate) Height() int                         { return 2 }
func (entryDelegate) Spacing() int                        { return 1 }
func (entryDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (entryDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(indexedEntry)
	if !ok {
		return
	}
	marker, label, name := "  ", styleBold.Render(entry.Code), entry.Name
	if index == m.Index() {
		marker, label, name = styleAccent.Render("▌ "), styleTitle.Render(entry.Code), styleAccent.Render(entry.Name)
	}
	dot := "●"
	if entry.Color != "" {
		dot = lipgloss.NewStyle().Foreground(lipgloss.Color(entry.Color)).Render(dot)
	}
	value := styleDim.Render(entry.Value)
	if entry.Negative {
		value = styleError.Render(entry.Value)
	}
	left := marker + dot + " " + label + strings.Repeat(" ", max(0, code.MaxLength-len(entry.Code))) + "  "
	// A long name gives its place to the value.
	if room := m.Width() - 4 - lipgloss.Width(left) - lipgloss.Width(value); lipgloss.Width(entry.Name) > room && room > 1 {
		short := []rune(entry.Name)
		name = strings.Replace(name, entry.Name, string(short[:min(len(short), room-1)])+"…", 1)
	}
	left += name
	gap := max(2, m.Width()-2-lipgloss.Width(left)-lipgloss.Width(value))
	description := entry.Description
	if description == "" {
		description = "no description"
	}
	fmt.Fprintf(w, "%s%s%s\n    %s", left, strings.Repeat(" ", gap), value, styleDim.Render(description))
}

type selectorKeys struct {
	Up, Down, NextPage, PrevPage, Select, Cancel key.Binding
}

var selectorKeyMap = selectorKeys{
	Up:       key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓", "move")),
	Down:     key.NewBinding(key.WithKeys("down")),
	NextPage: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgup/pgdn", "page")),
	PrevPage: key.NewBinding(key.WithKeys("pgup")),
	Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

// Selector is the interactive screen that picks one record. It shows a
// few records at a time and a search input that finds the others.
type Selector struct {
	title            string
	singular, plural string
	entries          []Entry
	search           textinput.Model
	list             list.Model
	help             help.Model
	selected         int
	cancelled        bool
}

// NewSelector builds a selector. singular and plural name the records,
// for example "account" and "accounts".
func NewSelector(title, singular, plural string, entries []Entry) Selector {
	search := textinput.New()
	search.Prompt = "Search: "
	search.Placeholder = "name or code"
	search.CharLimit = 60
	search.SetWidth(40)
	search.Focus()

	// The list only shows and pages the accounts. The selector does the
	// search, so the list filter and its letter shortcuts are off.
	l := list.New(nil, entryDelegate{}, selectorWidth, min(len(entries), selectorPage)*3+1)
	l.KeyMap = list.KeyMap{
		CursorUp:   selectorKeyMap.Up,
		CursorDown: selectorKeyMap.Down,
		NextPage:   selectorKeyMap.NextPage,
		PrevPage:   selectorKeyMap.PrevPage,
	}
	l.SetFilteringEnabled(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	s := Selector{title: title, singular: singular, plural: plural, entries: entries, search: search, list: l, help: help.New(), selected: -1}
	s.filter()
	return s
}

// filter shows the records whose code or name contains the search text.
func (s *Selector) filter() tea.Cmd {
	query := strings.ToLower(strings.TrimSpace(s.search.Value()))
	var items []list.Item
	for index, entry := range s.entries {
		if strings.Contains(strings.ToLower(entry.FilterValue()), query) {
			items = append(items, indexedEntry{entry, index})
		}
	}
	cmd := s.list.SetItems(items)
	s.list.ResetSelected()
	return cmd
}

// Init starts the cursor blink of the search input.
func (s Selector) Init() tea.Cmd { return textinput.Blink }

// Query returns the search text.
func (s Selector) Query() string { return s.search.Value() }

// Selected returns the index, in the entries given to NewSelector, of
// the record the user picked.
func (s Selector) Selected() (int, bool) {
	return s.selected, s.selected >= 0
}

// Cancelled reports whether the user left without a selection.
func (s Selector) Cancelled() bool { return s.cancelled }

// Update handles keys and window size changes.
func (s Selector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.list.SetWidth(min(msg.Width, selectorWidth))
		return s, nil
	case tea.KeyPressMsg:
		switch {
		case msg.String() == "ctrl+c":
			s.cancelled = true
			return s, tea.Quit
		case key.Matches(msg, selectorKeyMap.Cancel):
			if s.search.Value() == "" {
				s.cancelled = true
				return s, tea.Quit
			}
			s.search.SetValue("")
			return s, s.filter()
		case key.Matches(msg, selectorKeyMap.Select):
			if entry, ok := s.list.SelectedItem().(indexedEntry); ok {
				s.selected = entry.index
				return s, tea.Quit
			}
			return s, nil
		case key.Matches(msg, selectorKeyMap.Up, selectorKeyMap.Down, selectorKeyMap.NextPage, selectorKeyMap.PrevPage):
			s.list, cmd = s.list.Update(msg)
			return s, cmd
		}
	}
	before := s.search.Value()
	s.search, cmd = s.search.Update(msg)
	if s.search.Value() != before {
		return s, tea.Batch(cmd, s.filter())
	}
	return s, cmd
}

// View renders the selector. It is empty after the selector closes.
func (s Selector) View() tea.View {
	if s.selected >= 0 || s.cancelled {
		return tea.NewView("")
	}
	shown, total := len(s.list.Items()), len(s.entries)
	status := fmt.Sprintf("%d of %d %s", shown, total, s.plural)
	if shown == total {
		status = fmt.Sprintf("%d %s", total, s.plural)
		if total == 1 {
			status = "1 " + s.singular
		}
	}
	records := s.list.View()
	if shown == 0 {
		// Keep the height of the list so the screen does not jump.
		records = lipgloss.NewStyle().Height(s.list.Height()).Render(styleDim.Render("  No " + s.plural + " match the search"))
	}
	bindings := []key.Binding{selectorKeyMap.Up, selectorKeyMap.Select, selectorKeyMap.Cancel}
	if total > selectorPage {
		bindings = []key.Binding{selectorKeyMap.Up, selectorKeyMap.NextPage, selectorKeyMap.Select, selectorKeyMap.Cancel}
	}
	return tea.NewView(strings.Join([]string{
		"  " + styleTitle.Render(s.title),
		"  " + s.search.View(),
		"",
		records,
		"  " + styleDim.Render(status),
		"  " + s.help.ShortHelpView(bindings),
	}, "\n") + "\n")
}
