package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"finux/src/account"
	"finux/src/money"
)

// Field identifies one input of the account form.
type Field int

// The fields of the account form, in display order.
const (
	FieldName Field = iota
	FieldCode
	FieldDescription
	FieldCurrency
	FieldBalance
	FieldColor
	fieldCount
)

var fieldLabels = [fieldCount]string{"Name", "Code", "Description", "Currency", "Opening balance", "Color"}

const (
	labelWidth   = 17
	paletteWidth = 4
	formWidth    = 72
)

// FormOptions configures an account form.
type FormOptions struct {
	// Title is the heading of the form.
	Title string
	// Initial holds the starting values. In edit mode it is the account to change.
	Initial account.Account
	// Balance is the starting text of the opening balance field.
	Balance string
	// Edit shows only the fields that can change after creation.
	Edit bool
	// Preset lists fields that already have a final value. The form hides them.
	Preset []Field
	// Taken reports whether another account uses a code.
	Taken func(code string) bool
}

type formKeys struct {
	Next, Back, Save, Cancel, Left, Right, Up, Down key.Binding
}

var formKeyMap = formKeys{
	Next:   key.NewBinding(key.WithKeys("tab", "enter"), key.WithHelp("tab/enter", "next")),
	Back:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "back")),
	Save:   key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	Cancel: key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel")),
	Left:   key.NewBinding(key.WithKeys("left"), key.WithHelp("←/→", "choose")),
	Right:  key.NewBinding(key.WithKeys("right")),
	Up:     key.NewBinding(key.WithKeys("up"), key.WithHelp("arrows", "choose")),
	Down:   key.NewBinding(key.WithKeys("down")),
}

// Form is the interactive screen that creates or edits an account.
type Form struct {
	title       string
	initial     account.Account
	edit        bool
	fields      []Field
	focus       int
	inputs      [fieldCount]textinput.Model
	currency    int
	color       int
	codeTouched bool
	taken       func(string) bool
	err         string
	help        help.Model
	submitted   bool
	cancelled   bool
	width       int
}

// NewForm builds an account form.
func NewForm(options FormOptions) Form {
	f := Form{title: options.Title, initial: options.Initial, edit: options.Edit, taken: options.Taken, help: help.New(), width: formWidth}
	if f.taken == nil {
		f.taken = func(string) bool { return false }
	}
	if f.title == "" {
		f.title = "New account"
		if f.edit {
			f.title = "Edit account"
		}
	}
	hidden := map[Field]bool{}
	for _, field := range options.Preset {
		hidden[field] = true
	}
	if f.edit {
		hidden[FieldCurrency], hidden[FieldBalance] = true, true
	}
	for field := FieldName; field < fieldCount; field++ {
		if !hidden[field] {
			f.fields = append(f.fields, field)
		}
	}
	for i, code := range money.Codes() {
		if code == options.Initial.Currency {
			f.currency = i
		}
	}
	for i, color := range account.Palette {
		if color.Key == options.Initial.Color {
			f.color = i
		}
	}
	values := map[Field]string{
		FieldName:        options.Initial.Name,
		FieldCode:        options.Initial.Code,
		FieldDescription: options.Initial.Description,
		FieldBalance:     options.Balance,
	}
	placeholders := map[Field]string{FieldName: "Banco Inter", FieldCode: "INTER", FieldDescription: "optional"}
	for field, value := range values {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = placeholders[field]
		input.SetWidth(40)
		input.SetValue(value)
		f.inputs[field] = input
	}
	f.inputs[FieldCode].CharLimit = account.MaxCodeLength
	f.inputs[FieldName].CharLimit = 60
	f.inputs[FieldDescription].CharLimit = 120
	f.inputs[FieldBalance].CharLimit = 24
	f.codeTouched = options.Initial.Code != ""
	f.suggestCode()
	f.syncBalancePlaceholder()
	if len(f.fields) > 0 && f.isText(f.fields[0]) {
		f.inputs[f.fields[0]].Focus()
	}
	return f
}

// Init starts the cursor blink.
func (f Form) Init() tea.Cmd { return textinput.Blink }

// Focus returns the field that has the focus.
func (f Form) Focus() Field {
	if len(f.fields) == 0 {
		return fieldCount
	}
	return f.fields[f.focus]
}

