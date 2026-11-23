package main

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/schedule"
	"finux/src/setting"
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
  --goal <code>         Transactions linked to a goal
  --virtual             Transactions made with a virtual card of a credit card;
                        --virtual=false gives the other ones
  --type <type>         ` + strings.Join(transaction.Types(), ", ") + `, ` + transaction.Payment + `

` + green.Render("Options for add and edit:") + `
  --title <text>        Title, at most ` + strconv.Itoa(transaction.MaxTitle) + ` characters
  --description <text>  Description, at most ` + strconv.Itoa(transaction.MaxDescription) + ` characters
  --value <amount>      Amount in the currency of the account, for example 1234.56.
                        With installments: the total of the parts
  --type <type>         ` + strings.Join(transaction.Types(), ", ") + `
                        (default: expense for a red category, income for a green one)
  --category <code>     Category; red is for expenses, green for incomes, grey for all
  --tags <tags>         Tags, for example "market weekly" or "#market #weekly"
  --account <code>      Account or credit card of the transaction
  --date <date>         DD/MM/YYYY or YYYY/MM/DD (default: today)
  --goal <code>         Goal the transaction counts toward; 'none' takes it away.
                        See 'finux goals --help'
  --virtual             The person paid with a virtual card of the credit card.
                        In edit, --virtual=false takes the mark away

` + green.Render("Options for add, for an expense or an income in installments:") + `
  --installments <number>       Number of monthly parts, from 2 to ` + strconv.Itoa(transaction.MaxInstallments) + `
  --installment-value <amount>  Value of one part, in place of --value
  --from <number>               First part to create, for a purchase that is
                                already in progress (default: 1)

` + green.Render("Options for edit and delete, for a transaction in installments:") + `
  --all                 Change or delete all the parts
  --next                Change or delete this part and the parts after it

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
filters: #tag, @ACCOUNT, cat:CODE, type:TYPE, virtual:yes or virtual:no, and a
date such as 2026, 2026-11 or 17/11/2026.

A transfer is two transactions, one in each account. A change or a delete of
one of them also changes or deletes the other. A transfer keeps its type and
its accounts. In a shell, write a tag without # or in quotes.

A transaction in installments is one transaction for each monthly part. The
value is the total: the parts are equal and the cents that remain go to the
first part. --date is the date of the first part that add creates; each part
after it is one month later. On a credit card each part goes to the bill
after the one before, and all the parts use the limit from the first day. On
an account a part changes the balance on its date. The list shows the part
after the title, such as 3/10.

Without --all or --next, edit and delete change one part only, and the other
parts keep their numbers. With --all or --next, a title, a description, a
type, a category, tags, a goal, an account and --virtual go to each part; --value is
the new total of these parts and --installment-value is the new value of
each one; --date is the date of the first of these parts, and the others
follow by month. A part in a paid bill stops the change of all the parts:
use --next from a part in a bill that is not paid. The number of parts does
not change.

A transaction of a credit card goes to a bill of the card. A credit card has
no transfers. The payment of a bill is a transaction of the type payment: it
comes from 'finux cc pay' and it cannot change. See 'finux cc --help'.

