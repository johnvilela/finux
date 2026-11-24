package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/category"
	"finux/src/code"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/transaction"
)

// Field identifies one input of a form.
type Field int

// The fields of a form, in display order. Each form shows a part of
// them: the limit is for a credit card and a budget, the days are only
// for a credit card, the opening balance only for an account.
const (
	FieldName Field = iota
	FieldCode
	FieldDescription
	// FieldKind is the type of a goal.
	FieldKind
	FieldCurrency
	FieldBalance
	FieldLimit
	// FieldTarget and FieldDate are the target and the final date of a goal.
	FieldTarget
	// FieldCategories, FieldEvery and FieldStart are the categories, the
	// period and the start date of a budget.
	FieldCategories
	FieldEvery
	FieldStart
	FieldClosing
	FieldDue
	FieldDate
	// FieldRollover tells whether a budget moves what a period left to
	// the next period.
	FieldRollover
	FieldColor
	fieldCount
)

var fieldLabels = [fieldCount]string{
	"Name", "Code", "Description", "Type", "Currency", "Opening balance", "Limit", "Target",
	"Categories", "Period", "Start date", "Closing day", "Due day", "Final date", "Rollover", "Color",
}

// goalFields are the fields only a goal form has. A budget form has the
// date too, as its end date.
var goalFields = []Field{FieldKind, FieldTarget, FieldDate}

// budgetFields are the fields only a budget form has.
var budgetFields = []Field{FieldCategories, FieldEvery, FieldStart, FieldRollover}

// otherFields are the fields that an account, a credit card and a
// category do not have.
var otherFields = append(slices.Clone(goalFields), budgetFields...)

const (
	labelWidth   = 17
	paletteWidth = 4
	// formWidth is the width of a form before the terminal gives its
	// size. After that the form has the width of the terminal.
	formWidth = 78
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
	Next, Back, Save, Cancel, Left, Right, Up, Down, Toggle key.Binding
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
	Toggle: key.NewBinding(key.WithKeys("space", " "), key.WithHelp("space", "pick")),
}

// keyField moves between the fields of a form that edits a record.
var keyField = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "field"))