// Value returns the current text of a field.
func (f Form) Value(field Field) string {
	switch field {
	case FieldCurrency:
		return money.Codes()[f.currency]
	case FieldColor:
		return account.Palette[f.color].Key
	case FieldName, FieldCode, FieldDescription, FieldBalance:
		return f.inputs[field].Value()
	}
	return ""
}

// Error returns the validation message of the focused field, or "".
func (f Form) Error() string { return f.err }

// Submitted reports whether the user saved the form.
func (f Form) Submitted() bool { return f.submitted }

// Cancelled reports whether the user left the form without saving.
func (f Form) Cancelled() bool { return f.cancelled }

// Account returns the initial account with the values of the form applied.
func (f Form) Account() account.Account {
	result := f.initial
	result.Name = strings.TrimSpace(f.Value(FieldName))
	result.Code = f.Value(FieldCode)
	result.Description = strings.TrimSpace(f.Value(FieldDescription))
	result.Color = f.Value(FieldColor)
	if !f.edit {
		result.Currency = f.Value(FieldCurrency)
		result.OpeningBalance = f.balance()
	}
	return result
}

func (f Form) balance() int64 {
	amount, err := money.Parse(f.Value(FieldBalance), f.Value(FieldCurrency))
	if err != nil {
		return 0
	}
	return amount
}

func (f Form) isText(field Field) bool {
	return field == FieldName || field == FieldCode || field == FieldDescription || field == FieldBalance
}

func (f *Form) suggestCode() {
	if f.edit || f.codeTouched {
		return
	}
	f.inputs[FieldCode].SetValue(account.SuggestCode(f.Value(FieldName), f.taken))
	f.inputs[FieldCode].CursorEnd()
}

func (f *Form) syncBalancePlaceholder() {
	f.inputs[FieldBalance].Placeholder = money.Placeholder(f.Value(FieldCurrency))
}

func (f *Form) setFocus(index int) tea.Cmd {
	f.inputs[f.Focus()].Blur()
	f.focus = index
	f.err = ""
	if f.isText(f.Focus()) {
		return f.inputs[f.Focus()].Focus()
	}
	return nil
}

func (f Form) validate(field Field) string {
	switch field {
	case FieldName:
		if strings.TrimSpace(f.Value(FieldName)) == "" {
			return "Name is required"
		}
	case FieldCode:
		code := f.Value(FieldCode)
		if err := account.ValidateCode(code); err != nil {
			return sentence(err.Error())
		}
		if f.taken(code) {
			return "Code " + code + " is already in use"
		}
	case FieldBalance:
		if strings.TrimSpace(f.Value(FieldBalance)) == "" {
			return ""
		}
		if _, err := money.Parse(f.Value(FieldBalance), f.Value(FieldCurrency)); err != nil {
			// Keep only the reason; the field already shows the value.
			message := err.Error()
			return sentence(message[strings.LastIndex(message, ": ")+1:])
		}
	}
	return ""
}

func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

func (f Form) submit() (tea.Model, tea.Cmd) {
	for index, field := range f.fields {
		if message := f.validate(field); message != "" {
			cmd := f.setFocus(index)
			f.err = message
			return f, cmd
		}
	}
	f.submitted = true
	return f, tea.Quit
}

// Update handles a key or a timer message.
func (f Form) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		f.width = min(size.Width, formWidth)
		return f, nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f.updateInput(msg)
	}
	switch {
	case key.Matches(press, formKeyMap.Cancel):
		f.cancelled = true
		return f, tea.Quit
	case key.Matches(press, formKeyMap.Save):
		return f.submit()
	case key.Matches(press, formKeyMap.Back):
		if f.focus == 0 {
			return f, nil
		}
		return f, f.setFocus(f.focus - 1)
	case key.Matches(press, formKeyMap.Next):
		if message := f.validate(f.Focus()); message != "" {
			f.err = message
			return f, nil
		}
		if f.focus == len(f.fields)-1 {
			return f.submit()
		}
		return f, f.setFocus(f.focus + 1)
	}
	switch f.Focus() {
	case FieldCurrency:
		f.currency = move(f.currency, len(money.Codes()), press, 1, 0)
		f.syncBalancePlaceholder()
		return f, nil
	case FieldColor:
		f.color = move(f.color, len(account.Palette), press, 1, paletteWidth)
		return f, nil
	}
	return f.updateInput(msg)
}