--virtual marks a transaction of a credit card that the person paid with a
virtual card. The lists show [virtual] after its title, and the open bill of
the card shows the total of these transactions. A transaction that goes to an
account loses the mark.
`
}

// transactionsCommand runs one transactions subcommand.
type transactionsCommand struct {
	streams
	store      *transaction.Store
	accounts   *account.Store
	categories *category.Store
	cards      *card.Store
	goals      *goal.Store
	budgets    *budget.Store
	schedules  *schedule.Store
}

// errOutdated means the database lacks tables or columns of this Finux.
var errOutdated = errors.New("the database is out of date. Run 'finux init' to update it")

// transactionsReady reports whether the database has the tables of the
// transactions, the credit cards, the goals and the budgets. A database from an older Finux
// gets them with 'finux init'.
func transactionsReady(db *gorm.DB) bool {
	migrator := db.Migrator()
	return migrator.HasTable(&transaction.Transaction{}) && migrator.HasTable(&card.Card{}) && migrator.HasTable(&card.Bill{}) &&
		migrator.HasColumn(&transaction.Transaction{}, "card_id") && migrator.HasColumn(&transaction.Transaction{}, "bill_id") &&
		migrator.HasTable(&goal.Goal{}) && migrator.HasColumn(&transaction.Transaction{}, "goal_id") &&
		migrator.HasColumn(&transaction.Transaction{}, "installment_id") && migrator.HasColumn(&transaction.Transaction{}, "virtual") &&
		migrator.HasColumn(&transaction.Transaction{}, "status") && migrator.HasColumn(&card.Bill{}, "status") &&
		migrator.HasTable(&schedule.Schedule{}) && migrator.HasColumn(&transaction.Transaction{}, "schedule_id") &&
		migrator.HasTable(&setting.Setting{}) &&
		migrator.HasTable(&budget.Budget{}) && migrator.HasTable(&budget.Link{})
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
	db, err := openLedger(io)
	if err != nil {
		return exitStatus(io, "transactions", err)
	}
	defer closeDatabase(db)
	command.store, command.accounts, command.categories = transaction.NewStore(db), account.NewStore(db), category.NewStore(db)
	command.cards, command.goals, command.budgets = card.NewStore(db), goal.NewStore(db), budget.NewStore(db)
	command.schedules = schedule.NewStore(db)
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
	found, err := c.accounts.Find(ref, archived, today())
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
	found, err := c.accounts.Find(ref, archived, today())
	if errors.Is(err, account.ErrNotFound) {
		return found, fmt.Errorf("account not found: %s", ref)
	}
	return found, err
}

// findGoal finds the goal for the text of a --goal option. It returns 0
// for the word none and for an empty text: no goal. An archived goal is
// found, so that the store can tell why it takes no new transaction.
func (c *transactionsCommand) findGoal(ref string) (goal.Goal, error) {
	if ref = strings.TrimSpace(ref); ref == "" || strings.EqualFold(ref, noGoal) {
		return goal.Goal{}, nil
	}
	found, err := c.goals.Find(ref, true)
	if errors.Is(err, goal.ErrNotFound) {
		return found, fmt.Errorf("goal not found: %s", ref)
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
	var categoryRef, tags, search, date, accountRef, kind, goalRef string
	var virtual bool
	_, given, err := parse("list", args, func(f *flag.FlagSet) {
		f.StringVar(&categoryRef, "category", "", "")
		f.StringVar(&tags, "tag", "", "")
		f.StringVar(&search, "search", "", "")
		f.StringVar(&date, "date", "", "")
		f.StringVar(&accountRef, "account", "", "")
		f.StringVar(&kind, "type", "", "")
		f.StringVar(&goalRef, "goal", "", "")
		f.BoolVar(&virtual, "virtual", false, "")
	}, 0)
	if err != nil {
		return err
	}
	filter, err := c.filter(listFlags{category: categoryRef, tags: tags, search: search, date: date, account: accountRef, kind: kind, goal: goalRef, virtual: virtual}, given)
	if err != nil {
		return err
	}
	views, err := c.store.List(filter)
	if err != nil {
		return err
	}
	if len(views) == 0 && len(given) > 0 {
		return c.print("No transactions match the filter.\n")
	}
	if len(views) == 0 || !c.interactive {
		notice, err := dueNotice(c.schedules)
		if err != nil {
			return err
		}
		return c.print(ui.TransactionTable(views, c.plain) + notice)
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

// listFlags holds the text of the filters of list.
type listFlags struct {
	category, tags, search, date, account, kind, goal string
	// virtual selects the transactions made with a virtual card, or the
	// other ones.
	virtual bool
}

// filter makes the filter of a list from the options that were given.
func (c *transactionsCommand) filter(values listFlags, given map[string]bool) (transaction.Filter, error) {
	var err error
	filter := transaction.Filter{Title: strings.TrimSpace(values.search), Type: strings.ToLower(strings.TrimSpace(values.kind))}
	if given["type"] && !transaction.ValidType(filter.Type) && filter.Type != transaction.Payment {
		return filter, usageError(fmt.Sprintf("list: unknown type %q: use one of %s", values.kind, strings.Join(transaction.Types(), ", ")))
	}
	if given["date"] {
		if filter.Period, err = transaction.ParsePeriod(values.date); err != nil {
			return filter, usageError("list: " + err.Error())
		}
	}
	if given["tag"] {
		parsed, err := transaction.ParseTags(values.tags)
		if err != nil {
			return filter, usageError("list: " + err.Error())
		}
		filter.Tags = strings.Fields(parsed)
	}
	if given["category"] {
		found, err := c.findCategory(values.category)
		if err != nil {
			return filter, err
		}
		filter.Category = found.Code
	}
	if given["account"] {
		found, _, err := c.findPlace(values.account, true)
		if err != nil {
			return filter, err
		}
		filter.Account = found.Code
	}
	if given["goal"] {
		found, err := c.findGoal(values.goal)
		if err != nil {
			return filter, err
		}
		if found.ID == 0 {
			return filter, usageError("list: give the code of a goal")
		}
		filter.Goal = found.Code
	}
	if given["virtual"] {
		filter.Virtual = "no"
		if values.virtual {
			filter.Virtual = "yes"
		}
	}
	return filter, nil
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
	title, description, value, kind, category, tags, account, date, to, received, goal string
	// installmentValue is the value of one part of a transaction in
	// installments. installments is the number of parts and from is the
	// first part to create.
	installmentValue   string
	installments, from int
	// virtual tells that the person paid with a virtual card.
	virtual bool
	// all and next give the scope of an edit: all the parts, or the part
	// and the parts after it. parts is the number of parts of the scope.
	all, next bool
	parts     int
}

// define adds the options of add, when create is true, or of edit.
func (v *transactionFlags) define(flags *flag.FlagSet, create bool) {
	flags.StringVar(&v.title, "title", "", "")
	flags.StringVar(&v.description, "description", "", "")
	flags.StringVar(&v.value, "value", "", "")
	flags.StringVar(&v.kind, "type", "", "")
	flags.StringVar(&v.category, "category", "", "")
	flags.StringVar(&v.tags, "tags", "", "")
	flags.StringVar(&v.account, "account", "", "")
	flags.StringVar(&v.date, "date", "", "")
	flags.StringVar(&v.goal, "goal", "", "")
	flags.StringVar(&v.installmentValue, "installment-value", "", "")
	flags.BoolVar(&v.virtual, "virtual", false, "")
	if create {
		flags.StringVar(&v.to, "to", "", "")
		flags.StringVar(&v.received, "received", "", "")
		flags.IntVar(&v.installments, "installments", 0, "")
		flags.IntVar(&v.from, "from", 0, "")
		return
	}
	flags.BoolVar(&v.all, "all", false, "")
	flags.BoolVar(&v.next, "next", false, "")
}

// scopeOf reads the options --all and --next for a transaction. They
// need a transaction in installments.
func scopeOf(command string, all, next bool, current transaction.Transaction) (transaction.Scope, error) {
	switch {
	case all && next:
		return transaction.One, usageError(command + ": give --all or --next, not the two")
	case !all && !next:
		return transaction.One, nil
	case current.InstallmentID == 0:
		return transaction.One, usageError(fmt.Sprintf("%s: transaction %d is not an installment: --all and --next do not apply", command, current.ID))
	case all:
		return transaction.All, nil
	}
	return transaction.Next, nil
}

// changedOf tells which values the options of an edit change.
func changedOf(given map[string]bool) transaction.Changed {
	return transaction.Changed{
		Title: given["title"], Description: given["description"], Type: given["type"], Category: given["category"],
		Tags: given["tags"], Goal: given["goal"], Place: given["account"], Date: given["date"],
		Value: given["value"] || given["installment-value"], Virtual: given["virtual"],
	}
}

// span names the parts of a scope in a message: "Installments 4 to 10
// of Notebook updated (7 transactions)".
func span(parts []transaction.Transaction, title, action string) string {
	return fmt.Sprintf("Installments %d to %d of %s %s (%s)", parts[0].Part, parts[len(parts)-1].Part, title, action, plural(int64(len(parts)), "transaction"))
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
	draft, complete, err := c.newDraft(values, given)
	if err != nil {
		return err
	}
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
	message := label(created, "created")
	if created.InstallmentID != 0 {
		if message, err = c.createdParts(created); err != nil {
			return err
		}
	}
	return c.saved(created.ID, message)
}

// createdParts names the parts that an add created: their ids, their
// number and their values.
func (c *transactionsCommand) createdParts(first transaction.Transaction) (string, error) {
	parts, err := c.store.Parts(first.ID, transaction.All)
	if err != nil {
		return "", err
	}
	view, err := c.store.Find(first.ID)
	if err != nil {
		return "", err
	}
	return createdMessage(parts, view.Account.Currency), nil
}

// createdMessage is the feedback of an add of a transaction in
// installments. parts holds the parts it created.
func createdMessage(parts []transaction.Transaction, currency string) string {
	first, last := parts[0], parts[len(parts)-1]
	value := func(part transaction.Transaction) string { return money.Format(part.Value(), currency) }
	message := fmt.Sprintf("Transactions %d to %d created: ", first.ID, last.ID)
	switch {
	case first.Part > 1:
		return message + fmt.Sprintf("installments %d to %d of %d, %s each", first.Part, last.Part, first.Parts, value(first))
	case first.Value() != last.Value():
		return message + fmt.Sprintf("%d installments, the first of %s and the others of %s", len(parts), value(first), value(last))
	}
	return message + fmt.Sprintf("%d installments of %s", len(parts), value(first))
}

// saved prints the feedback of transactions that were created or
// changed, then one line for each budget that the transaction id
// leaves near or over its limit.
func (c *transactionsCommand) saved(id uint, message string) error {
	out := ui.Success(message) + "\n"
	view, err := c.store.Find(id)
	if err != nil {
		return err
	}
	warnings, err := budgetWarnings(c.budgets, view)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		out += ui.Warning(warning.Message) + "\n"
	}
	return c.print(out)
}

// newDraft makes the draft of a new transaction from the options that
// were given. complete is false when the draft still needs the form.
func (c *transactionsCommand) newDraft(values transactionFlags, given map[string]bool) (draft transaction.Draft, complete bool, err error) {
	draft = transaction.Draft{Date: today()}
	if err := values.check("add", given, &draft); err != nil {
		return draft, false, err
	}
	switch {
	case given["received"] && !given["to"]:
		return draft, false, usageError("add: --received is only for a transfer: give --to")
	case given["to"] && given["type"] && draft.Type != transaction.Transfer:
		return draft, false, usageError("add: only a transfer has a destination account (--to)")
	case given["to"]:
		draft.Type = transaction.Transfer
	case draft.Type == transaction.Transfer && !c.interactive:
		return draft, false, usageError("add: --to is necessary for a transfer")
	}

	var from, to account.Account
	if given["account"] {
		if from, draft.CardID, err = c.findPlace(values.account, false); err != nil {
			return draft, false, err
		}
		draft.AccountID = from.ID
	}
	if given["to"] {
		if to, err = c.findAccount(values.to, false); err != nil {
			return draft, false, err
		}
		draft.ToAccountID = to.ID
	}
	var chosen category.Category
	switch {
	case given["category"]:
		if chosen, err = c.findCategory(values.category); err != nil {
			return draft, false, err
		}
	case draft.Type == transaction.Transfer:
		if chosen, err = c.categories.Find(category.TransferCode); err != nil && !errors.Is(err, category.ErrNotFound) {
			return draft, false, err
		}
	}
	draft.CategoryID = chosen.ID
	if draft.Type == "" {
		switch chosen.Color {
		case "red":
			draft.Type = transaction.Expense
		case "green":
			draft.Type = transaction.Income
		case "grey":
			if !c.interactive {
				return draft, false, usageError(fmt.Sprintf("add: --type is necessary: the category %s is for all types", chosen.Code))
			}
		}
	}
	switch {
	case given["value"] && given["installment-value"]:
		return draft, false, usageError("add: give --value or --installment-value, not the two")
	case given["installment-value"] && !given["installments"]:
		return draft, false, usageError("add: --installment-value is only for installments: give --installments")
	case given["from"] && !given["installments"]:
		return draft, false, usageError("add: --from is only for installments: give --installments")
	case given["installments"] && (values.installments < 2 || values.installments > transaction.MaxInstallments):
		return draft, false, usageError(fmt.Sprintf("add: --installments must be a number from 2 to %d", transaction.MaxInstallments))
	case given["from"] && (values.from < 1 || values.from > values.installments):
		return draft, false, usageError(fmt.Sprintf("add: --from must be a number from 1 to %d", values.installments))
	case given["installments"] && draft.Type == transaction.Transfer:
		return draft, false, usageError("add: a transfer cannot have installments")
	}
	draft.Installments, draft.From = values.installments, values.from
	if given["value"] && given["account"] {
		if draft.Value, err = amount("add", values.value, from.Currency); err != nil {
			return draft, false, err
		}
	}
	// The value of a draft is the total of its parts.
	if given["installment-value"] && given["account"] {
		if draft.Value, err = amount("add", values.installmentValue, from.Currency); err != nil {
			return draft, false, err
		}
		draft.Value *= int64(values.installments)
	}
	if given["goal"] {
		target, err := c.findGoal(values.goal)
		if err != nil {
			return draft, false, err
		}
		draft.GoalID = target.ID
	}
	if given["virtual"] {
		draft.Virtual = values.virtual
	}
	if given["received"] {
		if draft.Received, err = amount("add", values.received, to.Currency); err != nil {
			return draft, false, err
		}
	}
	complete = given["title"] && (given["value"] || given["installment-value"]) && given["account"] && draft.CategoryID != 0 && draft.Type != "" &&
		(draft.Type != transaction.Transfer || given["to"])
	return draft, complete, nil
}

func (c *transactionsCommand) form(title string, draft transaction.Draft, values transactionFlags, given map[string]bool) (transaction.Draft, error) {
	accounts, err := c.accounts.List(false, today())
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
	goals, err := c.goals.List(false)
	if err != nil {
		return draft, err
	}
	var preset []ui.TxField
	for name, field := range map[string]ui.TxField{
		"title": ui.TxTitle, "description": ui.TxDescription, "value": ui.TxValue, "category": ui.TxCategory,
		"tags": ui.TxTags, "account": ui.TxAccount, "date": ui.TxDate, "to": ui.TxTo, "received": ui.TxReceived, "goal": ui.TxGoal, "virtual": ui.TxVirtual,
		"installments": ui.TxInstallments, "installment-value": ui.TxValue,
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
	// The form shows the total, also when the user gave the value of one part.
	if given["installment-value"] {
		if !given["account"] {
			return draft, usageError("add: --installment-value needs --account")
		}
		from, _, err := c.findPlace(values.account, false)
		if err != nil {
			return draft, err
		}
		values.value = money.Input(draft.Value, from.Currency)
	}
	form, err := program(ui.NewTransactionForm(ui.TransactionFormOptions{
		Title: title, Initial: draft, Value: values.value, Received: values.received,
		Accounts: accounts, Cards: cards, Categories: categories, Goals: goals, Titles: titles, Tags: tags, Preset: preset, Today: today(),
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
	scoped := given["all"] || given["next"]
	delete(given, "all")
	delete(given, "next")
	switch {
	case len(given) == 0 && scoped:
		return usageError("edit: give the options to change with --all or --next")
	case len(given) == 0 && !c.interactive:
		return usageError("edit: give --title, --description, --value, --type, --category, --tags, --account, --date, --goal or --virtual when input is not a terminal")
	case given["value"] && given["installment-value"]:
		return usageError("edit: give --value or --installment-value, not the two")
	case given["installment-value"] && !scoped:
		return usageError("edit: --installment-value needs --all or --next")
	}
	current, err := c.resolve("edit", reference(positional), "Edit transaction")
	if err != nil {
		return err
	}
	if current.Type == transaction.Payment {
		return errors.New("a payment cannot change: delete it to open the bill again")
	}
	scope, err := scopeOf("edit", values.all, values.next, current.Transaction)
	if err != nil {
		return err
	}
	draft := transaction.DraftOf(current.Transaction)
	if len(given) == 0 {
		if draft, err = c.editForm(current, draft); err != nil {
			return err
		}
	}
	if scope == transaction.One {
		if draft, err = c.change(current, draft, changes, values, given); err != nil {
			return err
		}
		updated, err := c.store.Update(current.ID, draft)
		if err != nil {
			return err
		}
		return c.saved(updated.ID, label(updated, "updated"))
	}
	updated, title, err := c.changeParts(current, scope, changes, values, given)
	if err != nil {
		return err
	}
	return c.saved(current.ID, span(updated, title, "updated"))
}

// changeParts applies the options that were given to the parts of a
// scope. It returns the parts and the title they now have.
func (c *transactionsCommand) changeParts(current transaction.View, scope transaction.Scope, changes transaction.Draft, values transactionFlags, given map[string]bool) ([]transaction.Transaction, string, error) {
	parts, err := c.store.Parts(current.ID, scope)
	if err != nil {
		return nil, "", err
	}
	values.parts = len(parts)
	draft, err := c.change(current, transaction.DraftOf(current.Transaction), changes, values, given)
	if err != nil {
		return nil, "", err
	}
	updated, err := c.store.UpdateParts(current.ID, scope, draft, changedOf(given))
	return updated, draft.Title, err
}

// editForm opens the form with the values of a transaction.
// change applies the options that were given to the draft of a
// transaction that exists. changes holds the options values.check read.
func (c *transactionsCommand) change(current transaction.View, draft, changes transaction.Draft, values transactionFlags, given map[string]bool) (transaction.Draft, error) {
	var err error
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
			return draft, err
		}
		draft.CategoryID = found.ID
	}
	if given["goal"] {
		target, err := c.findGoal(values.goal)
		if err != nil {
			return draft, err
		}
		draft.GoalID = target.ID
	}
	currency := current.Account.Currency
	if given["account"] {
		found, cardID, err := c.findPlace(values.account, true)
		if err != nil {
			return draft, err
		}
		if found.Archived() && (found.ID != current.AccountID || cardID != current.CardID) {
			return draft, fmt.Errorf("account not found: %s (it is archived; restore it first)", found.Code)
		}
		if found.Currency != currency && !given["value"] && !given["installment-value"] {
			return draft, usageError(fmt.Sprintf("edit: give --value: the account %s is in %s, not in %s", found.Code, found.Currency, currency))
		}
		draft.AccountID, draft.CardID, currency = found.ID, cardID, found.Currency
		// Only a credit card has virtual cards.
		if cardID == 0 {
			draft.Virtual = false
		}
	}
	if given["virtual"] {
		draft.Virtual = values.virtual
	}
	if given["value"] {
		if draft.Value, err = amount("edit", values.value, currency); err != nil {
			return draft, err
		}
	}
	// The value of a draft for a scope is the total of its parts.
	if given["installment-value"] {
		if draft.Value, err = amount("edit", values.installmentValue, currency); err != nil {
			return draft, err
		}
		draft.Value *= int64(values.parts)
	}
	return draft, nil
}

// editTitle is the heading of the form that changes a transaction.
func editTitle(current transaction.Transaction) string {
	if part := current.Installment(); part != "" {
		return fmt.Sprintf("Edit transaction %d (installment %s)", current.ID, part)
	}
	return fmt.Sprintf("Edit transaction %d", current.ID)
}

func (c *transactionsCommand) editForm(current transaction.View, draft transaction.Draft) (transaction.Draft, error) {
	accounts, err := c.accounts.List(false, today())
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
	goals, err := c.goals.List(false)
	if err != nil {
		return draft, err
	}
	// The goal the transaction has can be archived.
	if current.Goal != nil && !slices.ContainsFunc(goals, func(g goal.Goal) bool { return g.ID == current.GoalID }) {
		goals = append(goals, *current.Goal)
	}
	form, err := program(ui.NewTransactionForm(ui.TransactionFormOptions{
		Title: editTitle(current.Transaction), Initial: draft, Edit: true, Goals: goals,
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

// deleteQuestion asks for the confirmation of a delete. parts holds the
// parts of the scope.
func deleteQuestion(current transaction.Transaction, scope transaction.Scope, parts []transaction.Transaction) string {
	count := plural(int64(len(parts)), "transaction")
	switch {
	case scope == transaction.All:
		return fmt.Sprintf("Delete all the installments of %s (%s) permanently?", current.Title, count)
	case scope == transaction.Next:
		return fmt.Sprintf("Delete the installments %d to %d of %s (%s) permanently?", parts[0].Part, parts[len(parts)-1].Part, current.Title, count)
	case current.InstallmentID != 0:
		return fmt.Sprintf("Delete transaction %d (%s, installment %s) permanently? The other installments stay.", current.ID, current.Title, current.Installment())
	case current.PairID != 0:
		return fmt.Sprintf("Delete the transfer %s (transactions %d and %d) permanently?", current.Title, current.ID, current.PairID)
	case current.Type == transaction.Payment:
		return fmt.Sprintf("Delete the payment %d (%s)? The bill opens again.", current.ID, current.Title)
	}
	return fmt.Sprintf("Delete transaction %d (%s) permanently?", current.ID, current.Title)
}

func (c *transactionsCommand) delete(args []string) error {
	var yes, all, next bool
	positional, _, err := parse("delete", args, func(f *flag.FlagSet) {
		f.BoolVar(&yes, "yes", false, "")
		f.BoolVar(&all, "all", false, "")
		f.BoolVar(&next, "next", false, "")
	}, 1)
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
	scope, err := scopeOf("delete", all, next, current.Transaction)
	if err != nil {
		return err
	}
	parts, err := c.store.Parts(current.ID, scope)
	if err != nil {
		return err
	}
	if !yes {
		question := deleteQuestion(current.Transaction, scope, parts)
		confirm, err := program(ui.NewConfirm(question, "Delete permanently", true))
		if err != nil {
			return err
		}
		if !confirm.Confirmed() {
			return cancelled
		}
	}
	if scope == transaction.One {
		if err := c.store.Delete(current.ID); err != nil {
			return err
		}
		return c.print(ui.Success(label(current.Transaction, "deleted")) + "\n")
	}
	if _, err := c.store.DeleteParts(current.ID, scope); err != nil {
		return err
	}
	return c.print(ui.Success(span(parts, current.Title, "deleted")) + "\n")
}