// fieldStep returns where an arrow moves the focus in a form that edits
// a record: -1 to the field before, 1 to the next field, 0 for no move.
// A new record keeps the order of its fields.
func fieldStep(edit bool, press tea.KeyPressMsg) int {
	switch {
	case !edit:
		return 0
	case key.Matches(press, formKeyMap.Up):
		return -1
	case key.Matches(press, formKeyMap.Down):
		return 1
	}
	return 0
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
	title    string
	initial  account.Account
	category *category.Category
	card     *card.Card
	goal     *goal.Goal
	kind     int
	budget   *budget.Budget
	// categories are the choices of the categories field of a budget;
	// picked tells which ones count and cursor is the one under the keys.
	categories  []category.Category
	picked      []bool
	cursor      int
	rollover    int
	today       string
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
	hidden[FieldLimit], hidden[FieldClosing], hidden[FieldDue] = true, true, true
	for _, field := range otherFields {
		hidden[field] = true
	}
	if options.Edit {
		hidden[FieldCurrency], hidden[FieldBalance] = true, true
	}
	f := Form{title: title, initial: options.Initial, edit: options.Edit, taken: options.Taken, colors: paletteColors()}
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

// paletteColors returns the choices of the color field of an account
// or a credit card.
func paletteColors() []formColor {
	colors := make([]formColor, len(account.Palette))
	for i, color := range account.Palette {
		colors[i] = formColor{color.Key, color.Hex, color.Key}
	}
	return colors
}

// CardFormOptions configures a credit card form.
type CardFormOptions struct {
	// Title is the heading of the form.
	Title string
	// Initial holds the starting values. In edit mode it is the card to change.
	Initial card.Card
	// Limit is the starting text of the limit field.
	Limit string
	// Edit hides the currency: it does not change after creation.
	Edit bool
	// Preset lists fields that already have a final value. The form hides them.
	Preset []Field
	// Taken reports whether an account or another card uses a code.
	Taken func(code string) bool
}

// NewCardForm builds a credit card form. Its fields are name, code,
// description, currency, limit, closing day, due day and color.
func NewCardForm(options CardFormOptions) Form {
	title := options.Title
	if title == "" {
		title = "New credit card"
		if options.Edit {
			title = "Edit credit card"
		}
	}
	hidden := map[Field]bool{FieldBalance: true, FieldCurrency: options.Edit}
	for _, field := range append(slices.Clone(options.Preset), otherFields...) {
		hidden[field] = true
	}
	initial := options.Initial
	f := Form{title: title, card: &initial, edit: options.Edit, taken: options.Taken, colors: paletteColors()}
	for i, code := range money.Codes() {
		if code == initial.Currency {
			f.currency = i
		}
	}
	day := func(value int) string {
		if value == 0 {
			return ""
		}
		return strconv.Itoa(value)
	}
	f.build(hidden, initial.Color, map[Field]string{
		FieldName:        initial.Name,
		FieldCode:        initial.Code,
		FieldDescription: initial.Description,
		FieldLimit:       options.Limit,
		FieldClosing:     day(initial.ClosingDay),
		FieldDue:         day(initial.DueDay),
	}, "Visa Gold", "VISA")
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
	hidden := map[Field]bool{FieldCurrency: true, FieldBalance: true, FieldLimit: true, FieldClosing: true, FieldDue: true}
	for _, field := range append(slices.Clone(options.Preset), otherFields...) {
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

// GoalFormOptions configures a goal form.
type GoalFormOptions struct {
	// Title is the heading of the form.
	Title string
	// Initial holds the starting values. In edit mode it is the goal to change.
	Initial goal.Goal
	// Target is the starting text of the target field.
	Target string
	// Edit hides the currency: it does not change after creation.
	Edit bool
	// Preset lists fields that already have a final value. The form hides them.
	Preset []Field
	// Taken reports whether another goal uses a code.
	Taken func(code string) bool
}

// NewGoalForm builds a goal form. Its fields are title, code,
// description, type, currency, target and final date.
func NewGoalForm(options GoalFormOptions) Form {
	title := options.Title
	if title == "" {
		title = "New goal"
		if options.Edit {
			title = "Edit goal"
		}
	}
	hidden := map[Field]bool{
		FieldBalance: true, FieldLimit: true, FieldClosing: true, FieldDue: true, FieldColor: true, FieldCurrency: options.Edit,
	}
	for _, field := range append(slices.Clone(options.Preset), budgetFields...) {
		hidden[field] = true
	}
	initial := options.Initial
	f := Form{title: title, goal: &initial, edit: options.Edit, taken: options.Taken}
	for i, code := range money.Codes() {
		if code == initial.Currency {
			f.currency = i
		}
	}
	f.kind = max(0, slices.Index(goal.Types(), initial.Type))
	date := ""
	if initial.DueDate != "" {
		date = transaction.FormatDate(initial.DueDate)
	}
	f.build(hidden, "", map[Field]string{
		FieldName:        initial.Title,
		FieldCode:        initial.Code,
		FieldDescription: initial.Description,
		FieldTarget:      options.Target,
		FieldDate:        date,
	}, "Trip to Japan", "TRIP")
	f.inputs[FieldDescription].CharLimit = goal.MaxDescription
	return f
}

// BudgetFormOptions configures a budget form.
type BudgetFormOptions struct {
	// Title is the heading of the form.
	Title string
	// Initial holds the starting values. In edit mode it is the budget to
	// change.
	Initial budget.Budget
	// Limit is the starting text of the limit field.
	Limit string
	// Categories are the categories the user can pick.
	Categories []category.Category
	// Today is the current date as YYYY-MM-DD. A budget without a start
	// date starts from it.
	Today string
	// Edit hides the currency: it does not change after creation.
	Edit bool
	// Preset lists fields that already have a final value. The form hides them.
	Preset []Field
	// Taken reports whether another budget uses a code.
	Taken func(code string) bool
}

// NewBudgetForm builds a budget form. Its fields are title, code,
// description, currency, limit, categories, period, start date, end date
// and rollover.
func NewBudgetForm(options BudgetFormOptions) Form {
	title := options.Title
	if title == "" {
		title = "New budget"
		if options.Edit {
			title = "Edit budget"
		}
	}
	hidden := map[Field]bool{
		FieldKind: true, FieldBalance: true, FieldTarget: true, FieldClosing: true, FieldDue: true, FieldColor: true, FieldCurrency: options.Edit,
	}
	for _, field := range options.Preset {
		hidden[field] = true
	}
	initial := options.Initial
	f := Form{
		title: title, budget: &initial, edit: options.Edit, taken: options.Taken,
		categories: options.Categories, picked: make([]bool, len(options.Categories)), today: options.Today,
	}
	for i, code := range money.Codes() {
		if code == initial.Currency {
			f.currency = i
		}
	}
	for i, choice := range options.Categories {
		f.picked[i] = slices.Contains(initial.CategoryIDs, choice.ID)
	}
	if initial.Rollover {
		f.rollover = 1
	}
	date := func(iso string) string {
		if iso == "" {
			return ""
		}
		return transaction.FormatDate(iso)
	}
	// A new budget is monthly, unless it already is a one-time budget.
	every := strings.TrimPrefix(initial.Recurrence(), "every ")
	if !initial.Recurring() {
		every = ""
		if !options.Edit && initial.EndDate == "" {
			every = budget.Month
		}
	}
	f.build(hidden, "", map[Field]string{
		FieldName:        initial.Title,
		FieldCode:        initial.Code,
		FieldDescription: initial.Description,
		FieldLimit:       options.Limit,
		FieldEvery:       every,
		FieldStart:       date(initial.StartDate),
		FieldDate:        date(initial.EndDate),
	}, "Food", "FOOD")
	f.inputs[FieldDescription].CharLimit = budget.MaxDescription
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
	placeholders := map[Field]string{
		FieldName: namePlaceholder, FieldCode: codePlaceholder, FieldDescription: "optional",
		FieldClosing: "day of the month, 1 to 31", FieldDue: "day of the month, 1 to 31",
		FieldDate: "optional, DD/MM/YYYY", FieldStart: "optional, DD/MM/YYYY",
		FieldEvery: "month, 2 weeks, 15 days; empty for one time",
	}
	for _, field := range textFields {
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
	f.inputs[FieldLimit].CharLimit = 24
	f.inputs[FieldClosing].CharLimit = 2
	f.inputs[FieldDue].CharLimit = 2
	f.inputs[FieldTarget].CharLimit = 24
	f.inputs[FieldDate].CharLimit = 10
	f.inputs[FieldStart].CharLimit = 10
	f.inputs[FieldEvery].CharLimit = 16
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
		if len(f.colors) == 0 {
			return ""
		}
		return f.colors[f.color].key
	case FieldKind:
		return goal.Types()[f.kind]
	case FieldCategories:
		return f.pickedCodes()
	case FieldRollover:
		return yesNo(f.rollover == 1)
	}
	if f.isText(field) {
		return f.inputs[field].Value()
	}
	return ""
}

// textFields are the fields the user types in.
var textFields = []Field{
	FieldName, FieldCode, FieldDescription, FieldBalance, FieldLimit, FieldTarget, FieldEvery, FieldStart, FieldClosing, FieldDue, FieldDate,
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

// Card returns the initial credit card with the values of the form
// applied. Use it for a form that NewCardForm built.
func (f Form) Card() card.Card {
	var result card.Card
	if f.card != nil {
		result = *f.card
	}
	result.Name = strings.TrimSpace(f.Value(FieldName))
	result.Code = f.Value(FieldCode)
	result.Description = strings.TrimSpace(f.Value(FieldDescription))
	result.Color = f.Value(FieldColor)
	if !f.edit {
		result.Currency = f.Value(FieldCurrency)
	}
	if limit, err := money.Parse(f.Value(FieldLimit), result.Currency); err == nil {
		result.Limit = limit
	}
	if day, err := strconv.Atoi(strings.TrimSpace(f.Value(FieldClosing))); err == nil {
		result.ClosingDay = day
	}
	if day, err := strconv.Atoi(strings.TrimSpace(f.Value(FieldDue))); err == nil {
		result.DueDay = day
	}
	return result
}

// Goal returns the initial goal with the values of the form applied.
// Use it for a form that NewGoalForm built.
func (f Form) Goal() goal.Goal {
	var result goal.Goal
	if f.goal != nil {
		result = *f.goal
	}
	result.Title = strings.TrimSpace(f.Value(FieldName))
	result.Code = f.Value(FieldCode)
	result.Description = strings.TrimSpace(f.Value(FieldDescription))
	result.Type = f.Value(FieldKind)
	if !f.edit {
		result.Currency = f.Value(FieldCurrency)
	}
	if target, err := money.Parse(f.Value(FieldTarget), result.Currency); err == nil {
		result.Target = target
	}
	result.DueDate = ""
	if date, err := transaction.ParseDate(f.Value(FieldDate)); err == nil {
		result.DueDate = date
	}
	return result
}

// pickedCodes returns the codes of the picked categories of a budget
// form: "FOOD, FUN". It is "" when no category is picked.
func (f Form) pickedCodes() string {
	var codes []string
	for i, choice := range f.categories {
		if f.picked[i] {
			codes = append(codes, choice.Code)
		}
	}
	return strings.Join(codes, ", ")
}

// Budget returns the initial budget with the values of the form applied.
// Use it for a form that NewBudgetForm built. A budget without a start
// date gets the default one of its period.
func (f Form) Budget() budget.Budget {
	var result budget.Budget
	if f.budget != nil {
		result = *f.budget
	}
	result.Title = strings.TrimSpace(f.Value(FieldName))
	result.Code = f.Value(FieldCode)
	result.Description = strings.TrimSpace(f.Value(FieldDescription))
	if !f.edit {
		result.Currency = f.Value(FieldCurrency)
	}
	if limit, err := money.Parse(f.Value(FieldLimit), result.Currency); err == nil {
		result.Limit = limit
	}
	result.CategoryIDs = nil
	for i, choice := range f.categories {
		if f.picked[i] {
			result.CategoryIDs = append(result.CategoryIDs, choice.ID)
		}
	}
	if every, unit, err := budget.ParseEvery(f.Value(FieldEvery)); err == nil {
		result.Every, result.Unit = every, unit
	}
	result.StartDate = budget.DefaultStart(result.Every, result.Unit, f.today)
	if date, err := transaction.ParseDate(f.Value(FieldStart)); err == nil {
		result.StartDate = date
	}
	result.EndDate = ""
	if date, err := transaction.ParseDate(f.Value(FieldDate)); err == nil {
		result.EndDate = date
	}
	result.Rollover = f.rollover == 1
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

// currencyCode returns the currency of the record of the form. A card
// form in edit mode does not show the currency field.
func (f Form) currencyCode() string {
	if f.card != nil && f.edit {
		return f.card.Currency
	}
	if f.goal != nil && f.edit {
		return f.goal.Currency
	}
	if f.budget != nil && f.edit {
		return f.budget.Currency
	}
	return f.Value(FieldCurrency)
}

func (f Form) isText(field Field) bool {
	return slices.Contains(textFields, field)
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
	f.inputs[FieldLimit].Placeholder = money.Placeholder(f.currencyCode())
	f.inputs[FieldTarget].Placeholder = money.Placeholder(f.currencyCode())
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
			return f.label(FieldName) + " is required"
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
	case FieldLimit:
		if strings.TrimSpace(f.Value(FieldLimit)) == "" {
			return "Limit is required"
		}
		limit, err := money.Parse(f.Value(FieldLimit), f.currencyCode())
		if err != nil {
			message := err.Error()
			return sentence(message[strings.LastIndex(message, ": ")+1:])
		}
		if limit < 0 {
			return "Limit cannot be negative"
		}
		if f.budget != nil && limit == 0 {
			return "Limit must be more than zero"
		}
	case FieldEvery:
		if _, _, err := budget.ParseEvery(f.Value(FieldEvery)); err != nil {
			return reason(err)
		}
	case FieldStart:
		if strings.TrimSpace(f.Value(FieldStart)) == "" {
			return ""
		}
		if _, err := transaction.ParseDate(f.Value(FieldStart)); err != nil {
			return reason(err)
		}
	case FieldTarget:
		if strings.TrimSpace(f.Value(FieldTarget)) == "" {
			return "Target is required"
		}
		target, err := money.Parse(f.Value(FieldTarget), f.currencyCode())
		if err != nil {
			return reason(err)
		}
		if target <= 0 {
			return "Target must be more than zero"
		}
	case FieldDate:
		if strings.TrimSpace(f.Value(FieldDate)) == "" {
			if f.budget != nil && !f.Budget().Recurring() {
				return "End date is necessary without a period"
			}
			return ""
		}
		date, err := transaction.ParseDate(f.Value(FieldDate))
		if err != nil {
			return reason(err)
		}
		if f.budget != nil && date < f.Budget().StartDate {
			return "End date is before the start date"
		}
	case FieldClosing, FieldDue:
		day, err := strconv.Atoi(strings.TrimSpace(f.Value(field)))
		if err != nil {
			day = 0
		}
		if err := card.CheckDay(strings.ToLower(fieldLabels[field]), day); err != nil {
			return sentence(err.Error())
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

// inputRoom is the width of a text input in a form that has width
// columns: the room after the border, the padding, the label and the
// cursor at the end of the text.
func inputRoom(width int) int { return max(20, width-7-labelWidth) }

// Update handles a key or a timer message.
func (f Form) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		f.width = size.Width
		for field, input := range f.inputs {
			input.SetWidth(inputRoom(f.width))
			// The input then shows the text again from the new width.
			input.SetCursor(input.Position())
			f.inputs[field] = input
		}
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
		// An edit checks the fields when it saves, so the user can
		// leave a field and come back.
		if message := f.validate(f.Focus()); message != "" && !f.edit {
			f.err = message
			return f, nil
		}
		if f.focus == len(f.fields)-1 {
			return f.submit()
		}
		return f, f.setFocus(f.focus + 1)
	}
	// The arrows of a grid of colors stay in the grid while they can move.
	inGrid := f.Focus() == FieldColor && !f.colorsInRow() && move(f.color, len(f.colors), press, 0, paletteWidth) != f.color
	if step := fieldStep(f.edit, press); step != 0 && !inGrid {
		if next := f.focus + step; next >= 0 && next < len(f.fields) {
			return f, f.setFocus(next)
		}
		return f, nil
	}
	switch f.Focus() {
	case FieldKind:
		f.kind = move(f.kind, len(goal.Types()), press, 1, 0)
		return f, nil
	case FieldCurrency:
		f.currency = move(f.currency, len(money.Codes()), press, 1, 0)
		f.syncBalancePlaceholder()
		return f, nil
	case FieldCategories:
		if key.Matches(press, formKeyMap.Toggle) && len(f.picked) > 0 {
			// The form is a value: change a copy of the choices.
			f.picked = slices.Clone(f.picked)
			f.picked[f.cursor] = !f.picked[f.cursor]
			return f, nil
		}
		f.cursor = move(f.cursor, len(f.categories), press, 1, 0)
		return f, nil
	case FieldRollover:
		f.rollover = move(f.rollover, 2, press, 1, 0)
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
	case FieldCurrency, FieldKind, FieldRollover:
		bindings = append(bindings, formKeyMap.Left)
	case FieldCategories:
		bindings = append(bindings, formKeyMap.Left, formKeyMap.Toggle)
	case FieldColor:
		if f.colorsInRow() {
			bindings = append(bindings, formKeyMap.Left)
		} else {
			bindings = append(bindings, formKeyMap.Up)
		}
	}
	if f.edit {
		bindings = append(bindings, keyField)
	}
	return append(bindings, formKeyMap.Save, formKeyMap.Cancel)
}

func (f Form) row(field Field, focused bool) string {
	label := styleDim.Width(labelWidth).Render(f.label(field))
	if focused {
		label = styleTitle.Width(labelWidth).Render(f.label(field))
	}
	switch field {
	case FieldKind:
		labels := make([]string, len(goal.Types()))
		for i, kind := range goal.Types() {
			labels[i] = goalTypeLabel(kind)
		}
		return label + pills(labels, f.kind, focused)
	case FieldCurrency:
		return label + f.currencyRow(focused)
	case FieldCategories:
		return lipgloss.JoinHorizontal(lipgloss.Top, label, f.categoryRows(focused))
	case FieldRollover:
		return label + pills([]string{"no", "yes"}, f.rollover, focused)
	case FieldColor:
		if f.colorsInRow() {
			return label + f.colorRow(focused)
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, label, f.colorGrid(focused))
	}
	return label + f.inputs[field].View()
}

// label returns the label of a field. A goal and a budget have a title,
// not a name. The date of a budget is its end date.
func (f Form) label(field Field) string {
	switch {
	case (f.goal != nil || f.budget != nil) && field == FieldName:
		return "Title"
	case f.budget != nil && field == FieldDate:
		return "End date"
	}
	return fieldLabels[field]
}

// categoryRows shows the categories of a budget form in lines. A picked
// category has a mark; the one under the keys is reversed.
func (f Form) categoryRows(focused bool) string {
	if len(f.categories) == 0 {
		return styleDim.Render("no categories")
	}
	room := f.width - 6 - labelWidth
	var lines []string
	line, width := "", 0
	for index, choice := range f.categories {
		text, style := "  "+choice.Code+" ", styleDim
		if f.picked[index] {
			text, style = " ✓"+choice.Code+" ", styleTitle
		}
		if focused && index == f.cursor {
			style = style.Reverse(true)
		}
		if width > 0 && width+lipgloss.Width(text) > room {
			lines = append(lines, line)
			line, width = "", 0
		}
		line += style.Render(text)
		width += lipgloss.Width(text)
	}
	return strings.Join(append(lines, line), "\n")
}

func (f Form) currencyRow(focused bool) string {
	return pills(money.Codes(), f.currency, focused)
}

// pills shows the choices of a field in one row, with the selected one
// marked.
func pills(labels []string, selected int, focused bool) string {
	var rendered []string
	for index, label := range labels {
		style := styleDim
		if index == selected {
			style = styleTitle
			if focused {
				style = style.Reverse(true)
			}
		}
		rendered = append(rendered, style.Render(" "+label+" "))
	}
	return strings.Join(rendered, " ")
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
	if f.budget != nil {
		current := f.Budget()
		code, title := current.Code, current.Title
		if code == "" {
			code = "CODE"
		}
		if title == "" {
			title = "Budget title"
		}
		categories := f.pickedCodes()
		switch count := len(current.CategoryIDs); {
		case count == 0:
			categories = "all categories"
		case count > 3:
			categories = fmt.Sprintf("%d categories", count)
		}
		details := current.Recurrence() + " · " + money.Format(current.Limit, current.Currency) + " · " + categories
		// The title gets the room the other parts leave in one line.
		room := f.width - 6 - labelWidth - lipgloss.Width(code) - 4 - lipgloss.Width(details)
		return styleDim.Width(labelWidth).Render("Preview") +
			styleBold.Render(code) + "  " + cut(title, max(room, 6)) + "  " + styleDim.Render(details)
	}
	if f.goal != nil {
		current := f.Goal()
		code, title := current.Code, current.Title
		if code == "" {
			code = "CODE"
		}
		if title == "" {
			title = "Goal title"
		}
		details := goalTypeLabel(current.Type) + " · " + money.Format(current.Target, current.Currency)
		if current.DueDate != "" {
			details += " · until " + transaction.FormatDate(current.DueDate)
		}
		// The title gets the room the other parts leave in one line.
		room := f.width - 6 - labelWidth - lipgloss.Width(code) - 4 - lipgloss.Width(details)
		return styleDim.Width(labelWidth).Render("Preview") +
			styleBold.Render(code) + "  " + cut(title, max(room, 6)) + "  " + styleDim.Render(details)
	}
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
	if f.card != nil {
		current := f.Card()
		code, name := current.Code, current.Name
		if code == "" {
			code = "CODE"
		}
		if name == "" {
			name = "Card name"
		}
		// The days show when they have a value.
		details := money.Format(current.Limit, current.Currency)
		if current.ClosingDay != 0 {
			details += fmt.Sprintf(" · closes %d", current.ClosingDay)
		}
		if current.DueDay != 0 {
			details += fmt.Sprintf(" · due %d", current.DueDay)
		}
		return styleDim.Width(labelWidth).Render("Preview") +
			dot + " " + styleBold.Render(code) + "  " + cut(name, 24) + "  " + styleDim.Render(details)
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
