package main

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/money"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

func transactionsHelp() string {
	return "Manage transactions\n\n" +
		green.Render("Usage:") + " finux transactions [command] [arguments]\n" +
		"       finux tx [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [filters]       List transactions; select one to see its details (default)
  <id>                    Show the details of a transaction
  show <id>               Same as <id>
  add, a [options]        Create a transaction
  edit, e [id] [options]  Change a transaction
  delete, d [id]          Delete a transaction permanently
  help, --help, -h        Show this help

` + green.Render("Filters for list:") + `
  --category <code>     Transactions of a category
  --tag <tags>          Transactions that have all these tags, for example "market weekly"
  --search <text>       Transactions whose title contains the text
  --date <date>         A year (2026), a month (11/2026 or 2026/11) or a date
  --account <code>      Transactions of an account or a credit card
  --type <type>         ` + strings.Join(transaction.Types(), ", ") + `, ` + transaction.Payment + `

` + green.Render("Options for add and edit:") + `
  --title <text>        Title, at most ` + strconv.Itoa(transaction.MaxTitle) + ` characters
  --description <text>  Description, at most ` + strconv.Itoa(transaction.MaxDescription) + ` characters
  --value <amount>      Amount in the currency of the account, for example 1234.56
  --type <type>         ` + strings.Join(transaction.Types(), ", ") + `
                        (default: expense for a red category, income for a green one)
  --category <code>     Category; red is for expenses, green for incomes, grey for all
  --tags <tags>         Tags, for example "market weekly" or "#market #weekly"
  --account <code>      Account or credit card of the transaction
  --date <date>         DD/MM/YYYY or YYYY/MM/DD (default: today)

` + green.Render("Options for add, for a transfer between accounts:") + `
  --to <code>           Account that receives the money
  --received <amount>   Amount the other account receives, when the two accounts
                        have different currencies

` + green.Render("Options for delete:") + `
  --yes                 Do not ask for confirmation

Without options, add and edit open a form. The form suggests the titles and
the tags that other transactions have; press tab to accept a suggestion.
Without an id, edit and delete open the list.

In the list, the search input finds transactions by title. It also reads
filters: #tag, @ACCOUNT, cat:CODE, type:TYPE and a date such as 2026, 2026-11
or 17/11/2026.

A transfer is two transactions, one in each account. A change or a delete of
one of them also changes or deletes the other. A transfer keeps its type and
its accounts. In a shell, write a tag without # or in quotes.

A transaction of a credit card goes to a bill of the card. A credit card has
no transfers. The payment of a bill is a transaction of the type payment: it
comes from 'finux cc pay' and it cannot change. See 'finux cc --help'.
`
}

// transactionsCommand runs one transactions subcommand.
type transactionsCommand struct {
	streams
	store      *transaction.Store
	accounts   *account.Store
	categories *category.Store
	cards      *card.Store
}

// errOutdated means the database lacks tables or columns of this Finux.
var errOutdated = errors.New("the database is out of date. Run 'finux init' to update it")

// transactionsReady reports whether the database has the tables of the
// transactions and the credit cards. A database from an older Finux
// gets them with 'finux init'.
func transactionsReady(db *gorm.DB) bool {
	migrator := db.Migrator()
	return migrator.HasTable(&transaction.Transaction{}) && migrator.HasTable(&card.Card{}) && migrator.HasTable(&card.Bill{}) &&
		migrator.HasColumn(&transaction.Transaction{}, "card_id") && migrator.HasColumn(&transaction.Transaction{}, "bill_id")
}

func runTransactions(args []string, io streams) int {
	command := &transactionsCommand{streams: io}
	handlers := map[string]func([]string) error{
		"list": command.list, "l": command.list,
		"add": command.add, "a": command.add,
		"show": command.show,
		"edit": command.edit, "e": command.edit,
		"delete": command.delete, "d": command.delete,
	}
	// No argument or an option lists transactions. A word that is not a
	// command is the id of a transaction to show.
	handler, rest := command.list, args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if known, ok := handlers[args[0]]; ok {
			handler, rest = known, args[1:]
		} else {
			handler = command.show
		}
	}
	// Help needs no database, so look for it before the database opens.
	for i, arg := range rest {
		if arg == "-h" || arg == "--help" || i == 0 && isHelp(arg) {
			return write(io, transactionsHelp())
		}
	}
	db, err := openDatabase()
	if err == nil && !transactionsReady(db) {
		err = errOutdated
	}
	if err != nil {
		return exitStatus(io, "transactions", err)
	}
	command.store, command.accounts, command.categories = transaction.NewStore(db), account.NewStore(db), category.NewStore(db)
	command.cards = card.NewStore(db)
	return exitStatus(io, "transactions", handler(rest))
}

func (c *transactionsCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

const noTransactions = notice("No transactions yet. Run 'finux tx add'.")

// choose opens the list of transactions and returns the one the user
// selected.
func (c *transactionsCommand) choose(title string, views []transaction.View) (transaction.View, error) {
	ledger, err := program(ui.NewLedger(title, views))
	if err != nil {
		return transaction.View{}, err
	}
	index, ok := ledger.Selected()
	if !ok {
		return transaction.View{}, cancelled
	}
	return views[index], nil
}

// resolve finds the transaction for an id. Without an id it opens the
// list.
func (c *transactionsCommand) resolve(command, ref, title string) (transaction.View, error) {
	if ref != "" {
		id, err := strconv.ParseUint(ref, 10, 32)
		if err != nil {
			return transaction.View{}, fmt.Errorf("transaction not found: %s", ref)
		}
		found, err := c.store.Find(uint(id))
		if errors.Is(err, transaction.ErrNotFound) {
			return found, fmt.Errorf("transaction not found: %s", ref)
		}
		return found, err
	}
	if !c.interactive {
		return transaction.View{}, usageError(command + ": a transaction id is necessary when input is not a terminal")
	}
	views, err := c.store.List(transaction.Filter{})
	if err != nil {
		return transaction.View{}, err
	}
	if len(views) == 0 {
		return transaction.View{}, noTransactions
	}
	return c.choose(title, views)
}

// findPlace finds the account or the credit card of a transaction by
// code or id. An account wins when the two have the same id. A credit
// card comes back as an account with the ID 0, and its own id.
func (c *transactionsCommand) findPlace(ref string, archived bool) (account.Account, uint, error) {
	found, err := c.accounts.Find(ref, archived)
	if !errors.Is(err, account.ErrNotFound) {
		return found, 0, err
	}
	owner, err := c.cards.Find(ref, archived)
	if errors.Is(err, card.ErrNotFound) {
		return found, 0, fmt.Errorf("account not found: %s", ref)
	}
	shown := account.Account{Name: owner.Name, Code: owner.Code, Color: owner.Color, Currency: owner.Currency, DeletedAt: owner.DeletedAt}
	return shown, owner.ID, err
}

// findAccount finds an account by code or id. Archived accounts are
// found only when archived is true.
func (c *transactionsCommand) findAccount(ref string, archived bool) (account.Account, error) {
	found, err := c.accounts.Find(ref, archived)
	if errors.Is(err, account.ErrNotFound) {
		return found, fmt.Errorf("account not found: %s", ref)
	}
	return found, err
}

func (c *transactionsCommand) findCategory(ref string) (category.Category, error) {
	found, err := c.categories.Find(ref)
	if errors.Is(err, category.ErrNotFound) {
		return found, fmt.Errorf("category not found: %s", ref)
	}
	return found, err
}

func (c *transactionsCommand) list(args []string) error {
	var categoryRef, tags, search, date, accountRef, kind string
	_, given, err := parse("list", args, func(f *flag.FlagSet) {
		f.StringVar(&categoryRef, "category", "", "")
		f.StringVar(&tags, "tag", "", "")
		f.StringVar(&search, "search", "", "")
		f.StringVar(&date, "date", "", "")
		f.StringVar(&accountRef, "account", "", "")
		f.StringVar(&kind, "type", "", "")
	}, 0)
	if err != nil {
		return err
	}
	filter := transaction.Filter{Title: strings.TrimSpace(search), Type: strings.ToLower(strings.TrimSpace(kind))}
	if given["type"] && !transaction.ValidType(filter.Type) && filter.Type != transaction.Payment {
		return usageError(fmt.Sprintf("list: unknown type %q: use one of %s", kind, strings.Join(transaction.Types(), ", ")))
	}
	if given["date"] {
		if filter.Period, err = transaction.ParsePeriod(date); err != nil {
			return usageError("list: " + err.Error())
		}
	}
	if given["tag"] {
		parsed, err := transaction.ParseTags(tags)
		if err != nil {
			return usageError("list: " + err.Error())
		}
		filter.Tags = strings.Fields(parsed)
	}
	if given["category"] {
		found, err := c.findCategory(categoryRef)
		if err != nil {
			return err
		}
		filter.Category = found.Code
	}
	if given["account"] {
		found, _, err := c.findPlace(accountRef, true)
		if err != nil {
			return err
		}
		filter.Account = found.Code
	}
	views, err := c.store.List(filter)
	if err != nil {
		return err
	}
	if len(views) == 0 && len(given) > 0 {
		return c.print("No transactions match the filter.\n")
	}
	if len(views) == 0 || !c.interactive {
		return c.print(ui.TransactionTable(views, c.plain))
	}
	title := "Transactions"
	if len(given) > 0 {
		title += "  " + filter.String()
	}
	selected, err := c.choose(title, views)
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	return c.print(ui.TransactionCard(selected, c.plain))
}

func (c *transactionsCommand) show(args []string) error {
	positional, _, err := parse("show", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("show", reference(positional), "Show transaction")
	if err != nil {
		return err
	}
	return c.print(ui.TransactionCard(found, c.plain))
}

// transactionFlags holds the text of the options of add and edit.
type transactionFlags struct {
	title, description, value, kind, category, tags, account, date, to, received string
}

func (v *transactionFlags) define(flags *flag.FlagSet, transfer bool) {
	flags.StringVar(&v.title, "title", "", "")
	flags.StringVar(&v.description, "description", "", "")
	flags.StringVar(&v.value, "value", "", "")
	flags.StringVar(&v.kind, "type", "", "")
	flags.StringVar(&v.category, "category", "", "")
	flags.StringVar(&v.tags, "tags", "", "")
	flags.StringVar(&v.account, "account", "", "")
	flags.StringVar(&v.date, "date", "", "")
	if transfer {
		flags.StringVar(&v.to, "to", "", "")
		flags.StringVar(&v.received, "received", "", "")
	}
}

// check cleans and validates the options that need no database. It
// writes the title, description, type, tags and date to the draft.
func (v *transactionFlags) check(command string, given map[string]bool, draft *transaction.Draft) error {
	var err error
	if given["title"] {
		draft.Title = strings.TrimSpace(v.title)
		err = transaction.CheckTitle(draft.Title)
	}
	if err == nil && given["description"] {
		draft.Description = strings.TrimSpace(v.description)
		err = transaction.CheckDescription(draft.Description)
	}
	if err == nil && given["type"] {
		if draft.Type = strings.ToLower(strings.TrimSpace(v.kind)); !transaction.ValidType(draft.Type) {
			err = fmt.Errorf("unknown type %q: use one of %s", v.kind, strings.Join(transaction.Types(), ", "))
		}
	}
	if err == nil && given["tags"] {
		draft.Tags, err = transaction.ParseTags(v.tags)
	}
	if err == nil && given["date"] {
		draft.Date, err = transaction.ParseDate(v.date)
	}
	if err != nil {
		return usageError(command + ": " + err.Error())
	}
	return nil
}

// amount reads the text of an amount option in a currency.
func amount(command, text, currency string) (int64, error) {
	value, err := money.Parse(text, currency)
	if err == nil && value <= 0 {
		err = errors.New("the value must be more than zero")
	}
	if err != nil {
		return 0, usageError(command + ": " + err.Error())
	}
	return value, nil
}

// suggestions returns the titles and the tags in use.
func (c *transactionsCommand) suggestions() (titles, tags []string, err error) {
	if titles, err = c.store.Titles(); err != nil {
		return nil, nil, err
	}
	tags, err = c.store.Tags()
	return titles, tags, err
}

func (c *transactionsCommand) add(args []string) error {
	var values transactionFlags
	_, given, err := parse("add", args, func(f *flag.FlagSet) { values.define(f, true) }, 0)
	if err != nil {
		return err
	}
	// A new transaction has the date of today.
	draft := transaction.Draft{Date: today()}
	if err := values.check("add", given, &draft); err != nil {
		return err
	}
	switch {
	case given["received"] && !given["to"]:
		return usageError("add: --received is only for a transfer: give --to")
	case given["to"] && given["type"] && draft.Type != transaction.Transfer:
		return usageError("add: only a transfer has a destination account (--to)")
	case given["to"]:
		draft.Type = transaction.Transfer
	case draft.Type == transaction.Transfer && !c.interactive:
		return usageError("add: --to is necessary for a transfer")
	}

	var from, to account.Account
	if given["account"] {
		if from, draft.CardID, err = c.findPlace(values.account, false); err != nil {
			return err
		}
		draft.AccountID = from.ID
	}
	if given["to"] {
		if to, err = c.findAccount(values.to, false); err != nil {
			return err
		}
		draft.ToAccountID = to.ID
	}
	var chosen category.Category
	switch {
	case given["category"]:
		if chosen, err = c.findCategory(values.category); err != nil {
			return err
		}
	case draft.Type == transaction.Transfer:
		// A transfer has the Transfer category when the user gives none.
		if chosen, err = c.categories.Find(category.TransferCode); err != nil && !errors.Is(err, category.ErrNotFound) {
			return err
		}
	}
	draft.CategoryID = chosen.ID
	// The color of the category gives the type: red is an expense and
	// green is an income.
	if draft.Type == "" {
		switch chosen.Color {
		case "red":
			draft.Type = transaction.Expense
		case "green":
			draft.Type = transaction.Income
		case "grey":
			if !c.interactive {
				return usageError(fmt.Sprintf("add: --type is necessary: the category %s is for all types", chosen.Code))
			}
		}
	}
	if given["value"] && given["account"] {
		if draft.Value, err = amount("add", values.value, from.Currency); err != nil {
			return err
		}
	}
	if given["received"] {
		if draft.Received, err = amount("add", values.received, to.Currency); err != nil {
			return err
		}
	}

	complete := given["title"] && given["value"] && given["account"] && draft.CategoryID != 0 && draft.Type != "" &&
		(draft.Type != transaction.Transfer || given["to"])
	if !complete {
		if !c.interactive {
			return usageError("add: --title, --value, --account and --category are necessary when input is not a terminal")
		}
		if draft, err = c.form("New transaction", draft, values, given); err != nil {
			return err
		}
	}
	created, err := c.store.Create(draft)
	if err != nil {
		return err
	}
	return c.print(ui.Success(label(created, "created")) + "\n")
}

// form opens the form of a new transaction. It hides the fields whose
// options were given.
func (c *transactionsCommand) form(title string, draft transaction.Draft, values transactionFlags, given map[string]bool) (transaction.Draft, error) {
	accounts, err := c.accounts.List(false)
	if err != nil {
		return draft, err
	}
	cards, err := c.cards.List(false)
	if err != nil {
		return draft, err
	}
	if len(accounts)+len(cards) == 0 {
		return draft, notice("No accounts yet. Run 'finux ac add'.")
	}
	categories, err := c.categories.List()
	if err != nil {
		return draft, err
	}
	titles, tags, err := c.suggestions()
	if err != nil {
		return draft, err
	}
	var preset []ui.TxField
	for name, field := range map[string]ui.TxField{
		"title": ui.TxTitle, "description": ui.TxDescription, "value": ui.TxValue, "category": ui.TxCategory,
		"tags": ui.TxTags, "account": ui.TxAccount, "date": ui.TxDate, "to": ui.TxTo, "received": ui.TxReceived,
	} {
		if given[name] {
			preset = append(preset, field)
		}
	}
	// The type is final when the user gave it, or a destination account,
	// or a category that is for one type only.
	if draft.Type != "" && (given["type"] || given["to"] || given["category"]) {
		preset = append(preset, ui.TxType)
	}
	form, err := program(ui.NewTransactionForm(ui.TransactionFormOptions{
		Title: title, Initial: draft, Value: values.value, Received: values.received,
		Accounts: accounts, Cards: cards, Categories: categories, Titles: titles, Tags: tags, Preset: preset, Today: today(),
	}))
	if err != nil {
		return draft, err
	}
	if !form.Submitted() {
		return draft, cancelled
	}
	return form.Draft(), nil
}

// label names a transaction in a message: "Transaction 3", or for a
// transfer "Transfer" with the ids of its two sides.
func label(current transaction.Transaction, action string) string {
	if current.PairID != 0 {
		return fmt.Sprintf("Transfer %s: transactions %d and %d", action, current.ID, current.PairID)
	}
	return fmt.Sprintf("Transaction %d %s", current.ID, action)
}

func (c *transactionsCommand) edit(args []string) error {
	var values transactionFlags
	positional, given, err := parse("edit", args, func(f *flag.FlagSet) { values.define(f, false) }, 1)
	if err != nil {
		return err
	}
	var changes transaction.Draft
	if err := values.check("edit", given, &changes); err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --title, --description, --value, --type, --category, --tags, --account or --date when input is not a terminal")
	}
	current, err := c.resolve("edit", reference(positional), "Edit transaction")
	if err != nil {
		return err
	}
	if current.Type == transaction.Payment {
		return errors.New("a payment cannot change: delete it to open the bill again")
	}
	draft := transaction.DraftOf(current.Transaction)
	if len(given) == 0 {
		if draft, err = c.editForm(current, draft); err != nil {
			return err
		}
	}
	if given["title"] {
		draft.Title = changes.Title
	}
	if given["description"] {
		draft.Description = changes.Description
	}
	if given["type"] {
		draft.Type = changes.Type
	}
	if given["tags"] {
		draft.Tags = changes.Tags
	}
	if given["date"] {
		draft.Date = changes.Date
	}
	if given["category"] {
		found, err := c.findCategory(values.category)
		if err != nil {
			return err
		}
		draft.CategoryID = found.ID
	}
	currency := current.Account.Currency
	if given["account"] {
		found, cardID, err := c.findPlace(values.account, true)
		if err != nil {
			return err
		}
		if found.Archived() && (found.ID != current.AccountID || cardID != current.CardID) {
			return fmt.Errorf("account not found: %s (it is archived; restore it first)", found.Code)
		}
		if found.Currency != currency && !given["value"] {
			return usageError(fmt.Sprintf("edit: give --value: the account %s is in %s, not in %s", found.Code, found.Currency, currency))
		}
		draft.AccountID, draft.CardID, currency = found.ID, cardID, found.Currency
	}
	if given["value"] {
		if draft.Value, err = amount("edit", values.value, currency); err != nil {
			return err
		}
	}
	updated, err := c.store.Update(current.ID, draft)
	if err != nil {
		return err
	}
	return c.print(ui.Success(label(updated, "updated")) + "\n")
}

// editForm opens the form with the values of a transaction.
func (c *transactionsCommand) editForm(current transaction.View, draft transaction.Draft) (transaction.Draft, error) {
	accounts, err := c.accounts.List(false)
	if err != nil {
		return draft, err
	}
	cards, err := c.cards.List(false)
	if err != nil {
		return draft, err
	}
	// An archived account or card stays in the list of its transaction.
	switch {
	case current.CardID == 0 && !slices.ContainsFunc(accounts, func(a account.Account) bool { return a.ID == current.AccountID }):
		accounts = append(accounts, current.Account)
	case current.CardID != 0 && current.Card != nil && !slices.ContainsFunc(cards, func(c card.Card) bool { return c.ID == current.CardID }):
		cards = append(cards, *current.Card)
	}
	categories, err := c.categories.List()
	if err != nil {
		return draft, err
	}
	titles, tags, err := c.suggestions()
	if err != nil {
		return draft, err
	}
	form, err := program(ui.NewTransactionForm(ui.TransactionFormOptions{
		Title: fmt.Sprintf("Edit transaction %d", current.ID), Initial: draft, Edit: true,
		Value:    money.Input(current.Value(), current.Account.Currency),
		Accounts: accounts, Cards: cards, Categories: categories, Titles: titles, Tags: tags,
	}))
	if err != nil {
		return draft, err
	}
	if !form.Submitted() {
		return draft, cancelled
	}
	return form.Draft(), nil
}

func (c *transactionsCommand) delete(args []string) error {
	var yes bool
	positional, _, err := parse("delete", args, func(f *flag.FlagSet) { f.BoolVar(&yes, "yes", false, "") }, 1)
	if err != nil {
		return err
	}
	if !yes && !c.interactive {
		return usageError("delete: --yes is necessary when input is not a terminal")
	}
	current, err := c.resolve("delete", reference(positional), "Delete transaction")
	if err != nil {
		return err
	}
	if !yes {
		question := fmt.Sprintf("Delete transaction %d (%s) permanently?", current.ID, current.Title)
		if current.PairID != 0 {
			question = fmt.Sprintf("Delete the transfer %s (transactions %d and %d) permanently?", current.Title, current.ID, current.PairID)
		}
		if current.Type == transaction.Payment {
			question = fmt.Sprintf("Delete the payment %d (%s)? The bill opens again.", current.ID, current.Title)
		}
		confirm, err := program(ui.NewConfirm(question, "Delete permanently", true))
		if err != nil {
			return err
		}
		if !confirm.Confirmed() {
			return cancelled
		}
	}
	if err := c.store.Delete(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success(label(current.Transaction, "deleted")) + "\n")
}
