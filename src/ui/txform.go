package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"finux/src/account"
	"finux/src/category"
	"finux/src/money"
	"finux/src/transaction"
)

// TxField identifies one input of the transaction form.
type TxField int

// The fields of the transaction form, in display order. The form shows
// TxTo only for a transfer, and TxReceived only for a transfer between
// two currencies.
const (
	TxType TxField = iota
	TxTitle
	TxValue
	TxAccount
	TxTo
	TxReceived
	TxCategory
	TxDate
	TxTags
	TxDescription
	txFieldCount
)

var txLabels = [txFieldCount]string{"Type", "Title", "Value", "Account", "To account", "Received", "Category", "Date", "Tags", "Description"}

// TransactionFormOptions configures a transaction form.
type TransactionFormOptions struct {
	// Title is the heading of the form.
	Title string
	// Initial holds the starting values. Its Value and Received are not
	// used: the fields below give the text of the amounts.
	Initial transaction.Draft
	// Value and Received are the starting text of the amount fields.
	Value, Received string
	// Edit changes a transaction that exists. An expense or an income
	// cannot become a transfer; a transfer keeps its type and its accounts.
	Edit bool
	// Accounts and Categories are the records the user can choose.
	Accounts   []account.Account
	Categories []category.Category
	// Titles and Tags are the values in use. The form suggests them
	// while the user types.
	Titles, Tags []string
	// Preset lists fields that already have a final value. The form hides them.
	Preset []TxField
	// Today is the date of a new transaction, as YYYY-MM-DD.
	Today string
}

var (
	keyAccept  = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "accept suggestion"))
	keySuggest = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "other suggestions"))
	keyJump    = key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→ or a letter", "choose"))
)

// TransactionForm is the interactive screen that creates or edits a
// transaction.
type TransactionForm struct {
	title      string
	initial    transaction.Draft
	edit       bool
	accounts   []account.Account
	categories []category.Category
	tags       []string
	types      []string
	hidden     [txFieldCount]bool
	inputs     [txFieldCount]textinput.Model
	// The current choices: an index in types and the ids of records.
	kind                        int
	accountID, toID, categoryID uint
	focus                       TxField
	err                         string
	help                        help.Model
	submitted, cancelled        bool
	width                       int
}

// NewTransactionForm builds a transaction form.
func NewTransactionForm(options TransactionFormOptions) TransactionForm {
	transfer := options.Initial.Type == transaction.Transfer
	title := options.Title
	if title == "" {
		title = "New transaction"
		if options.Edit {
			title = "Edit transaction"
		}
	}
	f := TransactionForm{
		title: title, initial: options.Initial, edit: options.Edit,
		accounts: options.Accounts, categories: options.Categories, tags: options.Tags,
		accountID: options.Initial.AccountID, toID: options.Initial.ToAccountID, categoryID: options.Initial.CategoryID,
		help: help.New(), width: formWidth,
	}
	for _, field := range options.Preset {
		f.hidden[field] = true
	}
	// The types the user can choose. A transfer needs two accounts.
	switch {
	case options.Edit && transfer:
		f.types = []string{transaction.Transfer}
		f.hidden[TxType], f.hidden[TxAccount], f.hidden[TxTo], f.hidden[TxReceived] = true, true, true, true
	case options.Edit || len(options.Accounts) < 2 && !transfer:
		f.types = []string{transaction.Expense, transaction.Income}
	default:
		f.types = transaction.Types()
	}
	f.kind = max(0, slices.Index(f.types, options.Initial.Type))

	date := options.Initial.Date
	if date == "" {
		date = options.Today
	}
	if date == "" {
		date = transaction.Today()
	}
	values := map[TxField]string{
		TxTitle: options.Initial.Title, TxValue: options.Value, TxReceived: options.Received,
		TxDate: transaction.FormatDate(date), TxTags: options.Initial.Tags, TxDescription: options.Initial.Description,
	}
	limits := map[TxField]int{
		TxTitle: transaction.MaxTitle, TxValue: 24, TxReceived: 24, TxDate: 10, TxTags: 120, TxDescription: transaction.MaxDescription,
	}
	placeholders := map[TxField]string{TxTitle: "Market", TxDate: "DD/MM/YYYY", TxTags: "#market #weekly", TxDescription: "optional"}
	for field, limit := range limits {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = placeholders[field]
		input.CharLimit = limit
		input.SetWidth(48)
		input.SetValue(values[field])
		f.inputs[field] = input
	}
	f.inputs[TxTitle].ShowSuggestions = true
	f.inputs[TxTitle].SetSuggestions(options.Titles)
	f.inputs[TxTags].ShowSuggestions = true
	f.sync()
	if fields := f.fields(); len(fields) > 0 {
		f.focus = fields[0]
		if f.isText(f.focus) {
			f.inputs[f.focus].Focus()
		}
	}
	return f
}

