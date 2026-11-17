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
	"finux/src/money"
)

const (
	selectorWidth = 64
	// selectorPage is the number of accounts the selector shows at a time.
	selectorPage = 5
)

type accountItem struct {
	account account.Account
}

func (i accountItem) FilterValue() string { return i.account.Code + " " + i.account.Name }

type accountDelegate struct{}

func (accountDelegate) Height() int                         { return 2 }
func (accountDelegate) Spacing() int                        { return 1 }
func (accountDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (accountDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(accountItem)
	if !ok {
		return
	}
	current := entry.account
	marker, code, name := "  ", styleBold.Render(current.Code), current.Name
	if index == m.Index() {
		marker, code, name = styleAccent.Render("▌ "), styleTitle.Render(current.Code), styleAccent.Render(current.Name)
	}
	left := marker + swatch(current.Color, "●") + " " + code + strings.Repeat(" ", account.MaxCodeLength-len(current.Code)) + "  " + name
	balance := money.Format(current.Balance(), current.Currency)
	if current.Balance() < 0 {
		balance = styleError.Render(balance)
	}
	gap := max(2, m.Width()-2-lipgloss.Width(left)-lipgloss.Width(balance))
	description := current.Description
	if current.Archived() {
		description = strings.TrimSpace("archived  " + description)
	}
	if description == "" {
		description = "no description"
	}
	fmt.Fprintf(w, "%s%s%s\n    %s", left, strings.Repeat(" ", gap), balance, styleDim.Render(description))
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

// Selector is the interactive screen that picks one account. It shows a
// few accounts at a time and a search input that finds the others.
type Selector struct {
	title     string
	accounts  []account.Account
	search    textinput.Model
	list      list.Model
	help      help.Model
	selected  *account.Account
	cancelled bool
}

// NewSelector builds a selector for accounts.
func NewSelector(title string, accounts []account.Account) Selector {
	search := textinput.New()
	search.Prompt = "Search: "
	search.Placeholder = "name or code"
	search.CharLimit = 60
	search.SetWidth(40)
	search.Focus()

	// The list only shows and pages the accounts. The selector does the
	// search, so the list filter and its letter shortcuts are off.
	l := list.New(nil, accountDelegate{}, selectorWidth, min(len(accounts), selectorPage)*3+1)
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

	s := Selector{title: title, accounts: accounts, search: search, list: l, help: help.New()}
	s.filter()
	return s
}

// filter shows the accounts whose code or name contains the search text.
func (s *Selector) filter() tea.Cmd {
	query := strings.ToLower(strings.TrimSpace(s.search.Value()))
	var items []list.Item
	for _, current := range s.accounts {
		if strings.Contains(strings.ToLower(accountItem{current}.FilterValue()), query) {
			items = append(items, accountItem{current})
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

// Selected returns the account the user picked.
func (s Selector) Selected() (account.Account, bool) {
	if s.selected == nil {
		return account.Account{}, false
	}
	return *s.selected, true
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
			if entry, ok := s.list.SelectedItem().(accountItem); ok {
				s.selected = &entry.account
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
	if s.selected != nil || s.cancelled {
		return tea.NewView("")
	}
	shown, total := len(s.list.Items()), len(s.accounts)
	status := fmt.Sprintf("%d of %d accounts", shown, total)
	if shown == total {
		status = fmt.Sprintf("%d accounts", total)
		if total == 1 {
			status = "1 account"
		}
	}
	accounts := s.list.View()
	if shown == 0 {
		// Keep the height of the list so the screen does not jump.
		accounts = lipgloss.NewStyle().Height(s.list.Height()).Render(styleDim.Render("  No accounts match the search"))
	}
	bindings := []key.Binding{selectorKeyMap.Up, selectorKeyMap.Select, selectorKeyMap.Cancel}
	if total > selectorPage {
		bindings = []key.Binding{selectorKeyMap.Up, selectorKeyMap.NextPage, selectorKeyMap.Select, selectorKeyMap.Cancel}
	}
	return tea.NewView(strings.Join([]string{
		"  " + styleTitle.Render(s.title),
		"  " + s.search.View(),
		"",
		accounts,
		"  " + styleDim.Render(status),
		"  " + s.help.ShortHelpView(bindings),
	}, "\n") + "\n")
}