// move returns index after an arrow key. Left and right change it by
// horizontal, up and down by vertical. It stays inside [0, count).
func move(index, count int, press tea.KeyPressMsg, horizontal, vertical int) int {
	next := index
	switch {
	case key.Matches(press, formKeyMap.Left):
		next -= horizontal
	case key.Matches(press, formKeyMap.Right):
		next += horizontal
	case key.Matches(press, formKeyMap.Up):
		next -= vertical
	case key.Matches(press, formKeyMap.Down):
		next += vertical
	}
	if next < 0 || next >= count {
		return index
	}
	return next
}

func (f Form) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	field := f.Focus()
	if !f.isText(field) {
		return f, nil
	}
	before := f.inputs[field].Value()
	var cmd tea.Cmd
	f.inputs[field], cmd = f.inputs[field].Update(msg)
	if field == FieldCode {
		if clean := cleanCode(f.inputs[field].Value()); clean != f.inputs[field].Value() {
			position := f.inputs[field].Position()
			f.inputs[field].SetValue(clean)
			f.inputs[field].SetCursor(min(position, len(clean)))
		}
	}
	after := f.inputs[field].Value()
	if after == before {
		return f, cmd
	}
	f.err = ""
	switch field {
	case FieldCode:
		f.codeTouched = after != ""
	case FieldName:
		f.suggestCode()
	}
	return f, cmd
}

// cleanCode makes typed text uppercase and drops characters a code cannot have.
func cleanCode(text string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, text)
}

// View renders the form. It is empty after the form closes.
func (f Form) View() tea.View {
	if f.submitted || f.cancelled {
		return tea.NewView("")
	}
	rows := []string{styleTitle.Render(f.title), ""}
	for index, field := range f.fields {
		focused := index == f.focus
		rows = append(rows, f.row(field, focused))
		if focused && f.err != "" {
			rows = append(rows, strings.Repeat(" ", labelWidth)+styleError.Render(f.err))
		}
	}
	rows = append(rows, "", f.preview(), "", f.help.ShortHelpView(f.bindings()))
	return tea.NewView(styleBox.Width(f.width).Render(strings.Join(rows, "\n")) + "\n")
}

func (f Form) bindings() []key.Binding {
	bindings := []key.Binding{formKeyMap.Next, formKeyMap.Back}
	switch f.Focus() {
	case FieldCurrency:
		bindings = append(bindings, formKeyMap.Left)
	case FieldColor:
		bindings = append(bindings, formKeyMap.Up)
	}
	return append(bindings, formKeyMap.Save, formKeyMap.Cancel)
}

func (f Form) row(field Field, focused bool) string {
	label := styleDim.Width(labelWidth).Render(fieldLabels[field])
	if focused {
		label = styleTitle.Width(labelWidth).Render(fieldLabels[field])
	}
	switch field {
	case FieldCurrency:
		return label + f.currencyRow(focused)
	case FieldColor:
		return lipgloss.JoinHorizontal(lipgloss.Top, label, f.colorGrid(focused))
	}
	return label + f.inputs[field].View()
}

func (f Form) currencyRow(focused bool) string {
	var pills []string
	for index, code := range money.Codes() {
		style := styleDim
		if index == f.currency {
			style = styleTitle
			if focused {
				style = style.Reverse(true)
			}
		}
		pills = append(pills, style.Render(" "+code+" "))
	}
	return strings.Join(pills, " ")
}

func (f Form) colorGrid(focused bool) string {
	var lines []string
	var line strings.Builder
	for index, color := range account.Palette {
		left, right := " ", " "
		if index == f.color {
			left, right = "[", "]"
			if focused {
				left, right = styleBold.Render(left), styleBold.Render(right)
			} else {
				left, right = styleDim.Render(left), styleDim.Render(right)
			}
		}
		line.WriteString(left + swatch(color.Key, "██") + right)
		if index%paletteWidth == paletteWidth-1 {
			lines = append(lines, line.String())
			line.Reset()
		}
	}
	return strings.Join(append(lines, styleDim.Render(" "+f.Value(FieldColor))), "\n")
}

func (f Form) preview() string {
	current := f.Account()
	code, name := current.Code, current.Name
	if code == "" {
		code = "CODE"
	}
	if name == "" {
		name = "Account name"
	}
	balance := money.Format(current.Balance(), current.Currency)
	return styleDim.Width(labelWidth).Render("Preview") +
		swatch(current.Color, "●") + " " + styleBold.Render(code) + "  " + name + "  " + styleDim.Render(balance)
}
