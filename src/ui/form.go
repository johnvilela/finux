package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"finux/src/account"
	"finux/src/category"
	"finux/src/code"
	"finux/src/money"
)

// Field identifies one input of a form.
type Field int

// The fields of a form, in display order. The category form does not
// have the currency and balance fields.
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
	formWidth    = 78
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

// formColor is one choice of the color field.
type formColor struct {
	key, hex string
	// label is the text of the choice, for example "red expense".
	label string
}

// Form is the interactive screen that creates or edits an account or a
// category.
type Form struct {
	title       string
	initial     account.Account
	category    *category.Category
	colors      []formColor
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
	title := options.Title
	if title == "" {
		title = "New account"
		if options.Edit {
			title = "Edit account"
		}
	}
	hidden := map[Field]bool{}
	for _, field := range options.Preset {
		hidden[field] = true
	}
	if options.Edit {
		hidden[FieldCurrency], hidden[FieldBalance] = true, true
	}
	colors := make([]formColor, len(account.Palette))
	for i, color := range account.Palette {
		colors[i] = formColor{color.Key, color.Hex, color.Key}
	}
	f := Form{title: title, initial: options.Initial, edit: options.Edit, taken: options.Taken, colors: colors}
	for i, code := range money.Codes() {
		if code == options.Initial.Currency {
			f.currency = i
		}
	}
	f.build(hidden, options.Initial.Color, map[Field]string{
		FieldName:        options.Initial.Name,
		FieldCode:        options.Initial.Code,
		FieldDescription: options.Initial.Description,
		FieldBalance:     options.Balance,
	}, "Banco Inter", "INTER")
	return f
}

// CategoryFormOptions configures a category form.
type CategoryFormOptions struct {
	// Title is the heading of the form.
	Title string
	// Initial holds the starting values. In edit mode it is the category to change.
	Initial category.Category
	// Edit keeps the code as it is when the name changes.
	Edit bool
	// Preset lists fields that already have a final value. The form hides them.
	Preset []Field
	// Taken reports whether another category uses a code.
	Taken func(code string) bool
}

// NewCategoryForm builds a category form. Its fields are name, code,
// description and color.
func NewCategoryForm(options CategoryFormOptions) Form {
	title := options.Title
	if title == "" {
		title = "New category"
		if options.Edit {
			title = "Edit category"
		}
	}
	hidden := map[Field]bool{FieldCurrency: true, FieldBalance: true}
	for _, field := range options.Preset {
		hidden[field] = true
	}
	colors := make([]formColor, len(category.Colors))
	for i, color := range category.Colors {
		colors[i] = formColor{color.Key, color.Hex, color.Key + " " + color.Meaning}
	}
	initial := options.Initial
	f := Form{title: title, category: &initial, edit: options.Edit, taken: options.Taken, colors: colors}
	f.build(hidden, initial.Color, map[Field]string{
		FieldName:        initial.Name,
		FieldCode:        initial.Code,
		FieldDescription: initial.Description,
	}, "Groceries", "FOOD")
	return f
}

// build sets up the fields, the inputs and the first focus of a form.
func (f *Form) build(hidden map[Field]bool, color string, values map[Field]string, namePlaceholder, codePlaceholder string) {
	f.help, f.width = help.New(), formWidth
	if f.taken == nil {
		f.taken = func(string) bool { return false }
	}
	for field := FieldName; field < fieldCount; field++ {
		if !hidden[field] {
			f.fields = append(f.fields, field)
		}
	}
	for i, choice := range f.colors {
		if choice.key == color {
			f.color = i
		}
	}
	placeholders := map[Field]string{FieldName: namePlaceholder, FieldCode: codePlaceholder, FieldDescription: "optional"}
	for _, field := range []Field{FieldName, FieldCode, FieldDescription, FieldBalance} {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = placeholders[field]
		input.SetWidth(40)
		input.SetValue(values[field])
		f.inputs[field] = input
	}
	f.inputs[FieldCode].CharLimit = code.MaxLength
	f.inputs[FieldName].CharLimit = 60
	f.inputs[FieldDescription].CharLimit = 120
	f.inputs[FieldBalance].CharLimit = 24
	f.codeTouched = values[FieldCode] != ""
	f.suggestCode()
	f.syncBalancePlaceholder()
	if len(f.fields) > 0 && f.isText(f.fields[0]) {
		f.inputs[f.fields[0]].Focus()
	}
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
		return f.colors[f.color].key
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

// Category returns the initial category with the values of the form
// applied. Use it for a form that NewCategoryForm built.
func (f Form) Category() category.Category {
	var result category.Category
	if f.category != nil {
		result = *f.category
	}
	result.Name = strings.TrimSpace(f.Value(FieldName))
	result.Code = f.Value(FieldCode)
	result.Description = strings.TrimSpace(f.Value(FieldDescription))
	result.Color = f.Value(FieldColor)
	return result
}

// colorsInRow reports whether the color field is one row of choices,
// not a grid.
func (f Form) colorsInRow() bool { return len(f.colors) <= paletteWidth }

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
	f.inputs[FieldCode].SetValue(code.Suggest(f.Value(FieldName), f.taken))
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
		value := f.Value(FieldCode)
		if err := code.Validate(value); err != nil {
			return sentence(err.Error())
		}
		if f.taken(value) {
			return "Code " + value + " is already in use"
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
		vertical := paletteWidth
		if f.colorsInRow() {
			vertical = 0
		}
		f.color = move(f.color, len(f.colors), press, 1, vertical)
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
		if f.colorsInRow() {
			bindings = append(bindings, formKeyMap.Left)
		} else {
			bindings = append(bindings, formKeyMap.Up)
		}
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
		if f.colorsInRow() {
			return label + f.colorRow(focused)
		}
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

// colorRow shows a few colors as pills with their labels.
func (f Form) colorRow(focused bool) string {
	var pills []string
	for index, choice := range f.colors {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(choice.hex))
		if index == f.color {
			style = style.Bold(true)
			if focused {
				style = style.Reverse(true)
			} else {
				style = style.Underline(true)
			}
		}
		pills = append(pills, style.Render(" "+choice.label+" "))
	}
	return strings.Join(pills, " ")
}

func (f Form) colorGrid(focused bool) string {
	var lines []string
	var line strings.Builder
	for index, color := range f.colors {
		left, right := " ", " "
		if index == f.color {
			left, right = "[", "]"
			if focused {
				left, right = styleBold.Render(left), styleBold.Render(right)
			} else {
				left, right = styleDim.Render(left), styleDim.Render(right)
			}
		}
		line.WriteString(left + paint(color.hex, "██") + right)
		if index%paletteWidth == paletteWidth-1 {
			lines = append(lines, line.String())
			line.Reset()
		}
	}
	return strings.Join(append(lines, styleDim.Render(" "+f.Value(FieldColor))), "\n")
}

func (f Form) preview() string {
	dot := paint(f.colors[f.color].hex, "●")
	if f.category != nil {
		current := f.Category()
		code, name := current.Code, current.Name
		if code == "" {
			code = "CODE"
		}
		if name == "" {
			name = "Category name"
		}
		return styleDim.Width(labelWidth).Render("Preview") +
			dot + " " + styleBold.Render(code) + "  " + name + "  " + styleDim.Render(current.Meaning())
	}
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
		dot + " " + styleBold.Render(code) + "  " + name + "  " + styleDim.Render(balance)
}