// Init starts the cursor blink.
func (f TransactionForm) Init() tea.Cmd { return textinput.Blink }

func (f TransactionForm) kindName() string { return f.types[f.kind] }

func (f TransactionForm) isText(field TxField) bool {
	return field != TxType && field != TxAccount && field != TxTo && field != TxCategory
}

// fields returns the fields the form shows now, in order.
func (f TransactionForm) fields() []TxField {
	var fields []TxField
	for field := TxType; field < txFieldCount; field++ {
		switch {
		case f.hidden[field],
			field == TxTo && f.kindName() != transaction.Transfer,
			field == TxReceived && !f.twoCurrencies():
		default:
			fields = append(fields, field)
		}
	}
	return fields
}

func (f TransactionForm) accountByID(id uint) (account.Account, bool) {
	for _, current := range f.accounts {
		if current.ID == id {
			return current, true
		}
	}
	return account.Account{}, false
}

// destinations returns the accounts that can receive a transfer.
func (f TransactionForm) destinations() []account.Account {
	return slices.DeleteFunc(slices.Clone(f.accounts), func(current account.Account) bool { return current.ID == f.accountID })
}

// choices returns the categories the current type can use.
func (f TransactionForm) choices() []category.Category {
	return slices.DeleteFunc(slices.Clone(f.categories), func(current category.Category) bool {
		return !transaction.Accepts(f.kindName(), current.Color)
	})
}

// twoCurrencies reports whether the form is a new transfer between
// accounts with different currencies.
func (f TransactionForm) twoCurrencies() bool {
	if f.kindName() != transaction.Transfer || f.edit {
		return false
	}
	from, okFrom := f.accountByID(f.accountID)
	to, okTo := f.accountByID(f.toID)
	return okFrom && okTo && from.Currency != to.Currency
}

// sync makes the choices valid after the type or an account changed,
// and updates the hints of the amount fields.
func (f *TransactionForm) sync() {
	if _, ok := f.accountByID(f.accountID); !ok && len(f.accounts) > 0 {
		f.accountID = f.accounts[0].ID
	}
	destinations := f.destinations()
	if !slices.ContainsFunc(destinations, func(current account.Account) bool { return current.ID == f.toID }) {
		f.toID = 0
		if len(destinations) > 0 {
			f.toID = destinations[0].ID
		}
	}
	choices := f.choices()
	if !slices.ContainsFunc(choices, func(current category.Category) bool { return current.ID == f.categoryID }) {
		f.categoryID = 0
		if len(choices) > 0 {
			f.categoryID = choices[0].ID
		}
	}
	from, _ := f.accountByID(f.accountID)
	to, _ := f.accountByID(f.toID)
	f.inputs[TxValue].Placeholder = money.Placeholder(from.Currency)
	f.inputs[TxReceived].Placeholder = money.Placeholder(to.Currency)
}

// Focus returns the field that has the focus.
func (f TransactionForm) Focus() TxField { return f.focus }

// Value returns the current text of a field. For an account or a
// category it is the code.
func (f TransactionForm) Value(field TxField) string {
	switch field {
	case TxType:
		return f.kindName()
	case TxAccount:
		current, _ := f.accountByID(f.accountID)
		return current.Code
	case TxTo:
		current, _ := f.accountByID(f.toID)
		return current.Code
	case TxCategory:
		for _, current := range f.categories {
			if current.ID == f.categoryID {
				return current.Code
			}
		}
		return ""
	}
	return f.inputs[field].Value()
}

// Error returns the validation message of the focused field, or "".
func (f TransactionForm) Error() string { return f.err }

// Submitted reports whether the user saved the form.
func (f TransactionForm) Submitted() bool { return f.submitted }

// Cancelled reports whether the user left the form without saving.
func (f TransactionForm) Cancelled() bool { return f.cancelled }

// amount reads an amount field in the currency of an account.
func (f TransactionForm) amount(field TxField, accountID uint) (int64, error) {
	current, _ := f.accountByID(accountID)
	return money.Parse(f.Value(field), current.Currency)
}

// Draft returns the initial values with the values of the form applied.
func (f TransactionForm) Draft() transaction.Draft {
	draft := f.initial
	draft.Title = strings.TrimSpace(f.Value(TxTitle))
	draft.Description = strings.TrimSpace(f.Value(TxDescription))
	draft.Type = f.kindName()
	draft.AccountID, draft.CategoryID = f.accountID, f.categoryID
	draft.Value, _ = f.amount(TxValue, f.accountID)
	draft.ToAccountID, draft.Received = 0, 0
	if draft.Type == transaction.Transfer && !f.edit {
		draft.ToAccountID = f.toID
		if f.twoCurrencies() {
			draft.Received, _ = f.amount(TxReceived, f.toID)
		}
	}
	if date, err := transaction.ParseDate(f.Value(TxDate)); err == nil {
		draft.Date = date
	}
	if tags, err := transaction.ParseTags(f.Value(TxTags)); err == nil {
		draft.Tags = tags
	}
	return draft
}

// reason keeps the part of an error after its last colon: the field
// already shows the value.
func reason(err error) string {
	message := err.Error()
	return sentence(message[strings.LastIndex(message, ": ")+1:])
}

func (f TransactionForm) validate(field TxField) string {
	switch field {
	case TxTitle:
		if strings.TrimSpace(f.Value(TxTitle)) == "" {
			return "Title is required"
		}
	case TxValue, TxReceived:
		accountID := f.accountID
		if field == TxReceived {
			accountID = f.toID
		}
		if strings.TrimSpace(f.Value(field)) == "" {
			if field == TxReceived {
				return "Received value is required"
			}
			return "Value is required"
		}
		amount, err := f.amount(field, accountID)
		if err != nil {
			return reason(err)
		}
		if amount <= 0 {
			return "The value must be more than zero"
		}
	case TxAccount:
		if _, ok := f.accountByID(f.accountID); !ok {
			return "No account. Run 'finux ac add'"
		}
	case TxTo:
		if _, ok := f.accountByID(f.toID); !ok {
			return "A transfer needs two accounts"
		}
	case TxCategory:
		if f.categoryID == 0 {
			return "No category for this type. Run 'finux ct add'"
		}
	case TxDate:
		if _, err := transaction.ParseDate(f.Value(TxDate)); err != nil {
			return reason(err)
		}
	case TxTags:
		if _, err := transaction.ParseTags(f.Value(TxTags)); err != nil {
			return reason(err)
		}
	}
	return ""
}

func (f *TransactionForm) setFocus(field TxField) tea.Cmd {
	if f.isText(f.focus) {
		f.inputs[f.focus].Blur()
	}
	f.focus, f.err = field, ""
	if f.isText(field) {
		f.inputs[field].CursorEnd()
		return f.inputs[field].Focus()
	}
	return nil
}

func (f TransactionForm) submit() (tea.Model, tea.Cmd) {
	for _, field := range f.fields() {
		if message := f.validate(field); message != "" {
			cmd := f.setFocus(field)
			f.err = message
			return f, cmd
		}
	}
	f.submitted = true
	return f, tea.Quit
}

// suggestion returns the text the focused field suggests, or "" when
// it has nothing to add to what the user typed.
func (f TransactionForm) suggestion() string {
	if f.focus != TxTitle && f.focus != TxTags {
		return ""
	}
	input := f.inputs[f.focus]
	if len(input.MatchedSuggestions()) == 0 {
		return ""
	}
	if suggestion := input.CurrentSuggestion(); suggestion != input.Value() {
		return suggestion
	}
	return ""
}

// suggestTags completes the word the user types in the tags field with
// a tag in use that the field does not have yet.
func (f *TransactionForm) suggestTags() {
	value := f.inputs[TxTags].Value()
	start := strings.LastIndexAny(value, " ,") + 1
	word := strings.ToLower(value[start:])
	var suggestions []string
	if strings.HasPrefix(word, "#") && len(word) > 1 {
		present := strings.Fields(strings.ToLower(value[:start]))
		for _, tag := range f.tags {
			if strings.HasPrefix(tag, word) && !slices.Contains(present, tag) {
				suggestions = append(suggestions, value[:start]+tag)
			}
		}
	}
	f.inputs[TxTags].SetSuggestions(suggestions)
}

// Update handles a key or a timer message.
func (f TransactionForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		f.width = min(size.Width, formWidth)
		return f, nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f.updateInput(msg)
	}
	fields := f.fields()
	index := slices.Index(fields, f.focus)
	switch {
	case key.Matches(press, formKeyMap.Cancel):
		f.cancelled = true
		return f, tea.Quit
	case key.Matches(press, formKeyMap.Save):
		return f.submit()
	case key.Matches(press, formKeyMap.Back):
		if index <= 0 {
			return f, nil
		}
		return f, f.setFocus(fields[index-1])
	case key.Matches(press, keyAccept) && f.suggestion() != "":
		// Use the suggestion as it is stored, not with the case the user typed.
		f.inputs[f.focus].SetValue(f.suggestion())
		f.inputs[f.focus].CursorEnd()
		f.err = ""
		f.suggestTags()
		return f, nil
	case key.Matches(press, formKeyMap.Next):
		if message := f.validate(f.focus); message != "" {
			f.err = message
			return f, nil
		}
		if index == len(fields)-1 {
			return f.submit()
		}
		return f, f.setFocus(fields[index+1])
	}
	if f.isText(f.focus) {
		return f.updateInput(msg)
	}
	f.choose(press)
	return f, nil
}

// choose changes the choice of the focused field: arrows go to the
// previous or the next choice, a letter goes to the next choice that
// starts with it.
func (f *TransactionForm) choose(press tea.KeyPressMsg) {
	switch f.focus {
	case TxType:
		before := f.kind
		f.kind = move(f.kind, len(f.types), press, 1, 0)
		// A transfer starts with the Transfer category.
		for _, current := range f.categories {
			if f.kind != before && f.kindName() == transaction.Transfer && current.Code == category.TransferCode {
				f.categoryID = current.ID
			}
		}
	case TxAccount:
		f.accountID = pickAccount(f.accounts, f.accountID, press)
	case TxTo:
		f.toID = pickAccount(f.destinations(), f.toID, press)
	case TxCategory:
		choices := f.choices()
		labels := make([][2]string, len(choices))
		ids := make([]uint, len(choices))
		for i, current := range choices {
			labels[i], ids[i] = [2]string{current.Code, current.Name}, current.ID
		}
		if next := cycle(labels, slices.Index(ids, f.categoryID), press); next >= 0 {
			f.categoryID = ids[next]
		}
	}
	f.sync()
}

func pickAccount(accounts []account.Account, id uint, press tea.KeyPressMsg) uint {
	labels := make([][2]string, len(accounts))
	ids := make([]uint, len(accounts))
	for i, current := range accounts {
		labels[i], ids[i] = [2]string{current.Code, current.Name}, current.ID
	}
	if next := cycle(labels, slices.Index(ids, id), press); next >= 0 {
		return ids[next]
	}
	return id
}

// cycle returns the index of a list of choices after a key. Left and
// right go around the list. A letter or a digit goes to the next choice
// whose code or name starts with it. It returns -1 for an empty list.
func cycle(labels [][2]string, index int, press tea.KeyPressMsg) int {
	count := len(labels)
	if count == 0 {
		return -1
	}
	index = max(index, 0)
	switch {
	case key.Matches(press, formKeyMap.Left):
		return (index + count - 1) % count
	case key.Matches(press, formKeyMap.Right):
		return (index + 1) % count
	}
	letter := strings.ToLower(press.Text)
	if letter == "" || letter == " " {
		return index
	}
	for _, part := range []int{0, 1} {
		for offset := 1; offset <= count; offset++ {
			candidate := (index + offset) % count
			if strings.HasPrefix(strings.ToLower(labels[candidate][part]), letter) {
				return candidate
			}
		}
	}
	return index
}

func (f TransactionForm) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !f.isText(f.focus) {
		return f, nil
	}
	before := f.inputs[f.focus].Value()
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	if f.inputs[f.focus].Value() != before {
		f.err = ""
		if f.focus == TxTags {
			f.suggestTags()
		}
	}
	return f, cmd
}

// View renders the form. It is empty after the form closes.
func (f TransactionForm) View() tea.View {
	if f.submitted || f.cancelled {
		return tea.NewView("")
	}
	rows := []string{styleTitle.Render(f.title), ""}
	for _, field := range f.fields() {
		focused := field == f.focus
		rows = append(rows, f.row(field, focused))
		if focused && f.err != "" {
			rows = append(rows, strings.Repeat(" ", labelWidth)+styleError.Render(f.err))
		}
	}
	rows = append(rows, "", f.preview(), "", f.help.ShortHelpView(f.bindings()))
	return tea.NewView(styleBox.Width(f.width).Render(strings.Join(rows, "\n")) + "\n")
}

func (f TransactionForm) bindings() []key.Binding {
	switch {
	case f.suggestion() != "":
		return []key.Binding{keyAccept, keySuggest, formKeyMap.Save, formKeyMap.Cancel}
	case f.focus == TxType:
		return []key.Binding{formKeyMap.Next, formKeyMap.Back, formKeyMap.Left, formKeyMap.Save, formKeyMap.Cancel}
	case !f.isText(f.focus):
		return []key.Binding{formKeyMap.Next, keyJump, formKeyMap.Save, formKeyMap.Cancel}
	}
	return []key.Binding{formKeyMap.Next, formKeyMap.Back, formKeyMap.Save, formKeyMap.Cancel}
}

func (f TransactionForm) row(field TxField, focused bool) string {
	label := styleDim.Width(labelWidth).Render(txLabels[field])
	if focused {
		label = styleTitle.Width(labelWidth).Render(txLabels[field])
	}
	switch field {
	case TxType:
		var pills []string
		for index, kind := range f.types {
			style := styleDim
			if index == f.kind {
				style = typeStyle(kind).Bold(true)
				if focused {
					style = style.Reverse(true)
				}
			}
			pills = append(pills, style.Render(" "+kind+" "))
		}
		return label + strings.Join(pills, " ")
	case TxAccount, TxTo:
		id, accounts := f.accountID, f.accounts
		if field == TxTo {
			id, accounts = f.toID, f.destinations()
		}
		current, ok := f.accountByID(id)
		if !ok {
			return label + styleDim.Render("no account")
		}
		position := slices.IndexFunc(accounts, func(a account.Account) bool { return a.ID == id })
		return label + choice(swatch(current.Color, "●"), current.Code, current.Name+" · "+current.Currency, position, len(accounts), focused)
	case TxCategory:
		choices := f.choices()
		position := slices.IndexFunc(choices, func(c category.Category) bool { return c.ID == f.categoryID })
		if position < 0 {
			return label + styleDim.Render("no category")
		}
		current := choices[position]
		color, _ := category.ColorByKey(current.Color)
		return label + choice(paint(color.Hex, "●"), current.Code, current.Name, position, len(choices), focused)
	case TxDescription:
		if focused {
			count := fmt.Sprintf("%d/%d", utf8.RuneCountInString(f.Value(field)), transaction.MaxDescription)
			return label + f.inputs[field].View() + "\n" + strings.Repeat(" ", labelWidth) + styleDim.Render(count)
		}
	}
	return label + f.inputs[field].View()
}

// choice shows the current record of a list the user goes through.
func choice(dot, code, name string, position, count int, focused bool) string {
	left, right, label := styleDim.Render("‹"), styleDim.Render("›"), styleBold.Render(code)
	if focused {
		left, right, label = styleAccent.Render("‹"), styleAccent.Render("›"), styleTitle.Render(code)
	}
	return left + " " + dot + " " + label + "  " + name + " " + right + styleDim.Render(fmt.Sprintf("  %d/%d", position+1, count))
}

func (f TransactionForm) preview() string {
	draft := f.Draft()
	current, _ := f.accountByID(f.accountID)
	title := draft.Title
	if title == "" {
		title = "Title"
	}
	// The form does not know the side of a transfer it edits, so that
	// amount has no sign.
	amount := draft.Value
	if draft.Type == transaction.Expense || draft.Type == transaction.Transfer && !f.edit {
		amount = -amount
	}
	line := typeStyle(draft.Type).Render("●") + " " + styleBold.Render(title) + "  " +
		typeStyle(draft.Type).Render(money.Format(amount, current.Currency)) + "  " + styleDim.Render(current.Code)
	if to, ok := f.accountByID(draft.ToAccountID); ok {
		line += styleDim.Render(" → " + to.Code)
	}
	return styleDim.Width(labelWidth).Render("Preview") + line
}
